package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
)

// Only the already-approved protected replacement can be restored. This is
// neither a release rollback nor a caller-selected destination/config editor.
type MaintenanceRestorePlan struct {
	Transaction       string `json:"transaction"`
	MaintenanceSHA256 string `json:"maintenance_sha256"`
	Revision          uint64 `json:"revision"`
	Artifact          string `json:"artifact"`
	ArtifactSHA256    string `json:"artifact_sha256"`
	ArtifactCDHash    string `json:"artifact_cdhash"`
	Nonce             string `json:"nonce"`
	ParentDevice      uint64 `json:"parent_device"`
	ParentInode       uint64 `json:"parent_inode"`
	TargetExists      bool   `json:"target_exists"`
	TargetDevice      uint64 `json:"target_device"`
	TargetInode       uint64 `json:"target_inode"`
	TargetSize        int64  `json:"target_size"`
	TargetMode        uint32 `json:"target_mode"`
	TargetUID         uint32 `json:"target_uid"`
	TargetGID         uint32 `json:"target_gid"`
	TargetSHA256      string `json:"target_sha256"`
	SHA256            string `json:"sha256"`
}

type MaintenanceRestoration struct {
	Plan        MaintenanceRestorePlan `json:"plan"`
	Phase       string                 `json:"phase"`
	StageDevice uint64                 `json:"stage_device"`
	StageInode  uint64                 `json:"stage_inode"`
	StagePath   string                 `json:"stage_path"`
	BackupPath  string                 `json:"backup_path"`
}

type maintenanceArtifactPublication interface {
	Prepared() MaintenanceRestoration
	Publish(context.Context) error
	Close() error
}

func DecodeMaintenanceRestorePlan(raw []byte) (MaintenanceRestorePlan, error) {
	return servicedrain.DecodeMaintenance[MaintenanceRestorePlan](raw)
}

