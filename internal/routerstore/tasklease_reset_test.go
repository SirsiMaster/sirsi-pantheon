package routerstore

import (
	"errors"
	"testing"
	"time"
)

// TestResetTaskAttemptsUnblocksClaimAfterCeiling is the negative control for
// task-retry-ceiling-reset: a row that exhausts MaxRetriesPerItem via
// claim/release cycles was otherwise recoverable only by a hand UPDATE
// against router.db — the same operator-path gap breaker-no-operator-path
// found for tripped circuit breakers. Drives a real store to the ceiling,
// confirms it is unclaimable, resets, confirms it claims again.
//
// Delete ResetTaskAttempts' wiring and the final claim still returns
// ErrNoClaimableTask, so this fails by name.
func TestResetTaskAttemptsUnblocksClaimAfterCeiling(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "flaky", Subject: "flaky"}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < MaxRetriesPerItem; i++ {
		lease, err := s.ClaimTask("codex-home", "flaky", "worker", "thread", time.Minute)
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		if err := s.ReleaseTaskLease("codex-home", "flaky", lease.Token, "recoverable failure"); err != nil {
			t.Fatalf("release %d: %v", i, err)
		}
	}

	if _, err := s.ClaimTask("codex-home", "flaky", "worker", "thread", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
		t.Fatalf("claim at the ceiling = %v, want ErrNoClaimableTask", err)
	}

	if err := s.ResetTaskAttempts("codex-home", "flaky"); err != nil {
		t.Fatalf("ResetTaskAttempts: %v", err)
	}

	lease, err := s.ClaimTask("codex-home", "flaky", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatalf("claim after reset: %v", err)
	}
	if lease.Attempt != 1 {
		t.Fatalf("claim after reset: attempt = %d, want 1 (reset did not zero the counter)", lease.Attempt)
	}
}

// TestResetTaskAttemptsUnknownTask pins the not-found contract the command
// surfaces to the operator, so a typo reports a miss instead of succeeding.
func TestResetTaskAttemptsUnknownTask(t *testing.T) {
	s := newTestStore(t)
	if err := s.ResetTaskAttempts("codex-home", "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResetTaskAttempts on unknown task = %v, want ErrNotFound", err)
	}
}

// TestResetTaskAttemptsLeavesGenuineDependencyBlockAlone is the negative
// control for the status-restore side effect: a row blocked on a real
// dependency (blocked_by set) must NOT be unblocked by resetting attempts —
// only exhaustion-caused blocks (no blocked_by) get the status flip back to
// pending. Without the blocked_by guard this would silently release work
// that is genuinely waiting on something else.
func TestResetTaskAttemptsLeavesGenuineDependencyBlockAlone(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "waiting", Subject: "waiting", Status: "blocked"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask("codex-home", "waiting", TaskUpdate{BlockedBySet: true, BlockedBy: "some-other-task"}); err != nil {
		t.Fatalf("set blocked_by: %v", err)
	}

	if err := s.ResetTaskAttempts("codex-home", "waiting"); err != nil {
		t.Fatalf("ResetTaskAttempts: %v", err)
	}

	got, err := s.GetTask("codex-home", "waiting")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "blocked" {
		t.Fatalf("status after reset = %q, want still 'blocked' (genuine dependency block must survive an attempts reset)", got.Status)
	}
}
