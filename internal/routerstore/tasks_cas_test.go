package routerstore

import (
	"errors"
	"testing"
	"time"
)

// TestUpdateTaskRefusesStaleWriteAfterConcurrentClaim is the lost-fence race
// codex-home found post-merge on PR #553.
//
// UpdateTask reads the row, decides from that read whether the row may shed
// lease ownership, then writes. A ClaimTask landing in between installs a valid
// fenced lease and flips the row to in-progress. Before the compare-and-swap,
// the stale write would land a pending status AND clear the newly valid
// ownership fields, silently un-fencing live work — a worse failure than the
// lease poison the clearing exists to prevent.
//
// The interleaving is forced through afterTaskReadHook rather than raced with
// goroutines, so this test fails deterministically on a regression instead of
// flaking.
func TestUpdateTaskRefusesStaleWriteAfterConcurrentClaim(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "C1", Subject: "raced"}); err != nil {
		t.Fatal(err)
	}

	var lease *TaskLease
	afterTaskReadHook = func() {
		afterTaskReadHook = nil // fire exactly once
		claimed, err := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute)
		if err != nil {
			t.Fatalf("concurrent claim failed to set up the race: %v", err)
		}
		lease = claimed
	}
	t.Cleanup(func() { afterTaskReadHook = nil })

	_, err := s.UpdateTask("codex-home", "C1", TaskUpdate{Phase: "Canon"})
	if !errors.Is(err, ErrConcurrentTaskUpdate) {
		t.Fatalf("stale write must be refused, got %v", err)
	}

	if lease == nil {
		t.Fatal("hook did not run — the race was never set up")
	}
	token, _, claimedBy, threadID := leaseOwnership(t, s, "codex-home", "C1")
	if token != lease.Token {
		t.Fatalf("live fenced lease was stripped: got %q want %q", token, lease.Token)
	}
	if claimedBy != "worker-1" || threadID != "thread-1" {
		t.Fatalf("ownership clobbered: by=%q thread=%q", claimedBy, threadID)
	}

	got, err := s.GetTask("codex-home", "C1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "in-progress" {
		t.Fatalf("stale status landed: %q", got.Status)
	}
	if err := s.RenewTaskLease("codex-home", "C1", lease.Token, time.Minute); err != nil {
		t.Fatalf("the claim holder must still be able to renew: %v", err)
	}
}

// TestUpdateTaskSucceedsWhenStatusUnchanged is the guard on the guard: the CAS
// must not reject ordinary updates, or it would break every non-raced caller.
func TestUpdateTaskSucceedsWhenStatusUnchanged(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "N1", Subject: "normal"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateTask("codex-home", "N1", TaskUpdate{Phase: "Canon", Status: "blocked"})
	if err != nil {
		t.Fatalf("uncontended update must succeed: %v", err)
	}
	if got.Status != "blocked" || got.Phase != "Canon" {
		t.Fatalf("update did not land: %+v", got)
	}
}