func restoreDigest(p MaintenanceRestorePlan) string {
	p.SHA256 = ""
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (p MaintenanceRestorePlan) validate(s MaintenanceStatus) error {
	nonce, err := hex.DecodeString(p.Nonce)
	if err != nil || len(nonce) != 16 || hex.EncodeToString(nonce) != p.Nonce || p.Transaction != s.ID || p.MaintenanceSHA256 != s.Plan.SHA256 || p.Revision != s.Revision || p.ArtifactSHA256 != s.Plan.Original.ProgramSHA256 || p.ArtifactCDHash != s.Plan.ReplacementCDHash || p.Artifact == "" || p.ParentInode == 0 || p.SHA256 != restoreDigest(p) {
		return errors.New("legacy maintenance: exact approved replacement restoration plan required")
	}
	if p.TargetExists && (!serviceDigestRE.MatchString(p.TargetSHA256) || p.TargetInode == 0 || p.TargetSize < 0) {
		return errors.New("legacy maintenance: current target identity incomplete")
	}
	return nil
}

func (c ServiceController) PreviewLegacyRestore(ctx context.Context, id, digest, artifact string) (MaintenanceRestorePlan, error) {
	d, err := c.maintenance(ctx)
	if err != nil {
		return MaintenanceRestorePlan{}, err
	}
	var plan MaintenanceRestorePlan
	err = d.lock(func() error {
		s, err := d.load()
		if err != nil {
			return err
		}
		if s.ID != id || s.Plan.SHA256 != digest {
			return errors.New("legacy maintenance: exact retained transaction required")
		}
		if d.restorePreview == nil {
			return errors.New("legacy maintenance: verified replacement restoration unavailable")
		}
		if d.pending != nil {
			pending, err := d.pending(s)
			if err != nil || !pending {
				return errors.Join(errors.New("legacy maintenance: no pending transaction custody for restoration"), err)
			}
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		plan, err = d.restorePreview(ctx, s, artifact, hex.EncodeToString(nonce[:]))
		if err != nil {
			return err
		}
		return plan.validate(s)
	})
	return plan, err
}

func (c ServiceController) ConfirmLegacyRestore(ctx context.Context, p MaintenanceRestorePlan) (status MaintenanceStatus, err error) {
	d, err := c.maintenance(ctx)
	if err != nil {
		return status, err
	}
	err = d.lock(func() (resultErr error) {
		var err error
		status, err = d.load()
		if err != nil {
			return err
		}
		if err := p.validate(status); err != nil {
			return err
		}
		if d.pending != nil {
			pending, err := d.pending(status)
			if err != nil || !pending {
				return errors.Join(errors.New("legacy maintenance: retained preparation no longer owns restoration"), err)
			}
		}
		if d.restorePreview == nil || d.restorePrepare == nil {
			return errors.New("legacy maintenance: restoration backend unavailable")
		}
		fresh, err := d.restorePreview(ctx, status, p.Artifact, p.Nonce)
		if err != nil || fresh != p {
			return errors.Join(errors.New("legacy maintenance: restore selection/target changed before confirmation"), err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		stage, backup := maintenanceRestoreNames(p)
		r := MaintenanceRestoration{Plan: p, Phase: "preparing", StagePath: stage, BackupPath: backup}
		if err := maintenanceRestoreRecord(d, &status, r); err != nil {
			return err
		}
		publication, err := d.restorePrepare(ctx, status, p)
		if publication != nil {
			defer func() { resultErr = errors.Join(resultErr, publication.Close()) }()
		}
		if err != nil {
			return err
		}
		if publication == nil {
			return errors.New("legacy maintenance: prepared publication unavailable")
		}
		r = publication.Prepared()
		if err = maintenanceRestoreRecord(d, &status, r); err != nil {
			return err
		}
		r.Phase = "publication_requested"
		if err = maintenanceRestoreRecord(d, &status, r); err != nil {
			return err
		}
		// Publication has durable exact intent. Finish its bounded readback even
		// if the UI's request context is canceled; never silently restore old code.
		recovery, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = publication.Publish(recovery); err != nil {
			return err
		}
		r.Phase = "verified_current_replacement"
		err = maintenanceRestoreRecord(d, &status, r)
		return err
	})
	return status, err
}

func maintenanceRestoreNames(p MaintenanceRestorePlan) (string, string) {
	stem := "/usr/local/libexec/sirsi-pantheon/.root-relay-maintenance-" + p.Transaction + "-" + p.Nonce
	return stem + ".stage", stem + ".before"
}

func maintenanceRestoreRecord(d maintenanceOperations, s *MaintenanceStatus, r MaintenanceRestoration) error {
	next := *s
	next.Revision++
	next.Restoration = r
	if r.Phase == "preparing" && s.Restoration.Phase != "" && s.Restoration.Phase != "verified_current_replacement" {
		next.RetainedArtifactAttempts++
	}
	next.NextAction = "Replacement artifact restoration: " + r.Phase + "; retain named backup/stage and prior attempts; no service or envelope replay occurred"
	if err := d.persist(s.Revision, next); err != nil {
		return err
	}
	*s = next
	return nil
}

func maintenanceRestoreTransition(previous, next MaintenanceStatus) bool {
	a, b := previous, next
	a.Revision, b.Revision = 0, 0
	a.NextAction, b.NextAction = "", ""
	a.Restoration, b.Restoration = MaintenanceRestoration{}, MaintenanceRestoration{}
	a.RetainedArtifactAttempts, b.RetainedArtifactAttempts = 0, 0
	if a != b || next.Restoration.Plan.Transaction != next.ID || next.Restoration.Plan.MaintenanceSHA256 != next.Plan.SHA256 {
		return false
	}
	if next.Restoration.Phase == "preparing" {
		return next.Restoration.Plan.Revision == previous.Revision && next.Restoration.Plan.SHA256 == restoreDigest(next.Restoration.Plan)
	}
	if previous.Restoration.Plan != next.Restoration.Plan {
		return false
	}
	switch previous.Restoration.Phase {
	case "preparing":
		return next.Restoration.Phase == "prepared"
	case "prepared":
		return next.Restoration.Phase == "publication_requested"
	case "publication_requested":
		return next.Restoration.Phase == "verified_current_replacement"
	}
	return false
}
