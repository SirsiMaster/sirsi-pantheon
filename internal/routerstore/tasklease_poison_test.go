package routerstore

import (
	"testing"
	"time"
)

// leaseOwnership reads the raw ownership columns, bypassing Task so the test
// asserts on stored state rather than on whatever the scanner chooses to expose.
func leaseOwnership(t *testing.T, s *Store, agent, taskID string) (token, expires, claimedBy, threadID string) {
	t.Helper()
	row := s.db.QueryRow(`SELECT lease_token,lease_expires,claimed_by,thread_id FROM tasks WHERE agent=? AND task_id=?;`, agent, taskID)
	if err := row.Scan(&token, &expires, &claimedBy, &threadID); err != nil {
		t.Fatalf("read ownership for %s/%s: %v", agent, taskID, err)
	}
	return token, expires, claimedBy, threadID
}

// poisonTask reproduces the production defect directly in SQL: a non-active row
// that still carries lease ownership. It is written as a raw UPDATE on purpose —
// the point of the fix is that no supported API can produce this state anymore,
// so the regression test must forge it rather than reach it through the store.
func poisonTask(t *testing.T, s *Store, agent, taskID, status, token, expires string) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE tasks SET status=?,lease_token=?,lease_expires=?,claimed_by='dead-worker',thread_id='dead-thread' WHERE agent=? AND task_id=?;`,
		status, token, expires, agent, taskID); err != nil {
		t.Fatalf("poison %s/%s: %v", agent, taskID, err)
	}
}

// TestClaimRepairsLeasePoisonedNonActiveTask is the production incident:
// pantheon-serialized-binary-installer held lease 038b5d77… expiring
// 2026-08-06T07:40:25Z, was reconciled blocked -> pending with ownership
// retained, and became permanently unclaimable — claim-id returned ErrNoWork
// while renew/release rejected the stale token, and doctor --fix did not repair
// it. The row was excluded from lease reclaim (WHERE status='in-progress') and
// from claimableTaskPredicate (empty lease_token) at the same time.
func TestClaimRepairsLeasePoisonedNonActiveTask(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "P1", Subject: "runtime"}); err != nil {
		t.Fatal(err)
	}
	// Shaped like a lease token but deliberately not hex: the real incident token
	// (038b5d77…) trips gitleaks' generic-api-key rule. A lease token is an
	// ownership fence, not a credential, but there is no value in committing one.
	poisonTask(t, s, "codex-home", "P1", "pending", "poisoned-lease-token-fixture", "2026-08-06T07:40:25Z")

	// Pre-condition: the forged row is genuinely unclaimable before the fix.
	token, _, _, _ := leaseOwnership(t, s, "codex-home", "P1")
	if token == "" {
		t.Fatal("test did not actually poison the row")
	}

	lease, err := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute)
	if err != nil {
		t.Fatalf("poisoned task must be reclaimable after repair, got %v", err)
	}
	if lease.TaskID != "P1" {
		t.Fatalf("claimed the wrong task: %s", lease.TaskID)
	}
	if lease.Token == "poisoned-lease-token-fixture" {
		t.Fatal("reclaim must mint a fresh token, not reuse the poisoned one")
	}
}

// TestLeasePoisonRepairPreservesStatus pins the ledger-integrity half: repair
// clears ownership but must not rewrite the disposition reconciliation gave the
// row. Clearing a lease is a truth correction; changing a status would be
// falsifying history.
func TestLeasePoisonRepairPreservesStatus(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "K1", Subject: "keeps status"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "K2", Subject: "claimable"}); err != nil {
		t.Fatal(err)
	}
	// blocked is not claimable, so the repair pass runs but K1 is not selected.
	poisonTask(t, s, "codex-home", "K1", "blocked", "stale-token", "2026-08-06T07:00:00Z")

	if _, err := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute); err != nil {
		t.Fatalf("claim of the healthy task failed: %v", err)
	}

	got, err := s.GetTask("codex-home", "K1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "blocked" {
		t.Fatalf("repair must not change status, got %q", got.Status)
	}
	token, expires, claimedBy, threadID := leaseOwnership(t, s, "codex-home", "K1")
	if token != "" || expires != "" || claimedBy != "" || threadID != "" {
		t.Fatalf("ownership not cleared: token=%q expires=%q by=%q thread=%q", token, expires, claimedBy, threadID)
	}
}

// TestUpdateTaskClearsLeaseOwnership is the entry-point half: stop new poison
// being created. A row that UpdateTask leaves non-active must not retain
// ownership fields, whatever state it arrived in.
func TestUpdateTaskClearsLeaseOwnership(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "U1", Subject: "admin move"}); err != nil {
		t.Fatal(err)
	}
	poisonTask(t, s, "codex-home", "U1", "blocked", "stale-token", "2026-08-06T07:00:00Z")

	if _, err := s.UpdateTask("codex-home", "U1", TaskUpdate{Status: "pending"}); err != nil {
		t.Fatalf("blocked -> pending is a legal administrative transition: %v", err)
	}

	token, expires, claimedBy, threadID := leaseOwnership(t, s, "codex-home", "U1")
	if token != "" || expires != "" || claimedBy != "" || threadID != "" {
		t.Fatalf("UpdateTask left ownership behind: token=%q expires=%q by=%q thread=%q", token, expires, claimedBy, threadID)
	}
}

// TestUpdateTaskDoesNotStripActiveLease is the guard on the guard: an in-progress
// row holds a legitimate lease and a non-status edit must not strip it, or the
// fix would silently un-fence live work.
func TestUpdateTaskDoesNotStripActiveLease(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "A1", Subject: "active"}); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpdateTask("codex-home", "A1", TaskUpdate{Phase: "Runtime"}); err != nil {
		t.Fatalf("non-status edit of an active task must be allowed: %v", err)
	}

	token, _, _, _ := leaseOwnership(t, s, "codex-home", "A1")
	if token != lease.Token {
		t.Fatalf("active lease was stripped: got %q want %q", token, lease.Token)
	}
	if err := s.RenewTaskLease("codex-home", "A1", lease.Token, time.Minute); err != nil {
		t.Fatalf("lease must still renew after an unrelated edit: %v", err)
	}
}

// TestExpiredInProgressStillReclaims guards the pre-existing expiry path against
// regression from the new repair statement — the two must not interfere.
func TestExpiredInProgressStillReclaims(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "E1", Subject: "expires"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)

	second, err := s.ClaimNextTask("codex-home", "worker-2", "thread-2", time.Minute)
	if err != nil {
		t.Fatalf("expired lease must be reclaimable: %v", err)
	}
	if second.Token == first.Token {
		t.Fatal("reclaim must mint a fresh token")
	}
	if err := s.RenewTaskLease("codex-home", "E1", first.Token, time.Minute); err == nil {
		t.Fatal("the fenced-out worker must not be able to renew")
	}
	if err := s.CompleteTaskLease("codex-home", "E1", first.Token, "proof://E1"); err == nil {
		t.Fatal("the fenced-out worker must not be able to complete")
	}
}
