package apprecovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"
)

type Kind string
type Mode string

const (
	KindAppSavedState Kind = "app_saved_state"
	KindLaunchd       Kind = "launchd_service"
	KindCheckpointed  Kind = "checkpointed_process"
	KindUnsupported   Kind = "unsupported"
)

const (
	ModeRestore Mode = "restore"
	ModeFresh   Mode = "fresh"
)

type Phase string

const (
	PhaseCaptured Phase = "captured"
	PhaseStopped  Phase = "stopped"
	PhaseStarted  Phase = "started"
	PhaseReady    Phase = "ready"
	PhaseFailed   Phase = "failed"
)

var targetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

var ErrUnsupported = errors.New("recovery is unsupported for this target")

type Target struct {
	ID              string
	Kind            Kind
	BundleID        string
	ExecutablePath  string
	LaunchdTarget   string
	StatePaths      []string
	ReadinessURL    string
	ReadyTimeout    time.Duration
	StartArguments  []string
	FreshStatePaths []string
	AutoResume      bool
}

func (t Target) validate() error {
	if !targetIDPattern.MatchString(t.ID) {
		return errors.New("invalid recovery target id")
	}
	switch t.Kind {
	case KindAppSavedState:
		if t.BundleID == "" || t.ExecutablePath == "" {
			return errors.New("app recovery requires bundle id and executable path")
		}
	case KindLaunchd:
		if t.LaunchdTarget == "" || t.ExecutablePath == "" {
			return errors.New("launchd recovery requires target and executable path")
		}
		if len(t.FreshStatePaths) != 0 {
			return errors.New("generic launchd recovery cannot clear persistent state paths")
		}
	case KindCheckpointed:
		if t.ExecutablePath == "" {
			return errors.New("checkpointed recovery requires executable path")
		}
	case KindUnsupported:
		return ErrUnsupported
	default:
		return errors.New("unknown recovery kind")
	}
	return nil
}

type Snapshot struct {
	Files map[string]string `json:"files,omitempty"`
}

