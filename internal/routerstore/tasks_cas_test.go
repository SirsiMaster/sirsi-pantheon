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

// TestUpdateTaskRelabelsOrphanedInProgressWithoutLease is rs-43: a task stuck
// at status "in-progress" with no live lease (expired, or never fenced — the
// exact state hit live on codex-pantheon/inbox-20260927-134144-11a1 and
// fw-lead-overlap-094238, both unclaimable because their own dependency was
// unresolved, not because anyone held them) could never have its status label
// corrected — update refused unconditionally on t.Status=="in-progress", even
// though nobody held a lease to protect. Exiting in-progress for a
// non-executable status (blocked/pending) must succeed once the lease is
// provably not live, and must leave no stale lease columns behind.
func TestUpdateTaskRelabelsOrphanedInProgressWithoutLease(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "ra", TaskID: "O1", Subject: "orphaned"}); err != nil {
		t.Fatal(err)
	}
	// Forge the exact poison: status=in-progress, lease already expired.
	poisonTask(t, s, "ra", "O1", "in-progress", "dead-token", now.Add(-time.Hour).Format(time.RFC3339))

	got, err := s.UpdateTask("ra", "O1", TaskUpdate{Status: "blocked", Subject: "dependency never resolved"})
	if err != nil {
		t.Fatalf("relabel of an orphaned in-progress task must succeed without a lease: %v", err)
	}
	if got.Status != "blocked" {
		t.Fatalf("status did not land: %+v", got)
	}
	token, expires, claimedBy, threadID := leaseOwnership(t, s, "ra", "O1")
	if token != "" || expires != "" || claimedBy != "" || threadID != "" {
		t.Fatalf("stale lease columns survived relabel: token=%q expires=%q claimedBy=%q thread=%q", token, expires, claimedBy, threadID)
	}
}

// TestUpdateTaskRefusesRelabelOfLiveLeasedTask is the negative control for the
// fix above: a task actually held by a LIVE lease must still refuse a plain
// relabel — only ReleaseTaskLease (which requires the token) may move it.
func TestUpdateTaskRefusesRelabelOfLiveLeasedTask(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "ra", TaskID: "L1", Subject: "held"}); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNextTask("ra", "worker-1", "thread-1", time.Minute)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	_, err = s.UpdateTask("ra", "L1", TaskUpdate{Status: "blocked"})
	if err == nil {
		t.Fatal("relabel of a live-leased task must be refused")
	}

	token, _, claimedBy, threadID := leaseOwnership(t, s, "ra", "L1")
	if token != lease.Token || claimedBy != "worker-1" || threadID != "thread-1" {
		t.Fatalf("refused relabel must not touch ownership: token=%q claimedBy=%q thread=%q", token, claimedBy, threadID)
	}
}
