package routerstore

import (
	"testing"
	"time"
)

// tasks.first_claimed_at (schema v28) is the durable "was started" witness the
// recipient-halt stalled count reads. ResetTaskAttempts zeroes attempts, so
// attempts alone forgot that a task had ever been started.

func ageTask(t *testing.T, s *SQLiteStore, agent, id string) {
	t.Helper()
	old := time.Now().UTC().Add(-RecipientStallAfter - time.Hour).Format(time.RFC3339)
	if _, err := s.exec(`UPDATE tasks SET updated=? WHERE agent=? AND task_id=?;`, old, agent, id); err != nil {
		t.Fatal(err)
	}
}

func firstClaimedAt(t *testing.T, s *SQLiteStore, agent, id string) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow(`SELECT first_claimed_at FROM tasks WHERE agent=? AND task_id=?;`, agent, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func stalledOf(t *testing.T, s *SQLiteStore, agent string) int {
	t.Helper()
	l, err := s.RecipientLoad(agent)
	if err != nil {
		t.Fatal(err)
	}
	return l.Stalled
}

// A task claimed, released and then reset reads attempts=0 and status pending,
// yet was started: it stays counted. Reverting the predicate to
// (status='in-progress' OR attempts>0) makes this fail.
func TestResetAttemptsKeepsStartedTaskCounted(t *testing.T) {
	s := openTestStore(t)
	const a = "lane"
	if err := s.AddTask(Task{Agent: a, TaskID: "started", Subject: "s"}); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimTask(a, "started", "w", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseTaskLease(a, "started", lease.Token, "left"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetTaskAttempts(a, "started"); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := s.db.QueryRow(`SELECT attempts FROM tasks WHERE agent=? AND task_id='started';`, a).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("attempts after reset = %d (err %v), want 0", attempts, err)
	}
	if firstClaimedAt(t, s, a, "started") == "" {
		t.Fatal("reset-attempts cleared first_claimed_at")
	}
	ageTask(t, s, a, "started")
	if got := stalledOf(t, s, a); got != 1 {
		t.Fatalf("stalled = %d, want 1 (reset must not erase the started witness)", got)
	}
}

// Never claimed stays uncounted, however old.
func TestNeverClaimedTaskStaysUncounted(t *testing.T) {
	s := openTestStore(t)
	const a = "lane"
	if err := s.AddTask(Task{Agent: a, TaskID: "planned", Subject: "p"}); err != nil {
		t.Fatal(err)
	}
	ageTask(t, s, a, "planned")
	if v := firstClaimedAt(t, s, a, "planned"); v != "" {
		t.Fatalf("first_claimed_at = %q before any claim", v)
	}
	if got := stalledOf(t, s, a); got != 0 {
		t.Fatalf("stalled = %d, want 0", got)
	}
}

// Pre-v28 rows have no timestamp (no backfill): attempts>0 still counts them.
func TestPreMigrationStartedRowStillCountedByAttempts(t *testing.T) {
	s := openTestStore(t)
	const a = "lane"
	seedRecipientTasks(t, s, a, "old", "pending", 1, 2, time.Now().UTC().Add(-RecipientStallAfter-time.Hour), "")
	if got := stalledOf(t, s, a); got != 1 {
		t.Fatalf("stalled = %d, want 1 (attempts>0 witness kept for pre-v28 rows)", got)
	}
}

// The first claim stamps; a later claim never moves it.
func TestSecondClaimDoesNotMoveFirstClaimedAt(t *testing.T) {
	s := openTestStore(t)
	const a = "lane"
	if err := s.AddTask(Task{Agent: a, TaskID: "t", Subject: "t"}); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimTask(a, "t", "w", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if firstClaimedAt(t, s, a, "t") == "" {
		t.Fatal("first claim did not set first_claimed_at")
	}
	const marker = "2020-01-01T00:00:00Z"
	if _, err := s.exec(`UPDATE tasks SET first_claimed_at=? WHERE agent=? AND task_id='t';`, marker, a); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseTaskLease(a, "t", lease.Token, "again"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask(a, "t", "w", "thr", time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := firstClaimedAt(t, s, a, "t"); got != marker {
		t.Fatalf("second claim moved first_claimed_at to %q, want %q", got, marker)
	}
}