type Receipt struct {
	Schema      string    `json:"schema"`
	TargetID    string    `json:"target_id"`
	Kind        Kind      `json:"kind"`
	Mode        Mode      `json:"mode"`
	Phase       Phase     `json:"phase"`
	OldPID      int       `json:"old_pid,omitempty"`
	NewPID      int       `json:"new_pid,omitempty"`
	Snapshot    Snapshot  `json:"snapshot,omitempty"`
	FailureCode string    `json:"failure_code,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Driver interface {
	Capture(context.Context, Target) (Snapshot, error)
	ClearTransientState(context.Context, Target) error
	PID(context.Context, Target) (int, error)
	Stop(context.Context, Target, int) error
	Start(context.Context, Target, Snapshot) error
	Ready(context.Context, Target) error
}

type Store interface {
	Save(Receipt) error
	Load(string) (Receipt, error)
}

type Manager struct {
	targets map[string]Target
	driver  Driver
	store   Store
	now     func() time.Time
	poll    time.Duration
}

type Capability struct {
	TargetID         string `json:"target_id"`
	Kind             Kind   `json:"kind"`
	RestoreSupported bool   `json:"restore_supported"`
	FreshSupported   bool   `json:"fresh_supported"`
	AutoResume       bool   `json:"auto_resume"`
}

func (m *Manager) Capabilities() []Capability {
	result := make([]Capability, 0, len(m.targets))
	for _, target := range m.targets {
		result = append(result, Capability{
			TargetID:         target.ID,
			Kind:             target.Kind,
			RestoreSupported: target.Kind != KindUnsupported && len(target.StatePaths) > 0,
			FreshSupported:   target.Kind != KindUnsupported,
			AutoResume:       target.AutoResume,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TargetID < result[j].TargetID })
	return result
}

type ReconcileResult struct {
	TargetID string
	Receipt  Receipt
	Err      error
}

func (m *Manager) ReconcilePending(ctx context.Context) []ReconcileResult {
	results := make([]ReconcileResult, 0)
	for _, capability := range m.Capabilities() {
		if !capability.AutoResume {
			continue
		}
		receipt, err := m.store.Load(capability.TargetID)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			results = append(results, ReconcileResult{TargetID: capability.TargetID, Err: err})
			continue
		}
		if receipt.Phase != PhaseCaptured && receipt.Phase != PhaseStopped && receipt.Phase != PhaseStarted {
			continue
		}
		resumed, resumeErr := m.Resume(ctx, capability.TargetID)
		results = append(results, ReconcileResult{TargetID: capability.TargetID, Receipt: resumed, Err: resumeErr})
	}
	return results
}

func (m *Manager) Latest(targetID string) (Receipt, error) {
	if _, ok := m.targets[targetID]; !ok {
		return Receipt{}, errors.New("unknown recovery target")
	}
	return m.store.Load(targetID)
}

func NewManager(targets []Target, driver Driver, store Store) (*Manager, error) {
	if driver == nil || store == nil {
		return nil, errors.New("recovery driver and store are required")
	}
	registry := make(map[string]Target, len(targets))
	for _, target := range targets {
		if err := target.validate(); err != nil && !errors.Is(err, ErrUnsupported) {
			return nil, fmt.Errorf("target %q: %w", target.ID, err)
		}
		if _, exists := registry[target.ID]; exists {
			return nil, fmt.Errorf("duplicate recovery target %q", target.ID)
		}
		registry[target.ID] = target
	}
	return &Manager{targets: registry, driver: driver, store: store, now: time.Now, poll: 250 * time.Millisecond}, nil
}

func (m *Manager) Recover(ctx context.Context, targetID string, mode Mode) (Receipt, error) {
	target, ok := m.targets[targetID]
	if !ok {
		return Receipt{}, errors.New("unknown recovery target")
	}
	now := m.now().UTC()
	if mode != ModeRestore && mode != ModeFresh {
		return Receipt{}, errors.New("restart mode must be restore or fresh")
	}
	receipt := Receipt{Schema: "pantheon.app-recovery.v1", TargetID: target.ID, Kind: target.Kind, Mode: mode, StartedAt: now, UpdatedAt: now}
	if err := target.validate(); err != nil {
		return m.fail(receipt, "unsupported", err)
	}

	oldPID, err := m.driver.PID(ctx, target)
	if err != nil {
		return m.fail(receipt, "pid_lookup_failed", err)
	}
	receipt.OldPID = oldPID
	if mode == ModeRestore {
		if len(target.StatePaths) == 0 {
			return m.fail(receipt, "restore_state_undeclared", errors.New("restore requires declared state paths"))
		}
		receipt.Snapshot, err = m.driver.Capture(ctx, target)
		if err != nil {
			return m.fail(receipt, "state_capture_failed", err)
		}
	}
	receipt.Phase = PhaseCaptured
	if err := m.persist(&receipt); err != nil {
		return receipt, err
	}
	if err := m.driver.Stop(ctx, target, oldPID); err != nil {
		return m.fail(receipt, "stop_failed", err)
	}
	if mode == ModeFresh && target.Kind != KindLaunchd {
		if err := m.waitForStop(ctx, target); err != nil {
			return m.fail(receipt, "stop_verification_failed", err)
		}
	}
	if mode == ModeFresh {
		if err := m.driver.ClearTransientState(ctx, target); err != nil {
			return m.fail(receipt, "transient_state_clear_failed", err)
		}
	}
	receipt.Phase = PhaseStopped
	if err := m.persist(&receipt); err != nil {
		return receipt, err
	}
	return m.startAndVerify(ctx, target, receipt)
}

func (m *Manager) waitForStop(ctx context.Context, target Target) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(m.poll)
	defer ticker.Stop()
	for {
		pid, err := m.driver.PID(ctx, target)
		if err == nil && pid == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("original process did not exit")
		case <-ticker.C:
		}
	}
}

func (m *Manager) Resume(ctx context.Context, targetID string) (Receipt, error) {
	target, ok := m.targets[targetID]
	if !ok {
		return Receipt{}, errors.New("unknown recovery target")
	}
	receipt, err := m.store.Load(targetID)
	if err != nil {
		return Receipt{}, err
	}
	if receipt.Schema != "pantheon.app-recovery.v1" || receipt.TargetID != target.ID || receipt.Kind != target.Kind {
		return receipt, errors.New("recovery receipt does not match registered target authority")
	}
	if receipt.Mode != ModeRestore && receipt.Mode != ModeFresh {
		return receipt, errors.New("recovery receipt has invalid restart mode")
	}
	if receipt.Mode == ModeRestore && len(receipt.Snapshot.Files) == 0 {
		return receipt, errors.New("restore receipt has no captured state")
	}
	if receipt.Phase == PhaseReady {
		return receipt, nil
	}
	if receipt.Phase != PhaseCaptured && receipt.Phase != PhaseStopped && receipt.Phase != PhaseStarted {
		return receipt, errors.New("recovery receipt is not resumable")
	}
	return m.startAndVerify(ctx, target, receipt)
}

func (m *Manager) startAndVerify(ctx context.Context, target Target, receipt Receipt) (Receipt, error) {
	shouldStart := receipt.Phase != PhaseStarted
	if receipt.Phase == PhaseStarted {
		pid, err := m.driver.PID(ctx, target)
		shouldStart = err == nil && pid == 0
	}
	if shouldStart {
		if err := m.driver.Start(ctx, target, receipt.Snapshot); err != nil {
			return m.fail(receipt, "start_failed", err)
		}
		receipt.Phase = PhaseStarted
		if err := m.persist(&receipt); err != nil {
			return receipt, err
		}
	}
	timeout := target.ReadyTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(m.poll)
	defer ticker.Stop()
	for {
		newPID, pidErr := m.driver.PID(ctx, target)
		readyErr := m.driver.Ready(ctx, target)
		if pidErr == nil && readyErr == nil && newPID > 0 && (receipt.OldPID == 0 || newPID != receipt.OldPID) {
			receipt.NewPID = newPID
			receipt.Phase = PhaseReady
			if err := m.persist(&receipt); err != nil {
				return receipt, err
			}
			return receipt, nil
		}
		select {
		case <-ctx.Done():
			return m.fail(receipt, "recovery_canceled", ctx.Err())
		case <-deadline.C:
			return m.fail(receipt, "readiness_timeout", errors.New("replacement process did not become ready"))
		case <-ticker.C:
		}
	}
}

func (m *Manager) persist(receipt *Receipt) error {
	receipt.UpdatedAt = m.now().UTC()
	return m.store.Save(*receipt)
}

func (m *Manager) fail(receipt Receipt, code string, cause error) (Receipt, error) {
	receipt.Phase = PhaseFailed
	receipt.FailureCode = code
	if err := m.persist(&receipt); err != nil {
		return receipt, fmt.Errorf("%s: %v; receipt: %w", code, cause, err)
	}
	return receipt, fmt.Errorf("%s: %w", code, cause)
}
