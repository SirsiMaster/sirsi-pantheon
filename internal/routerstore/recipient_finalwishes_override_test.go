package routerstore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// FinalWishes seven-day override (owner decision 2026-11-11, name-keyed by owner ruling): the four
// lanes may send to one another past the recipient limit until 2026-11-18T05:50:00Z.
// Delete with the block in facade.go after 2026-11-18.

var fwSet = []string{"claude-finalwishes-m1", "claude-finalwishes-m5", "codex-finalwishes", "sirsi-software-admin"}

type fwPair struct{ from, to string }

func fwInSetPairs() []fwPair {
	var in []fwPair
	for _, f := range fwSet {
		for _, to := range fwSet {
			if f != to {
				in = append(in, fwPair{f, to})
			}
		}
	}
	return in
}

var fwSaturations = map[string]func(*testing.T, *SQLiteStore, string, time.Time){
	"99 started tasks untouched 24h": func(t *testing.T, s *SQLiteStore, to string, now time.Time) {
		seedRecipientTasks(t, s, to, "a", "pending", 99, 1, now.Add(-RecipientStallAfter-time.Hour), "")
	},
	"99 unacked items": func(t *testing.T, s *SQLiteStore, to string, _ time.Time) { seedOpen(t, s, to, 99) },
}

func isSaturated(err error) bool { return errors.Is(err, ErrRecipientSaturated) }

var fwSeq int

func fwSend(s *SQLiteStore, p fwPair) error {
	fwSeq++
	_, _, err := s.SendGuarded(SendReq{From: p.from, To: p.to, Title: fmt.Sprintf("fw work %d", fwSeq), Type: "proposal"})
	return err
}

// All 12 ordered in-set pairs are admitted before expiry, past either kind of saturation.
func TestFinalWishesOverrideAdmitsInSetPairs(t *testing.T) {
	before := finalWishesOverrideEnds.Add(-time.Hour)
	for satName, sat := range fwSaturations {
		for _, to := range fwSet {
			t.Run(satName+"/to "+to, func(t *testing.T) {
				s := openTestStore(t)
				s.now = func() time.Time { return before }
				sat(t, s, to, before)
				for _, p := range fwInSetPairs() {
					if p.to == to {
						if err := fwSend(s, p); err != nil {
							t.Errorf("%s>%s refused: %v", p.from, p.to, err)
						}
					}
				}
			})
		}
	}
}

// At and after expiry, outside the set, and the web lanes keep the limit.
func TestFinalWishesOverrideScopeAndExpiry(t *testing.T) {
	before := finalWishesOverrideEnds.Add(-time.Hour)
	out := []fwPair{
		{"claude-finalwishes-m5", "some-other-lane"},
		{"some-other-lane", "codex-finalwishes"},
		{"claude-finalwishes-web", "codex-finalwishes"},
		{"codex-finalwishes", "codex-finalwishes-web"},
		{"mercury", "sirsi-software-admin"},
		{"claude-finalwishes-m1", "claude-finalwishes-web"},
	}
	for satName, sat := range fwSaturations {
		t.Run(satName, func(t *testing.T) {
			targets := map[string]bool{}
			for _, p := range out {
				targets[p.to] = true
			}
			for to := range targets {
				s := openTestStore(t)
				s.now = func() time.Time { return before }
				sat(t, s, to, before)
				for _, p := range out {
					if p.to == to {
						if err := fwSend(s, p); !isSaturated(err) {
							t.Errorf("out of scope %s>%s: err=%v, want saturated", p.from, p.to, err)
						}
					}
				}
			}
			for _, at := range []time.Time{finalWishesOverrideEnds, finalWishesOverrideEnds.Add(time.Hour)} {
				s := openTestStore(t)
				s.now = func() time.Time { return at }
				sat(t, s, "codex-finalwishes", at)
				if err := fwSend(s, fwPair{"claude-finalwishes-m1", "codex-finalwishes"}); !isSaturated(err) {
					t.Errorf("at %s: err=%v, want saturated", at.Format(time.RFC3339), err)
				}
			}
		})
	}
}

// The raw Send path has no audit transaction: no exemption, even in-set and before expiry.
func TestFinalWishesOverrideNotOnRawSend(t *testing.T) {
	s := openTestStore(t)
	s.now = func() time.Time { return finalWishesOverrideEnds.Add(-time.Hour) }
	seedOpen(t, s, "codex-finalwishes", 99)
	for _, f := range fwSet {
		if f == "codex-finalwishes" {
			continue
		}
		if _, err := s.Send(f, "codex-finalwishes", "raw", "proposal", "x"); !isSaturated(err) {
			t.Errorf("raw Send %s: err=%v, want saturated", f, err)
		}
	}
}

