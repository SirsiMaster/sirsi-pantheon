package routerstore

import "fmt"

// Additive protocol discovery, not an activation flag or fleet attestation.
// A capability read can hit a different revision than the mutation: unknown
// YieldTaskLease still refuses and NEVER falls back to a failure release.
type TaskYieldProtocol struct {
	Version            int  `json:"version"`
	Clean              bool `json:"clean"`
	ExternalWait       bool `json:"external_wait"`
	CurrentAttemptOnly bool `json:"current_attempt_only"`
	AtomicAudit        bool `json:"atomic_audit"`
}

func (s *SQLiteStore) TaskYieldCapabilities(agent string) (TaskYieldProtocol, error) {
	if agent == "" {
		return TaskYieldProtocol{}, fmt.Errorf("task yield capability: agent required")
	}
	return TaskYieldProtocol{1, true, true, true, true}, nil
}
func (rs *RemoteStore) TaskYieldCapabilities(agent string) (TaskYieldProtocol, error) {
	var out TaskYieldProtocol
	err := rs.call("TaskYieldCapabilities", []any{agent}, &out)
	return out, err
}

// TaskAttemptOutcome is selected by the owning caller, never classified from
// an error/reason string. Legacy failure and lease expiry retain their charges.
type TaskAttemptOutcome struct {
	Disposition   string
	EvidenceRef   string
	Dependency    string
	FailureReason string
}
type taskOutcomeStore interface {
	TaskYieldCapabilities(string) (TaskYieldProtocol, error)
	YieldTaskLease(string, string, string, string, string, string) error
	ReleaseTaskLease(string, string, string, string) error
}

// SettleTaskAttempt is the actual CLI/structured caller dispatch boundary.
// Refusal leaves ownership and accounting unchanged for reconciliation.
func SettleTaskAttempt(s taskOutcomeStore, agent, task, token string, o TaskAttemptOutcome) error {
	switch o.Disposition {
	case "failure":
		if o.EvidenceRef != "" || o.Dependency != "" {
			return fmt.Errorf("failure release cannot carry clean-yield fields")
		}
		return s.ReleaseTaskLease(agent, task, token, o.FailureReason)
	case "clean", "external-wait":
		if o.FailureReason != "" || o.EvidenceRef == "" || o.Disposition == "clean" && o.Dependency != "" || o.Disposition == "external-wait" && o.Dependency == "" {
			return fmt.Errorf("task outcome: explicit clean/wait evidence and matching dependency required")
		}
		p, err := s.TaskYieldCapabilities(agent)
		if err != nil {
			return fmt.Errorf("clean yield not negotiated; lease retained: %w", err)
		}
		if p.Version != 1 || !p.CurrentAttemptOnly || !p.AtomicAudit || o.Disposition == "clean" && !p.Clean || o.Disposition == "external-wait" && !p.ExternalWait {
			return fmt.Errorf("clean yield protocol incompatible; lease retained")
		}
		return s.YieldTaskLease(agent, task, token, o.Disposition, o.EvidenceRef, o.Dependency)
	default:
		return fmt.Errorf("task outcome: explicit failure, clean or external-wait required")
	}
}
