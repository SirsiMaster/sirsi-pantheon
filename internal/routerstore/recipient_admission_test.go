package routerstore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// tightenAdmission shrinks the recipient limits so fixtures stay small.
func tightenAdmission(t *testing.T) {
	t.Helper()
	w, h := RecipientWarnAt, RecipientHaltAt
	RecipientWarnAt, RecipientHaltAt = 3, 5
	t.Cleanup(func() { RecipientWarnAt, RecipientHaltAt = w, h })
}

// seedTasks inserts n pending tasks that were STARTED (attempts=1): the
// recipient "stalled" count only sees started work (owner decision 2026-11-11).
func seedTasks(t *testing.T, s *SQLiteStore, agent string, n int, updated time.Time, leaseExpires string) {
	t.Helper()
	seedTasksAttempts(t, s, agent, n, updated, leaseExpires, 1)
}

func seedTasksAttempts(t *testing.T, s *SQLiteStore, agent string, n int, updated time.Time, leaseExpires string, attempts int) {
	t.Helper()
	ts := updated.Format(time.RFC3339)
	for i := 0; i < n; i++ {
		if _, err := s.exec(`INSERT INTO tasks (agent, task_id, subject, status, responsible_party, created, updated, lease_expires, attempts)
			VALUES (?, ?, 'fixture', 'pending', ?, ?, ?, ?, ?);`, agent, fmt.Sprintf("t-%s-%03d", leaseExpires, i), agent, ts, ts, leaseExpires, attempts); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecipientAdmission(t *testing.T) {
	tightenAdmission(t)
	now := time.Now().UTC()
	stale := now.Add(-RecipientStallAfter - time.Hour)
	future := now.Add(time.Hour).Format(time.RFC3339)
	for _, tc := range []struct {
		name     string
		from     string
		seed     func(*testing.T, *SQLiteStore)
		refused  bool
		wantWarn bool
	}{
		{"quiet lane accepts", "ra", func(*testing.T, *SQLiteStore) {}, false, false},
		{"unacked in warn band accepts with warning", "ra", func(t *testing.T, s *SQLiteStore) { seedOpen(t, s, "lane", 3) }, false, true},
		{"unacked at the limit refuses", "ra", func(t *testing.T, s *SQLiteStore) { seedOpen(t, s, "lane", 5) }, true, true},
		{"acknowledged items do not count", "ra", func(t *testing.T, s *SQLiteStore) {
			seedOpen(t, s, "lane", 8)
			if _, err := s.exec(`UPDATE items SET acked_at = ? WHERE to_agent = 'lane';`, now.Format(time.RFC3339)); err != nil {
				t.Fatal(err)
			}
		}, false, false},
		{"started-then-abandoned tasks at the limit refuse", "ra", func(t *testing.T, s *SQLiteStore) { seedTasks(t, s, "lane", 5, stale, "") }, true, true},
		{"never-started planned tasks do not count", "ra", func(t *testing.T, s *SQLiteStore) { seedTasksAttempts(t, s, "lane", 8, stale, "", 0) }, false, false},
		{"fresh tasks do not count", "ra", func(t *testing.T, s *SQLiteStore) { seedTasks(t, s, "lane", 8, now, "") }, false, false},
		{"live-leased old tasks do not count", "ra", func(t *testing.T, s *SQLiteStore) { seedTasks(t, s, "lane", 8, stale, future) }, false, false},
		{"another lane's backlog does not count", "ra", func(t *testing.T, s *SQLiteStore) { seedOpen(t, s, "other", 8) }, false, false},
		{"router-internal sender is exempt", "routerstore", func(t *testing.T, s *SQLiteStore) { seedOpen(t, s, "lane", 8) }, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			tc.seed(t, s)
			before, err := s.Inbox("lane")
			if err != nil {
				t.Fatal(err)
			}
			id, _, err := s.SendGuarded(SendReq{From: tc.from, To: "lane", Title: "new work"})
			if tc.refused != errors.Is(err, ErrRecipientSaturated) {
				t.Fatalf("refused=%v, got err %v", tc.refused, err)
			}
			if !tc.refused && err != nil {
				t.Fatal(err)
			}
			after, err := s.Inbox("lane")
			if err != nil {
				t.Fatal(err)
			}
			if grew := len(after) - len(before); tc.refused && (grew != 0 || id != "") {
				t.Fatalf("a refused send must append nothing (grew %d, id %q)", grew, id)
			} else if !tc.refused && grew != 1 {
				t.Fatalf("an accepted send must append one item, grew %d", grew)
			}
			load, err := s.RecipientLoad("lane")
			if err != nil {
				t.Fatal(err)
			}
			if got := load.Warning("lane") != ""; got != tc.wantWarn {
				t.Fatalf("warning=%v want %v (load %+v)", got, tc.wantWarn, load)
			}
		})
	}
}

// Terminal history is not recipient backlog. A superseded description alone
// does not cancel pending work, and age/lease guards apply independently.
func TestRecipientAdmissionTerminalTaskAccounting(t *testing.T) {
	tightenAdmission(t)
	now := time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-RecipientStallAfter - time.Hour)
	for _, tc := range []struct {
		name, status, subject, leaseExpires string
		updated                             time.Time
		wantStalled                         int
	}{
		{"aged_canceled", "canceled", "superseded", "", stale, 0},
		{"aged_done", "done", "completed", "", stale, 0},
		{"pending_superseded_description", "pending", "superseded by replacement; not canceled", "", stale, 8},
		{"true_pending_saturation", "pending", "unfinished work", "", stale, 8},
		{"true_blocked_saturation", "blocked", "blocked work", "", stale, 8}, {"active_lease", "in-progress", "live work", now.Add(time.Hour).Format(time.RFC3339), stale, 0},
		{"expired_lease", "in-progress", "unfinished work", now.Add(-time.Hour).Format(time.RFC3339), stale, 8},
		{"fresh_pending", "pending", "new work", "", now, 0},
	} {
		for _, path := range []string{"guarded", "raw"} {
			t.Run(tc.name+"/"+path, func(t *testing.T) {
				s := openTestStore(t)
				s.now = func() time.Time { return tc.updated }
				seedTasks(t, s, "lane", 8, tc.updated, tc.leaseExpires)
				if _, err := s.exec(`UPDATE tasks SET subject=?,status=? WHERE agent='lane';`, tc.subject, "pending"); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 8; i++ {
					id := fmt.Sprintf("t-%s-%03d", tc.leaseExpires, i)
					var err error
					switch tc.status {
					case "canceled":
						err = s.CancelTask("lane", id, "board:fixture", "superseded")
					case "done":
						err = s.CompleteTaskByOwner("lane", id, "board:fixture", "fixture completion")
					default:
						_, err = s.exec(`UPDATE tasks SET status=? WHERE agent='lane' AND task_id=?;`, tc.status, id)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if tc.status == "in-progress" {
					if _, err := s.exec(`UPDATE tasks SET lease_token='fixture-lease',claimed_by='fixture-worker',thread_id='fixture-thread' WHERE agent='lane';`); err != nil {
						t.Fatal(err)
					}
				}
				s.now = func() time.Time { return now }
				load, err := s.RecipientLoad("lane")
				if err != nil || load.Unacked != 0 || load.Stalled != tc.wantStalled {
					t.Fatalf("load=%+v err=%v, want 0 unacked/%d stalled", load, err, tc.wantStalled)
				}
				var id string
				if path == "guarded" {
					id, _, err = s.SendGuarded(SendReq{From: "fixture-sender", To: "lane", Title: "new work", Type: "proposal"})
				} else {
					id, err = s.Send("fixture-sender", "lane", "new work", "proposal", "fixture")
				}
				refused := tc.wantStalled >= RecipientHaltAt
				if errors.Is(err, ErrRecipientSaturated) != refused || (!refused && err != nil) {
					t.Fatalf("refused=%v, got %v", refused, err)
				}
				items, err := s.Inbox("lane")
				if err != nil {
					t.Fatal(err)
				}
				if refused && (id != "" || len(items) != 0) || !refused && (id == "" || len(items) != 1) {
					t.Fatalf("refused=%v id=%q inbox=%d", refused, id, len(items))
				}
			})
		}
	}
}

// TestRecipientSaturationIsOneEscalation: repeated refusals bump one singleton,
// never a flood of escalations (§2b axiom 5).
func TestRecipientSaturationIsOneEscalation(t *testing.T) {
	tightenAdmission(t)
	s := openTestStore(t)
	seedOpen(t, s, "lane", 5)
	for i := 0; i < 4; i++ {
		_, _, err := s.SendGuarded(SendReq{From: "ra", To: "lane", Title: fmt.Sprintf("retry %d", i)})
		if !errors.Is(err, ErrRecipientSaturated) || !strings.Contains(err.Error(), "lane has 5 unacknowledged") {
			t.Fatalf("send %d: want a saturation refusal naming the lane and counts, got %v", i, err)
		}
	}
	var rows, occurrences int
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(occurrences), 0) FROM items
		WHERE source_item = 'saturated:lane' AND failure_class = 'recipient_saturated';`).Scan(&rows, &occurrences); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || occurrences != 4 {
		t.Fatalf("want one escalation seen 4 times, got %d rows / %d occurrences", rows, occurrences)
	}
}
