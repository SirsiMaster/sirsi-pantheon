package routerstore

import (
	"testing"
	"time"
)

func TestReclaimExpiredTaskLeasesCrossesAgentBoundaries(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	for _, task := range []Task{
		{Agent: "codex-nexus", TaskID: "orphaned", Subject: "orphaned"},
		{Agent: "claude-deck", TaskID: "orphaned", Subject: "orphaned"},
	} {
		if err := s.AddTask(task); err != nil {
			t.Fatal(err)
		}
	}
	// Both claimed by a session that then vanished — never renewed, never
	// completed, never released. Neither agent has come back to claim again,
	// so the per-claim reclaim (scoped to the calling agent) never fires.
	if _, err := s.ClaimTask("codex-nexus", "orphaned", "worker", "thread", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask("claude-deck", "orphaned", "worker", "thread", time.Minute); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour) // both leases now long expired

	found, err := s.ReclaimExpiredTaskLeases(true /* dry run */)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("dry run found %d expired leases, want 2: %+v", len(found), found)
	}
	// Dry run must not write.
	if token, _, claimedBy, _ := leaseOwnership(t, s, "codex-nexus", "orphaned"); token == "" || claimedBy == "" {
		t.Fatalf("dry run mutated state: token=%q claimed_by=%q", token, claimedBy)
	}

	found, err = s.ReclaimExpiredTaskLeases(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("live run found %d expired leases, want 2: %+v", len(found), found)
	}

	for _, agent := range []string{"codex-nexus", "claude-deck"} {
		got, err := s.GetTask(agent, "orphaned")
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "in-progress" {
			t.Fatalf("%s: status changed to %q, want unchanged in-progress (not a verdict rewrite)", agent, got.Status)
		}
		if token, _, claimedBy, threadID := leaseOwnership(t, s, agent, "orphaned"); token != "" || claimedBy != "" || threadID != "" {
			t.Fatalf("%s: lease ownership not cleared: token=%q claimed_by=%q thread_id=%q", agent, token, claimedBy, threadID)
		}
		// Now claimable again by anyone, including a different worker.
		if _, err := s.ClaimTask(agent, "orphaned", "new-worker", "new-thread", time.Minute); err != nil {
			t.Fatalf("%s: still not claimable after reclaim: %v", agent, err)
		}
	}
}

func TestReclaimExpiredTaskLeasesLeavesLiveLeaseUntouched(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "active", Subject: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask("codex-home", "active", "worker", "thread", 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute) // lease still well within its TTL

	found, err := s.ReclaimExpiredTaskLeases(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("reclaimed a live, unexpired lease: %+v", found)
	}
	if token, _, claimedBy, _ := leaseOwnership(t, s, "codex-home", "active"); token == "" || claimedBy == "" {
		t.Fatalf("live lease ownership was cleared: token=%q claimed_by=%q", token, claimedBy)
	}
}

func TestReclaimExpiredTaskLeasesBlocksExhaustedAttempts(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "flaky", Subject: "flaky"}); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= MaxRetriesPerItem; attempt++ {
		if _, err := s.ClaimTask("codex-home", "flaky", "worker", "thread", time.Minute); err != nil {
			t.Fatalf("attempt %d claim: %v", attempt, err)
		}
		now = now.Add(2 * time.Minute)
	}

	found, err := s.ReclaimExpiredTaskLeases(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || !found[0].Blocked {
		t.Fatalf("expected one blocked reclaim, got %+v", found)
	}
	got, err := s.GetTask("codex-home", "flaky")
	if err != nil || got.Status != "blocked" {
		t.Fatalf("exhausted task not blocked: %+v err=%v", got, err)
	}
}
