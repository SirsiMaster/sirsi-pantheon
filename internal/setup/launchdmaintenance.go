package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const RootRelayMaintenanceProfile = "root-relay-legacy-maintenance-v1"

type MaintenanceImageIdentity struct {
	PID    int    `json:"pid"`
	CDHash string `json:"cdhash"`
}

// MaintenancePlan is not a drain receipt. Loaded identifies the retained old
// signed payload separately from the replacement currently at Program.
type MaintenancePlan struct {
	Profile           string          `json:"profile"`
	Original          ServiceInstance `json:"original"`
	LegacyPayload     string          `json:"legacy_payload"`
	LegacySHA256      string          `json:"legacy_sha256"`
	LoadedCDHash      string          `json:"loaded_cdhash"`
	KernelStart       string          `json:"kernel_start"`
	ReplacementCDHash string          `json:"replacement_cdhash"`
	LegacySource      string          `json:"legacy_source"`
	RootDevice        uint64          `json:"root_device"`
	RootInode         uint64          `json:"root_inode"`
	WriterScope       string          `json:"writer_scope"`
	SHA256            string          `json:"sha256"`
}

func maintenanceDigest(p MaintenancePlan) string {
	p.SHA256 = ""
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func jsonMaintenance(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	return append(b, '\n'), err
}

func (p MaintenancePlan) validate() error {
	if p.Profile != RootRelayMaintenanceProfile || p.Original.Label != "ai.sirsi.router.relay" ||
		p.Original.Target != "system/ai.sirsi.router.relay" ||
		p.Original.Plist != "/Library/LaunchDaemons/ai.sirsi.router.relay.plist" ||
		p.Original.Program != "/usr/local/libexec/sirsi-pantheon/sirsi" || validateServiceInstance(p.Original) != nil ||
		!serviceDigestRE.MatchString(p.LegacySHA256) || p.LegacyPayload == "" || p.LegacySource == "" ||
		len(p.LoadedCDHash) != 40 || len(p.ReplacementCDHash) != 40 || p.KernelStart == "" || p.RootInode == 0 ||
		p.WriterScope != "cooperative-publication-only; uncovered-writers-and-old-remote-outcomes-unknown" ||
		p.SHA256 != maintenanceDigest(p) {
		return errors.New("legacy maintenance: complete exact root profile and loaded/replacement identities required")
	}
	for _, value := range []string{p.LoadedCDHash, p.ReplacementCDHash} {
		if b, err := hex.DecodeString(value); err != nil || hex.EncodeToString(b) != value {
			return errors.New("legacy maintenance: invalid loaded/replacement code identity")
		}
	}
	return nil
}

// MaintenanceStatus is the private durable transaction projection. No token,
// plist environment or request body belongs in it. Transport debt is never a
// certificate that an old remote operation committed or did not commit.
type MaintenanceStatus struct {
	Schema      int             `json:"schema"`
	ID          string          `json:"id"`
	Revision    uint64          `json:"revision"`
	Plan        MaintenancePlan `json:"plan"`
	Phase       string          `json:"phase"`
	Successor   ServiceInstance `json:"successor"`
	Generation  string          `json:"generation"`
	Preserved   int             `json:"preserved_envelopes"`
	OldOutcomes string          `json:"old_outcomes"`
	NextAction  string          `json:"next_action"`
}

type maintenanceReady struct {
	Instance   ServiceInstance
	Generation string
	Held       bool
}

// All effects are instance-local. Only the exact root backend implements this
// operation; it is deliberately separate from ordinary/strict Refresh.
type maintenanceOperations struct {
	preview    func(context.Context, string) (MaintenancePlan, error)
	lock       func(func() error) error
	load       func() (MaintenanceStatus, error)
	persist    func(uint64, MaintenanceStatus) error
	exclude    func(context.Context, MaintenanceStatus, func() error) error
	revalidate func(context.Context, MaintenancePlan, bool) error
	retire     func(context.Context, MaintenancePlan) error
	retirement func(context.Context, MaintenancePlan) (string, error)
	preserve   func(context.Context, MaintenanceStatus) (int, error)
	bootstrap  func(context.Context, MaintenanceStatus) error
	resume     func(context.Context, MaintenanceStatus) error
	ready      func(context.Context, MaintenanceStatus) (maintenanceReady, error)
	activate   func(context.Context, MaintenanceStatus, maintenanceReady) error
	finish     func(context.Context, MaintenanceStatus) error
}

type maintenanceProvider interface {
	maintenanceOperations() maintenanceOperations
}

func (c ServiceController) maintenance(ctx context.Context) (maintenanceOperations, error) {
	if c.Authorize == nil || c.Backend == nil {
		return maintenanceOperations{}, errors.New("legacy maintenance: existing owner authorization and native backend required")
	}
	// Resolve/authenticate before taking the physical-root publication barrier.
	if err := c.Authorize(ctx, "ai.sirsi.router.relay"); err != nil {
		return maintenanceOperations{}, err
	}
	b, ok := c.Backend.(maintenanceProvider)
	if !ok {
		return maintenanceOperations{}, errors.New("legacy maintenance: root profile unsupported on this backend")
	}
	return b.maintenanceOperations(), nil
}

func (c ServiceController) PreviewLegacyMaintenance(ctx context.Context, legacyPayload string) (MaintenancePlan, error) {
	d, err := c.maintenance(ctx)
	if err != nil {
		return MaintenancePlan{}, err
	}
	p, err := d.preview(ctx, legacyPayload)
	if err == nil {
		err = p.validate()
	}
	return p, err
}

func (c ServiceController) LegacyMaintenanceStatus(ctx context.Context) (MaintenanceStatus, error) {
	d, err := c.maintenance(ctx)
	if err != nil {
		return MaintenanceStatus{}, err
	}
	return d.load()
}

// Confirm acknowledges interruption and retained outcome-unknown custody, not
// "--drained". The complete preview digest is the immutable approval binding.
func (c ServiceController) ConfirmLegacyMaintenance(ctx context.Context, p MaintenancePlan) (MaintenanceStatus, error) {
	if err := p.validate(); err != nil {
		return MaintenanceStatus{}, err
	}
	d, err := c.maintenance(ctx)
	if err != nil {
		return MaintenanceStatus{}, err
	}
	var result MaintenanceStatus
	err = d.lock(func() error {
		old, err := d.load()
		if err != nil {
			return err
		}
		if old.ID != "" && old.Phase != "completed" {
			result = old
			return errors.New("legacy maintenance: retained transaction requires exact status/recovery, not another retirement")
		}
		fresh, err := d.preview(ctx, p.LegacyPayload)
		if err != nil || fresh != p {
			return errors.Join(errors.New("legacy maintenance: preview changed before confirmation"), err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		result = MaintenanceStatus{Schema: 1, ID: hex.EncodeToString(nonce[:]), Revision: old.Revision,
			Plan: p, OldOutcomes: "unknown_not_certified_drained"}
		if err := maintenanceAdvance(d, &result, "prepared", "Confirm maintenance or inspect retained transaction"); err != nil {
			result.Phase = "preparation_outcome_unverified"
			result.NextAction = "Read exact private journal status; no service retirement was attempted and no automatic reset is authorized"
			return err
		}
		return runLegacyMaintenance(ctx, d, &result, false)
	})
	return result, err
}

func (c ServiceController) RecoverLegacyMaintenance(ctx context.Context, id, digest string) (MaintenanceStatus, error) {
	d, err := c.maintenance(ctx)
	if err != nil {
		return MaintenanceStatus{}, err
	}
	var result MaintenanceStatus
	err = d.lock(func() error {
		var err error
		result, err = d.load()
		if err != nil {
			return err
		}
		if result.ID == "" || result.ID != id || result.Plan.SHA256 != digest || result.Plan.validate() != nil {
			return errors.New("legacy maintenance: recovery transaction/plan mismatch")
		}
		if result.Phase == "completed" {
			return nil // Durable completion is history, not authority to restart again.
		}
		return runLegacyMaintenance(ctx, d, &result, true)
	})
	return result, err
}

func maintenanceAdvance(d maintenanceOperations, s *MaintenanceStatus, phase, next string) error {
	nextStatus := *s
	nextStatus.Revision++
	nextStatus.Phase, nextStatus.NextAction = phase, next
	if s.Phase != "" && !maintenanceTransition(*s, nextStatus) {
		return errors.New("legacy maintenance: invalid phase transition")
	}
	if err := d.persist(s.Revision, nextStatus); err != nil {
		return fmt.Errorf("legacy maintenance: phase %s persistence failed; retain transaction: %w", phase, err)
	}
	*s = nextStatus
	return nil
}

func runLegacyMaintenance(ctx context.Context, d maintenanceOperations, s *MaintenanceStatus, recovering bool) error {
	return d.exclude(ctx, *s, func() error {
		if s.Phase == "prepared" {
			if err := d.revalidate(ctx, s.Plan, true); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err // No lifecycle mutation; the durable hold remains recoverable.
			}
			if err := maintenanceAdvance(d, s, "retirement_requested", "Inspect retirement; never blindly repeat bootout"); err != nil {
				return err
			}
			retireErr := d.retire(ctx, s.Plan)
			// A failed launchctl command is not proof of either liveness or absence.
			probe, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			state, probeErr := d.retirement(probe, s.Plan)
			cancel()
			if state != "retired" || probeErr != nil {
				return errors.Join(fmt.Errorf("legacy maintenance: retirement %s; read-only status/recovery required", state), retireErr, probeErr)
			}
			// Even a command error may be resolved by independent positive retirement
			// proof. It is never resolved by a failed launchctl print alone.
			if err := maintenanceAdvance(d, s, "retired", "Resume held replacement activation; preserve old transport outcomes"); err != nil {
				return err
			}
		} else if s.Phase == "retirement_requested" {
			state, err := d.retirement(ctx, s.Plan)
			if err != nil || state != "retired" {
				return errors.Join(fmt.Errorf("legacy maintenance: retirement %s; no repeat bootout", state), err)
			}
			if err := maintenanceAdvance(d, s, "retired", "Resume held replacement activation"); err != nil {
				return err
			}
		}
		// Once retired, caller cancellation cannot abandon the finite recovery.
		recovery, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := d.revalidate(recovery, s.Plan, false); err != nil {
			return err
		}
		if s.Phase == "retired" {
			count, err := d.preserve(recovery, *s)
			if err != nil {
				return err
			}
			updated := *s
			updated.Preserved = count
			if err := maintenanceAdvance(d, &updated, "custody_preserved", "Bootstrap exact replacement held before startup recovery"); err != nil {
				return err
			}
			*s = updated
		}
		if s.Phase == "custody_preserved" {
			if err := maintenanceAdvance(d, s, "bootstrap_requested", "Inspect held successor before retry; never duplicate bootstrap"); err != nil {
				return err
			}
			if err := d.bootstrap(recovery, *s); err != nil {
				return err
			}
		}
		if recovering && (s.Phase == "bootstrap_requested" || s.Phase == "activation_requested" || s.Phase == "activated") {
			if err := d.resume(recovery, *s); err != nil {
				return err
			}
		}
		ready, err := d.ready(recovery, *s)
		if err != nil {
			return err
		}
		if ready.Instance.PID == s.Plan.Original.PID && ready.Instance.Started == s.Plan.Original.Started {
			return errors.New("legacy maintenance: original instance cannot be its successor")
		}
		if s.Generation != "" && (s.Generation != ready.Generation || s.Successor != ready.Instance) {
			if !recovering || !ready.Held {
				return errors.New("legacy maintenance: successor identity changed; no stale activation")
			}
			// Reboot/controller-death recovery binds a fresh, independently verified
			// held generation to this same durable transaction before activation.
			updated := *s
			updated.Successor, updated.Generation = ready.Instance, ready.Generation
			if err := maintenanceAdvance(d, &updated, "bootstrap_requested", "Recover only this freshly verified held generation"); err != nil {
				return err
			}
			*s = updated
		}
		if s.Phase != "activated" {
			if !ready.Held && s.Phase != "activation_requested" {
				return errors.New("legacy maintenance: replacement did not start held")
			}
			if s.Phase != "activation_requested" {
				updated := *s
				updated.Successor, updated.Generation = ready.Instance, ready.Generation
				if err := maintenanceAdvance(d, &updated, "activation_requested", "Verify transaction-bound activation before removing startup hold"); err != nil {
					return err
				}
				*s = updated
			}
			if err := d.activate(recovery, *s, ready); err != nil {
				return err
			}
			if err := maintenanceAdvance(d, s, "activated", "Producer activated; old remote outcomes and retained envelopes remain separate recovery debt"); err != nil {
				return err
			}
		}
		if err := d.finish(recovery, *s); err != nil {
			return err
		}
		return maintenanceAdvance(d, s, "completed", "Activation verified; retained envelopes and old remote outcomes remain separate recovery debt; no replay authorized")
	})
}

func maintenanceTransition(previous, next MaintenanceStatus) bool {
	if previous.ID != next.ID {
		return (previous.ID == "" || previous.Phase == "completed") && next.Phase == "prepared"
	}
	if previous.Plan != next.Plan || previous.Preserved > next.Preserved {
		return false
	}
	switch previous.Phase {
	case "prepared":
		return next.Phase == "retirement_requested"
	case "retirement_requested":
		return next.Phase == "retired"
	case "retired":
		return next.Phase == "custody_preserved"
	case "custody_preserved":
		return next.Phase == "bootstrap_requested"
	case "bootstrap_requested":
		return next.Phase == "activation_requested" || next.Phase == "bootstrap_requested"
	case "activation_requested":
		return next.Phase == "activated" || next.Phase == "bootstrap_requested"
	case "activated":
		return next.Phase == "completed" || next.Phase == "bootstrap_requested"
	}
	return false
}