// The two older exemptions are untouched, with the override expired.
func TestOlderExemptionsSurviveOverrideExpiry(t *testing.T) {
	after := finalWishesOverrideEnds.Add(24 * time.Hour)
	for _, tc := range []fwPair{{"routerstore", "lane"}, {"ra", "sirsi-software-admin"}} {
		s := openTestStore(t)
		s.now = func() time.Time { return after }
		seedOpen(t, s, tc.to, 99)
		if err := fwSend(s, tc); err != nil {
			t.Fatalf("%s -> %s must stay exempt: %v", tc.from, tc.to, err)
		}
	}
}

// Each over-limit admit is audited once, in the item's own transaction: total counter and per-item
// counter. An unsaturated recipient leaves no trace.
func TestFinalWishesOverrideAdmitIsAudited(t *testing.T) {
	s := openTestStore(t)
	s.now = func() time.Time { return finalWishesOverrideEnds.Add(-time.Hour) }
	count := func(where string) int {
		var n int
		if err := s.db.QueryRow(`SELECT COALESCE(SUM(value),0) FROM counters WHERE name ` + where + `;`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	total := func() int { return count("= 'finalwishes_override_admits'") }
	perItem := func() int { return count("LIKE 'finalwishes_override_admit:%'") }
	a, b := fwPair{"claude-finalwishes-m5", "codex-finalwishes"}, fwPair{"claude-finalwishes-m1", "codex-finalwishes"}
	if err := fwSend(s, a); err != nil || total() != 0 || perItem() != 0 {
		t.Fatalf("unsaturated: err=%v total=%d perItem=%d, want accepted and 0/0", err, total(), perItem())
	}
	seedOpen(t, s, "codex-finalwishes", 99)
	for i, p := range []fwPair{a, b} {
		if err := fwSend(s, p); err != nil || total() != i+1 || perItem() != i+1 {
			t.Fatalf("admit %d: err=%v total=%d perItem=%d", i+1, err, total(), perItem())
		}
	}
}

// ---- through the real HTTP handler -------------------------------------------------------------

func isSaturatedWire(err error) bool {
	return err != nil && (isSaturated(err) || strings.Contains(err.Error(), ErrRecipientSaturated.Error()))
}

func newFWFixture(t *testing.T, scope []string) *scopedFixture {
	f := newScopedFixture(t, scope)
	f.backend.now = func() time.Time { return finalWishesOverrideEnds.Add(-time.Hour) }
	seedOpen(t, f.backend, "codex-finalwishes", 99)
	return f
}

// INTENDED (owner ruling: trust the lane name for 7 days): an in-set send over an UNSCOPED host token,
// and over the bootstrap token, is admitted past a saturated recipient. The raw path is not.
func TestFinalWishesOverrideOverHTTPAdmitsInSetOnUnscopedHostToken(t *testing.T) {
	f := newFWFixture(t, []string{"cloud-a"})
	hostTok, _, err := f.backend.MintHostToken("mac-host", "unscoped")
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ tok, host string }{"unscoped host token": {hostTok, "mac-host"}, "bootstrap": {"t0k", "bootstrap"}} {
		rs := NewRemoteStore(f.url, c.tok)
		rs.sessionDir, rs.agent, rs.threadID, rs.host = "", "claude-finalwishes-m5", "", c.host
		if _, _, err := rs.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "codex-finalwishes", Title: "in-set " + name, Type: "proposal"}); err != nil {
			t.Errorf("%s in-set send refused: %v", name, err)
		}
		if _, _, err := rs.SendGuarded(SendReq{From: "mercury", To: "codex-finalwishes", Title: "out-of-set " + name, Type: "proposal"}); !isSaturatedWire(err) {
			t.Errorf("%s non-member sender: err=%v, want saturated", name, err)
		}
		if _, err := rs.Send("claude-finalwishes-m5", "codex-finalwishes", "raw "+name, "proposal", "x"); !isSaturatedWire(err) {
			t.Errorf("%s raw Send: err=%v, want saturated", name, err)
		}
	}
}

// Unchanged: a lane-scoped token whose scope lacks the asserted sender is refused by the scope check.
func TestFinalWishesOverrideOverHTTPScopeMustStillContainSender(t *testing.T) {
	f := newFWFixture(t, []string{"claude-finalwishes-m5"})
	rs := f.client("claude-finalwishes-m5")
	if _, _, err := rs.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "codex-finalwishes", Title: "own lane", Type: "proposal"}); err != nil {
		t.Fatalf("scoped, sender in scope: refused: %v", err)
	}
	_, _, err := rs.SendGuarded(SendReq{From: "sirsi-software-admin", To: "codex-finalwishes", Title: "forged", Type: "proposal"})
	wantLaneScope(t, "in-set sender outside token scope", err)
}
