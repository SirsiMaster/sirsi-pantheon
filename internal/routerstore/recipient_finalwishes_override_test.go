package routerstore

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// FinalWishes seven-day override (owner decision 2026-11-11): the four lanes may
// send to one another past the recipient limit until 2026-11-18T05:50:00Z.
// Delete with the block in facade.go after 2026-11-18.

func TestFinalWishesOverride(t *testing.T) {
	set := []string{"claude-finalwishes-m1", "claude-finalwishes-m5", "codex-finalwishes", "sirsi-software-admin"}
	before := finalWishesOverrideEnds.Add(-time.Hour)
	type pair struct{ from, to string }
	var in []pair
	for _, f := range set {
		for _, to := range set {
			if f != to {
				in = append(in, pair{f, to})
			}
		}
	}
	if len(in) != 12 {
		t.Fatalf("want 12 ordered pairs, got %d", len(in))
	}
	out := []pair{
		{"claude-finalwishes-m5", "some-other-lane"},        // member to non-member
		{"some-other-lane", "codex-finalwishes"},            // non-member to member
		{"claude-finalwishes-web", "codex-finalwishes"},     // web lane is not in the set
		{"codex-finalwishes", "codex-finalwishes-web"},      // nor is it a valid recipient
		{"mercury", "sirsi-software-admin"},                 // ordinary sender to a member
		{"claude-finalwishes-m1", "claude-finalwishes-web"}, // member to the web lane
	}
	saturations := map[string]func(*testing.T, *SQLiteStore, string, time.Time){
		"99 started tasks untouched 24h": func(t *testing.T, s *SQLiteStore, to string, now time.Time) {
			seedRecipientTasks(t, s, to, "a", "pending", 99, 1, now.Add(-RecipientStallAfter-time.Hour), "")
		},
		"99 unacked items": func(t *testing.T, s *SQLiteStore, to string, _ time.Time) { seedOpen(t, s, to, 99) },
	}
	run := func(t *testing.T, p pair, at time.Time, sat func(*testing.T, *SQLiteStore, string, time.Time), want bool, path string) {
		s := openTestStore(t)
		s.now = func() time.Time { return at }
		sat(t, s, p.to, at)
		var err error
		if path == "guarded" {
			_, _, err = s.SendGuarded(SendReq{From: p.from, To: p.to, Title: "fw work", Type: "proposal"})
		} else {
			_, err = s.Send(p.from, p.to, "fw work", "proposal", "x")
		}
		if got := !errors.Is(err, ErrRecipientSaturated); got != want || (want && err != nil) {
			t.Fatalf("%s -> %s at %s: accepted=%v want %v (err %v)", p.from, p.to, at.Format(time.RFC3339), got, want, err)
		}
	}
	for satName, sat := range saturations {
		for _, path := range []string{"guarded", "raw"} {
			for _, p := range in {
				t.Run(fmt.Sprintf("%s/%s/in-set accepted before expiry/%s>%s", satName, path, p.from, p.to), func(t *testing.T) { run(t, p, before, sat, true, path) })
				t.Run(fmt.Sprintf("%s/%s/refused at the expiry instant/%s>%s", satName, path, p.from, p.to), func(t *testing.T) { run(t, p, finalWishesOverrideEnds, sat, false, path) })
				t.Run(fmt.Sprintf("%s/%s/refused after expiry/%s>%s", satName, path, p.from, p.to), func(t *testing.T) { run(t, p, finalWishesOverrideEnds.Add(time.Hour), sat, false, path) })
			}
			for _, p := range out {
				t.Run(fmt.Sprintf("%s/%s/out of scope refused/%s>%s", satName, path, p.from, p.to), func(t *testing.T) { run(t, p, before, sat, false, path) })
			}
		}
	}
}

// The two older exemptions are untouched, with the override expired.
func TestOlderExemptionsSurviveOverrideExpiry(t *testing.T) {
	after := finalWishesOverrideEnds.Add(24 * time.Hour)
	for _, tc := range []struct{ from, to string }{{"routerstore", "lane"}, {"ra", "sirsi-software-admin"}} {
		s := openTestStore(t)
		s.now = func() time.Time { return after }
		if tc.to == "lane" {
			seedOpen(t, s, "lane", 99)
		} else {
			seedOpen(t, s, tc.to, 99)
		}
		if _, _, err := s.SendGuarded(SendReq{From: tc.from, To: tc.to, Title: "x", Type: "proposal"}); err != nil {
			t.Fatalf("%s -> %s must stay exempt: %v", tc.from, tc.to, err)
		}
	}
}

// An override admission past a saturated recipient is counted (no schema change); an ordinary
// admission to an unsaturated member is not.
func TestFinalWishesOverrideAdmissionIsCounted(t *testing.T) {
	s := openTestStore(t)
	s.now = func() time.Time { return finalWishesOverrideEnds.Add(-time.Hour) }
	count := func() int {
		var n int
		if err := s.db.QueryRow(`SELECT COALESCE(SUM(value),0) FROM counters WHERE name='finalwishes_override_admits';`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, _, err := s.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "codex-finalwishes", Title: "quiet", Type: "proposal"}); err != nil || count() != 0 {
		t.Fatalf("unsaturated recipient: err=%v count=%d, want accepted and 0", err, count())
	}
	seedOpen(t, s, "codex-finalwishes", 99)
	if _, _, err := s.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "codex-finalwishes", Title: "over", Type: "proposal"}); err != nil || count() != 1 {
		t.Fatalf("saturated recipient: err=%v count=%d, want accepted and 1", err, count())
	}
}
