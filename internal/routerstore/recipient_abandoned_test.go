package routerstore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Owner decision 2026-11-11 ("count only abandoned work"): the recipient's
// stalled count is tasks someone STARTED and then left for 24h. A planned
// backlog nobody started never halts a lane. These run at the production
// limits (warn 50, halt 99), not the tightened ones.

// seedRecipientTasks inserts n tasks with an explicit status, attempts, lease and age.
func seedRecipientTasks(t *testing.T, s *SQLiteStore, agent, prefix, status string, n, attempts int, updated time.Time, leaseExpires string) {
	t.Helper()
	ts := updated.Format(time.RFC3339)
	for i := 0; i < n; i++ {
		if _, err := s.exec(`INSERT INTO tasks (agent, task_id, subject, status, responsible_party, created, updated, lease_expires, attempts)
			VALUES (?, ?, 'fixture', ?, ?, ?, ?, ?, ?);`, agent, fmt.Sprintf("%s-%03d", prefix, i), status, agent, ts, ts, leaseExpires, attempts); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecipientHaltCountsAbandonedWorkOnly(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-RecipientStallAfter - time.Hour)
	future := now.Add(time.Hour).Format(time.RFC3339)
	past := now.Add(-time.Hour).Format(time.RFC3339)
	const ssa = "sirsi-software-admin"
	for _, tc := range []struct {
		name        string
		from, to    string
		seed        func(*testing.T, *SQLiteStore)
		wantStalled int
		wantUnacked int
		refused     bool
		wantWarn    bool
	}{
		{"live incident: 150 never-claimed pending tasks do not halt", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedRecipientTasks(t, s, "lane", "p", "pending", 150, 0, stale, "")
			}, 0, 0, false, false},
		{"99 started then left 24h: refused", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, "lane", "a", "pending", 99, 1, stale, "") }, 99, 0, true, true},
		{"98 started then left 24h: accepted with the warn band", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, "lane", "a", "pending", 98, 1, stale, "") }, 98, 0, false, true},
		{"started, lease expired, untouched 24h (in-progress, no live lease) counts", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedRecipientTasks(t, s, "lane", "a", "in-progress", 99, 1, stale, past)
			}, 99, 0, true, true},
		{"in-progress with no claim counter still counts as started", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedRecipientTasks(t, s, "lane", "a", "in-progress", 99, 0, stale, "")
			}, 99, 0, true, true},
		{"started task with a LIVE lease does not count", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedRecipientTasks(t, s, "lane", "a", "in-progress", 120, 1, stale, future)
			}, 0, 0, false, false},
		{"started task touched within 24h does not count", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, "lane", "a", "pending", 120, 1, now, "") }, 0, 0, false, false},
		{"started-then-blocked untouched 24h counts", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, "lane", "b", "blocked", 99, 2, stale, "") }, 99, 0, true, true},
		{"never-started blocked does not count", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedRecipientTasks(t, s, "lane", "b", "blocked", 150, 0, stale, "")
			}, 0, 0, false, false},
		{"done, canceled and withdrawn do not count", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedRecipientTasks(t, s, "lane", "d", "done", 50, 1, stale, "")
				seedRecipientTasks(t, s, "lane", "c", "canceled", 50, 1, stale, "")
				seedRecipientTasks(t, s, "lane", "w", "withdrawn", 50, 1, stale, "")
			}, 0, 0, false, false},
		{"unacked half unchanged: 99 unacked refuses with zero stalled", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) {
				seedOpen(t, s, "lane", 99)
				seedRecipientTasks(t, s, "lane", "p", "pending", 150, 0, stale, "")
			}, 0, 99, true, true},
		{"unacked warn band at 50 with zero stalled", "ra", "lane",
			func(t *testing.T, s *SQLiteStore) { seedOpen(t, s, "lane", 50) }, 0, 50, false, true},
		{"router-internal sender exempt from a halted lane", "routerstore", "lane",
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, "lane", "a", "pending", 99, 1, stale, "") }, 99, 0, false, true},
		{"ra reaches a halted SSA", "ra", ssa,
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, ssa, "a", "pending", 99, 1, stale, "") }, 99, 0, false, true},
		{"another sender is still refused at a halted SSA", "mercury", ssa,
			func(t *testing.T, s *SQLiteStore) { seedRecipientTasks(t, s, ssa, "a", "pending", 99, 1, stale, "") }, 99, 0, true, true},
	} {
		for _, path := range []string{"guarded", "raw"} {
			t.Run(tc.name+"/"+path, func(t *testing.T) {
				s := openTestStore(t)
				tc.seed(t, s)
				load, err := s.RecipientLoad(tc.to)
				if err != nil || load.Stalled != tc.wantStalled || load.Unacked != tc.wantUnacked {
					t.Fatalf("load=%+v err=%v, want %d unacked / %d stalled", load, err, tc.wantUnacked, tc.wantStalled)
				}
				if got := load.Warning(tc.to); (got != "") != tc.wantWarn {
					t.Fatalf("warning=%q, want warn=%v", got, tc.wantWarn)
				} else if got != "" && !strings.Contains(got, "started tasks untouched for 24h") {
					t.Fatalf("warning must say started and untouched for 24h, got %q", got)
				}
				before, err := s.Inbox(tc.to)
				if err != nil {
					t.Fatal(err)
				}
				if path == "guarded" {
					_, _, err = s.SendGuarded(SendReq{From: tc.from, To: tc.to, Title: "new work", Type: "proposal"})
				} else {
					_, err = s.Send(tc.from, tc.to, "new work", "proposal", "x")
				}
				if errors.Is(err, ErrRecipientSaturated) != tc.refused || (!tc.refused && err != nil) {
					t.Fatalf("refused=%v, got err %v", tc.refused, err)
				}
				after, err := s.Inbox(tc.to)
				if err != nil {
					t.Fatal(err)
				}
				if grew := len(after) - len(before); tc.refused && grew != 0 || !tc.refused && grew != 1 {
					t.Fatalf("refused=%v but inbox grew by %d", tc.refused, grew)
				}
			})
		}
	}
}

