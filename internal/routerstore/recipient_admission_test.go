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

func seedTasks(t *testing.T, s *SQLiteStore, agent string, n int, updated time.Time, leaseExpires string) {
	t.Helper()
	ts := updated.Format(time.RFC3339)
	for i := 0; i < n; i++ {
		if _, err := s.exec(`INSERT INTO tasks (agent, task_id, subject, status, responsible_party, created, updated, lease_expires)
			VALUES (?, ?, 'fixture', 'pending', ?, ?, ?, ?);`, agent, fmt.Sprintf("t-%s-%03d", leaseExpires, i), agent, ts, ts, leaseExpires); err != nil {
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
		{"stalled tasks at the limit refuse", "ra", func(t *testing.T, s *SQLiteStore) { seedTasks(t, s, "lane", 5, stale, "") }, true, true},
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
