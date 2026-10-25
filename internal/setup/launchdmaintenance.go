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
	Schema                   int                    `json:"schema"`
	ID                       string                 `json:"id"`
	Revision                 uint64                 `json:"revision"`
	Plan                     MaintenancePlan        `json:"plan"`
	Phase                    string                 `json:"phase"`
	Successor                ServiceInstance        `json:"successor"`
	Generation               string                 `json:"generation"`
	Preserved                int                    `json:"preserved_envelopes"`
	OldOutcomes              string                 `json:"old_outcomes"`
	NextAction               string                 `json:"next_action"`
	Restoration              MaintenanceRestoration `json:"restoration,omitzero"`
	RetainedArtifactAttempts int                    `json:"retained_artifact_attempts,omitempty"`
}

type maintenanceReady struct {
	Instance   ServiceInstance
	Generation string
	Held       bool
}

// All effects are instance-local. Only the exact root backend implements this
// operation; it is deliberately separate from ordinary/strict Refresh.
type maintenanceOperations struct {
	preview           func(context.Context, string) (MaintenancePlan, error)
	lock              func(func() error) error
	load              func() (MaintenanceStatus, error)
	persist           func(uint64, MaintenanceStatus) error
	prepare           func(MaintenanceStatus) error
	pending           func(MaintenanceStatus) (bool, error)
	clearPreparation  func(MaintenanceStatus) error
	loadRecovery      func(string, string) (MaintenanceStatus, error)
	preparedSuccessor func(context.Context, MaintenanceStatus) (bool, error)
	abortVerify       func(context.Context, MaintenanceStatus) error
	abortFinish       func(context.Context, MaintenanceStatus) error
	restorePreview    func(context.Context, MaintenanceStatus, string, string) (MaintenanceRestorePlan, error)
	restorePrepare    func(context.Context, MaintenanceStatus, MaintenanceRestorePlan) (maintenanceArtifactPublication, error)
	exclude           func(context.Context, MaintenanceStatus, func() error) error
	revalidate        func(context.Context, MaintenancePlan, bool) error
	retire            func(context.Context, MaintenancePlan) error
	retirement        func(context.Context, MaintenancePlan) (string, error)
	preserve          func(context.Context, MaintenanceStatus) (int, error)
	bootstrap         func(context.Context, MaintenanceStatus) error
	resume            func(context.Context, MaintenanceStatus) error
	ready             func(context.Context, MaintenanceStatus) (maintenanceReady, error)
	activate          func(context.Context, MaintenanceStatus, maintenanceReady) error
	finish            func(context.Context, MaintenanceStatus) error
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
		if old.ID != "" && old.Phase != "completed" && old.Phase != "aborted" {
			result = old
			return errors.New("legacy maintenance: retained transaction requires exact status/recovery, not another retirement")
		}
		if old.ID != "" && d.pending != nil {
			pending, err := d.pending(old)
			if err != nil {
				return err
			}
			if pending {
				result = old
				return errors.New("legacy maintenance: completed transaction still has scoped cleanup; recover it before new confirmation")
			}
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
		if d.prepare != nil {
			if err := d.prepare(result); err != nil {
				result.Phase = "preparation_outcome_unverified"
				result.NextAction = "Inspect or abort the exact preparation; no retirement was attempted"
				return err
			}
		}
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
		if d.loadRecovery != nil {
			result, err = d.loadRecovery(id, digest)
		} else {
			result, err = d.load()
		}
		if err != nil {
			return err
		}
		if result.ID == "" || result.ID != id || result.Plan.SHA256 != digest || result.Plan.validate() != nil {
			return errors.New("legacy maintenance: recovery transaction/plan mismatch")
		}
		if result.Phase == "completed" || result.Phase == "aborted" {
			if d.pending == nil {
				return nil
			}
			pending, err := d.pending(result)
			if err != nil || !pending {
				return err
			}
		}
		if result.Phase == "abort_requested" {
			return errors.New("legacy maintenance: use exact abort to finish pre-retirement cleanup")
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

func (c ServiceController) AbortLegacyMaintenance(ctx context.Context, id, digest string) (MaintenanceStatus, error) {
	d, err := c.maintenance(ctx)
	if err != nil {
		return MaintenanceStatus{}, err
	}
	var status MaintenanceStatus
	err = d.lock(func() error {
		var err error
		if d.loadRecovery != nil {
			status, err = d.loadRecovery(id, digest)
		} else {
			status, err = d.load()
		}
		if err != nil {
			return err
		}
		if status.ID != id || status.Plan.SHA256 != digest || status.Plan.validate() != nil {
			return errors.New("legacy maintenance: exact abort transaction/plan required")
		}
		if status.Phase != "prepared" && status.Phase != "abort_requested" && status.Phase != "aborted" {
			return errors.New("legacy maintenance: retirement may have begun; abort is not authority to revive or replay")
		}
		if status.Phase == "aborted" && d.pending != nil {
			pending, err := d.pending(status)
			if err != nil || !pending {
				return err
			}
		}
		if d.abortVerify == nil || d.abortFinish == nil {
			return errors.New("legacy maintenance: verified pre-retirement abort unavailable")
		}
		return d.exclude(ctx, status, func() error {
			if err := d.abortVerify(ctx, status); err != nil {
				return err
			}
			if status.Phase == "prepared" {
				if err := maintenanceAdvance(d, &status, "abort_requested", "Verify original still owns the exact job before restoring only this admission"); err != nil {
					return err
				}
			}
			if status.Phase == "abort_requested" {
				if err := maintenanceAdvance(d, &status, "aborted", "No retirement performed; verified scoped hold cleanup may still require recovery"); err != nil {
					return err
				}
			}
			return d.abortFinish(ctx, status)
		})
	})
	return status, err
}

func runLegacyMaintenance(ctx context.Context, d maintenanceOperations, s *MaintenanceStatus, recovering bool) error {
	return d.exclude(ctx, *s, func() error {
		if s.Phase == "prepared" || s.Phase == "aborted" {
			if recovering && d.preparedSuccessor != nil {
				held, err := d.preparedSuccessor(ctx, *s)
				if err != nil {
					return err
				}
				if held {
					if err := maintenanceAdvance(d, s, "retirement_observed", "Original instance disappeared; exact successor is already startup-held; no bootout performed"); err != nil {
						return err
					}
					if err := maintenanceAdvance(d, s, "retired", "Preserve custody before activating the independently verified held successor"); err != nil {
						return err
					}
				}
				if !held && s.Phase == "aborted" {
					return errors.New("legacy maintenance: original is still alive; finish the exact abort cleanup instead of starting a successor")
				}
			}
		}
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
		} else if s.Phase == "retirement_observed" {
			if d.preparedSuccessor == nil {
				return errors.New("legacy maintenance: held-successor recovery unavailable")
			}
			held, err := d.preparedSuccessor(ctx, *s)
			if err != nil || !held {
				return errors.Join(errors.New("legacy maintenance: previously observed retirement no longer established"), err)
			}
			if err := maintenanceAdvance(d, s, "retired", "Recover the exact independently held successor"); err != nil {
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
		if recovering && (s.Phase == "bootstrap_requested" || s.Phase == "activation_requested" || s.Phase == "activated" || s.Phase == "completed") {
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
		if s.Phase == "completed" && ready.Instance == s.Successor && ready.Generation == s.Generation && !ready.Held {
			if err := d.finish(recovery, *s); err != nil {
				return err
			}
			if d.clearPreparation != nil {
				return d.clearPreparation(*s)
			}
			return nil
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
		if err := maintenanceAdvance(d, s, "completed", "Activation verified; retained envelopes and old remote outcomes remain separate recovery debt; no replay authorized"); err != nil {
			return err
		}
		if d.clearPreparation != nil {
			return d.clearPreparation(*s)
		}
		return nil
	})
}

func maintenanceTransition(previous, next MaintenanceStatus) bool {
	if previous.ID == next.ID && previous.Phase == next.Phase && next.Restoration != previous.Restoration {
		return maintenanceRestoreTransition(previous, next)
	}
	if previous.ID != next.ID {
		return (previous.ID == "" || previous.Phase == "completed" || previous.Phase == "aborted") && next.Phase == "prepared"
	}
	if previous.Plan != next.Plan || previous.Preserved > next.Preserved {
		return false
	}
	switch previous.Phase {
	case "prepared":
		return next.Phase == "retirement_requested" || next.Phase == "retirement_observed" || next.Phase == "abort_requested"
	case "abort_requested":
		return next.Phase == "aborted"
	case "aborted":
		return next.Phase == "retirement_observed"
	case "retirement_observed":
		return next.Phase == "retired"
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
	case "completed":
		// Only exact explicit recovery with a retained preparation can reach
		// this branch; it binds a freshly verified held process generation.
		return next.Phase == "bootstrap_requested"
	}
	return false
}