// The real lifecycle, not seeded columns: claim then release (or let the lease
// lapse) leaves a started task that counts after 24h; a never-claimed sibling
// does not; ResetTaskAttempts erases the only evidence of the start, so the
// reset task reads as planned again (the documented blind spot).
func TestRecipientStalledFollowsRealClaimLifecycle(t *testing.T) {
	s := openTestStore(t)
	t0 := time.Date(2026, time.November, 10, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return t0 }
	seedRecipientTasks(t, s, "lane", "started", "pending", 2, 0, t0, "")
	seedRecipientTasks(t, s, "lane", "planned", "pending", 5, 0, t0, "")

	rel, err := s.ClaimTask("lane", "started-000", "w", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseTaskLease("lane", "started-000", rel.Token, "dropped"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimTask("lane", "started-001", "w", "thr", time.Minute); err != nil { // lease lapses, never released
		t.Fatal(err)
	}

	count := func(at time.Time) int {
		s.now = func() time.Time { return at }
		l, lerr := s.RecipientLoad("lane")
		if lerr != nil {
			t.Fatal(lerr)
		}
		return l.Stalled
	}
	if got := count(t0.Add(2 * time.Hour)); got != 0 {
		t.Fatalf("started tasks touched 2h ago must not count yet, got %d", got)
	}
	if got := count(t0.Add(RecipientStallAfter + time.Hour)); got != 2 {
		t.Fatalf("released + lapsed started tasks untouched 24h must count 2 (5 planned must not), got %d", got)
	}
	s.now = func() time.Time { return t0.Add(RecipientStallAfter + time.Hour) }
	if err = s.ResetTaskAttempts("lane", "started-000"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.exec(`UPDATE tasks SET updated=? WHERE agent='lane' AND task_id='started-000';`, t0.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if got := count(t0.Add(RecipientStallAfter + time.Hour)); got != 1 {
		t.Fatalf("a reset task reads as never started (known blind spot): want 1, got %d", got)
	}
}
