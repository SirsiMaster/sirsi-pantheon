package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TaskEligibilityScopeRow           = "task-row"
	TaskEligibilityCallerNotEvaluated = "not-evaluated"
)

// TaskEligibility is a read-only task-row diagnosis, not a caller/session claim
// authorization preview. It exposes holder, expiry, attempts and failure reason,
// NEVER the lease token. Reasons describe row conditions only. Empty scope or
// caller-evaluation metadata from an older service means unknown; it must not
// be treated as a caller verdict.
type TaskEligibility struct {
	Scope            string   `json:"scope"`
	CallerEvaluation string   `json:"caller_evaluation"`
	Agent            string   `json:"agent"`
	TaskID           string   `json:"task_id"`
	Status           string   `json:"status"`
	ResponsibleParty string   `json:"responsible_party"`
	BlockedBy        string   `json:"blocked_by"`
	BlockedByState   string   `json:"blocked_by_state"` // "", "task:<status>", "task:missing" or "external-reason"
	ClaimedBy        string   `json:"claimed_by"`
	ThreadID         string   `json:"thread_id"`
	LeaseHeld        bool     `json:"lease_held"`
	LeaseExpires     string   `json:"lease_expires"`
	LeaseExpired     bool     `json:"lease_expired"` // held but past expiry; the next claim pass reconciles it as a failed attempt
	Attempts         int      `json:"attempts"`
	MaxAttempts      int      `json:"max_attempts"`
	FailureReason    string   `json:"failure_reason"`
	Claimable        bool     `json:"claimable"`    // row predicate only; no caller/session evaluation
	Dispatchable     bool     `json:"dispatchable"` // row predicate plus responsible party; not execution permission
	Reasons          []string `json:"reasons"`
}

// TaskEligibility has no caller/session context. It reads row predicates without
// changing anything; service authorization and actual claim guards are separate.
func (s *SQLiteStore) TaskEligibility(agent, taskID string) (TaskEligibility, error) {
	agent, taskID = strings.TrimSpace(agent), strings.TrimSpace(taskID)
	e := TaskEligibility{Scope: TaskEligibilityScopeRow, CallerEvaluation: TaskEligibilityCallerNotEvaluated,
		Agent: agent, TaskID: taskID, MaxAttempts: MaxRetriesPerItem}
	var leaseToken string
	err := s.db.QueryRow(`SELECT status,responsible_party,blocked_by,claimed_by,thread_id,lease_token,lease_expires,attempts,failure_reason
		FROM tasks WHERE agent=? AND task_id=?;`, agent, taskID).
		Scan(&e.Status, &e.ResponsibleParty, &e.BlockedBy, &e.ClaimedBy, &e.ThreadID, &leaseToken, &e.LeaseExpires, &e.Attempts, &e.FailureReason)
	if errors.Is(err, sql.ErrNoRows) {
		// Wrap the sentinel so the service returns a named not-found, not an HTTP 500.
		return TaskEligibility{}, fmt.Errorf("%w: task %s/%s", ErrNotFound, agent, taskID)
	}
	if err != nil {
		return TaskEligibility{}, fmt.Errorf("routerstore: task eligibility: %w", err)
	}
	now := s.clock().UTC()
	e.LeaseHeld = leaseToken != ""
	if e.LeaseHeld && e.LeaseExpires != "" {
		if t, perr := time.Parse(time.RFC3339, e.LeaseExpires); perr == nil && !t.After(now) {
			e.LeaseExpired = true
		}
	}

	depDone := true
	if e.BlockedBy != "" {
		var depStatus string
		derr := s.db.QueryRow(`SELECT status FROM tasks WHERE agent=? AND task_id=?;`, agent, e.BlockedBy).Scan(&depStatus)
		switch {
		case errors.Is(derr, sql.ErrNoRows):
			e.BlockedByState = "external-reason" // not a task id: a free-text reason, cleared by hand
			depDone = false
		case derr != nil:
			return TaskEligibility{}, fmt.Errorf("routerstore: task eligibility dependency: %w", derr)
		default:
			e.BlockedByState = "task:" + depStatus
			depDone = depStatus == "done"
		}
	}

	switch e.Status {
	case "done":
		e.Reasons = append(e.Reasons, "task is done")
	case "blocked":
		e.Reasons = append(e.Reasons, "status is blocked (claim only takes pending or in-progress)")
	}
	if e.BlockedBy != "" && !depDone {
		e.Reasons = append(e.Reasons, fmt.Sprintf("blocked_by %q is unresolved (%s); clear it with `task update --blocked-by \"\"` (needs no lease) once true", e.BlockedBy, e.BlockedByState))
	}
	if e.LeaseHeld && !e.LeaseExpired {
		e.Reasons = append(e.Reasons, fmt.Sprintf("a live lease is held by %q (thread %s) until %s", e.ClaimedBy, e.ThreadID, e.LeaseExpires))
	}
	if e.LeaseHeld && e.LeaseExpired {
		e.Reasons = append(e.Reasons, fmt.Sprintf("an EXPIRED lease from %q (%s) is still recorded; the next claim pass reclaims it and counts it as a failed attempt", e.ClaimedBy, e.LeaseExpires))
	}
	if e.Attempts >= MaxRetriesPerItem {
		e.Reasons = append(e.Reasons, fmt.Sprintf("attempts %d/%d: retry ceiling reached; after fixing the cause inspect `task resume-claim` for an evidence-bound history-preserving recovery lease (requires compatible service)", e.Attempts, MaxRetriesPerItem))
	}
	if e.FailureReason != "" {
		e.Reasons = append(e.Reasons, "last failure: "+e.FailureReason)
	}
	e.Claimable = (e.Status == "pending" || e.Status == "in-progress") && depDone && !e.LeaseHeld && e.Attempts < MaxRetriesPerItem
	e.Dispatchable = e.Claimable && (e.ResponsibleParty == "" || e.ResponsibleParty == "self" || e.ResponsibleParty == e.Agent)
	if e.Claimable && !e.Dispatchable {
		e.Reasons = append(e.Reasons, fmt.Sprintf("task-row is claimable, but responsible_party is %q: a wake loop will not start this lane's worker for it", e.ResponsibleParty))
	}
	if len(e.Reasons) == 0 {
		e.Reasons = []string{"row predicates only; caller/session eligibility is not evaluated"}
	}
	return e, nil
}
