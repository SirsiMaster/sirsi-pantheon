package routerstore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// FinalWishes seven-day override (owner decision 2026-11-11), bound to VERIFIED caller identity
// (SSA review 20261111-064341): the four lanes may send to one another past the recipient limit
// until 2026-11-18T05:50:00Z, but only when the server authenticated the sender through a
// lane-scoped token whose scope contains it. Delete with the block in facade.go after 2026-11-18.

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

// fwSend sends once. verified uses the server-only entry; otherwise the public SendGuarded (a bootstrap,
// unscoped-host, relay or local in-process caller reaches the store only through that).
func fwSend(s *SQLiteStore, p fwPair, verified bool) error {
	fwSeq++
	r := SendReq{From: p.from, To: p.to, Title: fmt.Sprintf("fw work %d", fwSeq), Type: "proposal"}
	var err error
	if verified {
		_, _, err = s.sendGuardedVerified(r)
	} else {
		_, _, err = s.SendGuarded(r)
	}
	return err
}

// The core requirement: verified in-set sends pass, the SAME sends unverified keep the limit.
func TestFinalWishesOverrideRequiresVerifiedSender(t *testing.T) {
	before := finalWishesOverrideEnds.Add(-time.Hour)
	for satName, sat := range fwSaturations {
		for _, to := range fwSet {
			t.Run(satName+"/to "+to, func(t *testing.T) {
				s := openTestStore(t)
				s.now = func() time.Time { return before }
				sat(t, s, to, before)
				for _, p := range fwInSetPairs() {
					if p.to != to {
						continue
					}
					if err := fwSend(s, p, true); err != nil {
						t.Errorf("verified %s>%s refused: %v", p.from, p.to, err)
					}
					// Unverified, same pair: ordinary saturation, exactly as if the override did not exist.
					if err := fwSend(s, p, false); !isSaturated(err) {
						t.Errorf("UNVERIFIED %s>%s: err=%v, want ErrRecipientSaturated", p.from, p.to, err)
					}
				}
			})
		}
	}
}

// Verified but outside the owner's scope, or at/after expiry, keeps the limit.
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
						if err := fwSend(s, p, true); !isSaturated(err) {
							t.Errorf("verified out-of-scope %s>%s: err=%v, want saturated", p.from, p.to, err)
						}
					}
				}
			}
			for _, at := range []time.Time{finalWishesOverrideEnds, finalWishesOverrideEnds.Add(time.Hour)} {
				s := openTestStore(t)
				s.now = func() time.Time { return at }
				sat(t, s, "codex-finalwishes", at)
				if err := fwSend(s, fwPair{"claude-finalwishes-m1", "codex-finalwishes"}, true); !isSaturated(err) {
					t.Errorf("verified at %s: err=%v, want saturated", at.Format(time.RFC3339), err)
				}
			}
		})
	}
}

// The raw Send path has no verified entry and no audit transaction: no exemption, ever.
func TestFinalWishesOverrideNotOnRawSend(t *testing.T) {
	before := finalWishesOverrideEnds.Add(-time.Hour)
	s := openTestStore(t)
	s.now = func() time.Time { return before }
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

// The two older exemptions are untouched and need no verification, with the override expired.
func TestOlderExemptionsSurviveUnverified(t *testing.T) {
	after := finalWishesOverrideEnds.Add(24 * time.Hour)
	for _, tc := range []fwPair{{"routerstore", "lane"}, {"ra", "sirsi-software-admin"}} {
		s := openTestStore(t)
		s.now = func() time.Time { return after }
		seedOpen(t, s, tc.to, 99)
		if err := fwSend(s, tc, false); err != nil {
			t.Fatalf("%s -> %s must stay exempt unverified: %v", tc.from, tc.to, err)
		}
	}
}

// Each over-limit admit is audited once, in the item's own transaction: total counter and per-item
// counter. An unsaturated recipient and an unverified refusal leave no trace.
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
	if err := fwSend(s, a, true); err != nil || total() != 0 || perItem() != 0 {
		t.Fatalf("unsaturated: err=%v total=%d perItem=%d, want accepted and 0/0", err, total(), perItem())
	}
	seedOpen(t, s, "codex-finalwishes", 99)
	if err := fwSend(s, a, false); !isSaturated(err) || total() != 0 {
		t.Fatalf("unverified: err=%v total=%d, want refused and 0", err, total())
	}
	for i, p := range []fwPair{a, b} {
		if err := fwSend(s, p, true); err != nil || total() != i+1 || perItem() != i+1 {
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

// A lane-scoped token whose scope holds the sender is verified: the handler-to-store plumbing admits it.
func TestFinalWishesOverrideOverHTTPScopedTokenIsVerified(t *testing.T) {
	f := newFWFixture(t, []string{"claude-finalwishes-m5"})
	rs := f.client("claude-finalwishes-m5")
	req := SendReq{From: "claude-finalwishes-m5", To: "codex-finalwishes", Title: "scoped ok", Type: "proposal"}
	if _, _, err := rs.SendGuarded(req); err != nil {
		t.Fatalf("scoped, sender in scope, in-set pair: refused: %v", err)
	}
	// Not in set: the limit holds even for a verified sender.
	if _, _, err := rs.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "mercury-other", Title: "x", Type: "proposal"}); err != nil {
		t.Fatalf("unsaturated recipient refused: %v", err)
	}
	for i := 0; i < 99; i++ { // distinct ids: seedOpen's ids would overwrite the codex-finalwishes rows
		if err := f.backend.Put(Item{ID: fmt.Sprintf("pl-%03d", i), From: "seed", To: "plain-lane", Title: "t", Status: "open"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := rs.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "plain-lane", Title: "y", Type: "proposal"}); !isSaturatedWire(err) {
		t.Fatalf("verified to non-member: err=%v, want saturated", err)
	}
	// The raw path over the same scoped token gets no exemption.
	if _, err := rs.Send("claude-finalwishes-m5", "codex-finalwishes", "raw", "proposal", "x"); !isSaturatedWire(err) {
		t.Fatalf("raw Send over HTTP: err=%v, want saturated", err)
	}
}

// Unchanged: a scope that does not contain the asserted sender is refused by the lane-scope check.
func TestFinalWishesOverrideOverHTTPScopeMustContainSender(t *testing.T) {
	f := newFWFixture(t, []string{"claude-finalwishes-m5"})
	_, _, err := f.client("claude-finalwishes-m5").SendGuarded(SendReq{From: "codex-finalwishes-x", To: "codex-finalwishes", Title: "forged", Type: "proposal"})
	wantLaneScope(t, "scope without sender", err)
	_, _, err = f.client("claude-finalwishes-m5").SendGuarded(SendReq{From: "sirsi-software-admin", To: "codex-finalwishes", Title: "forged", Type: "proposal"})
	wantLaneScope(t, "in-set sender outside token scope", err)
}

// Bootstrap and unscoped host tokens can assert any `from`: they are never verified, and no JSON
// field a body carries can change that.
func TestFinalWishesOverrideOverHTTPUnscopedNeverVerified(t *testing.T) {
	f := newFWFixture(t, []string{"cloud-a"})
	hostTok, _, err := f.backend.MintHostToken("mac-host", "unscoped")
	if err != nil {
		t.Fatal(err)
	}
	clients := map[string]*RemoteStore{}
	for name, tok := range map[string]string{"bootstrap": "t0k", "unscoped host token": hostTok} {
		rs := NewRemoteStore(f.url, tok)
		rs.sessionDir, rs.agent, rs.threadID = "", "claude-finalwishes-m5", ""
		rs.host = "mac-host"
		if name == "bootstrap" {
			rs.host = "bootstrap"
		}
		clients[name] = rs
	}
	for name, rs := range clients {
		if _, _, err := rs.SendGuarded(SendReq{From: "claude-finalwishes-m5", To: "codex-finalwishes", Title: "plain", Type: "proposal"}); !isSaturatedWire(err) {
			t.Errorf("%s: err=%v, want saturated", name, err)
		}
		forged := map[string]any{"from": "claude-finalwishes-m5", "to": "codex-finalwishes", "title": "forged " + name, "type": "proposal"}
		for _, k := range []string{"senderVerified", "SenderVerified", "sender_verified", "verified", "Verified", "fromVerified", "bound", "scoped", "laneScoped"} {
			forged[k] = true
		}
		var id string
		var deduped bool
		if err := rs.call("SendGuarded", []any{forged}, &id, &deduped); !isSaturatedWire(err) {
			t.Errorf("%s with forged verification fields: err=%v id=%q, want saturated", name, err, id)
		}
	}
	// The same forged body over a scoped token that lacks the sender is still a lane-scope refusal.
	rs := f.client("cloud-a")
	var id string
	var deduped bool
	err = rs.call("SendGuarded", []any{map[string]any{"from": "claude-finalwishes-m5", "to": "codex-finalwishes", "title": "f", "type": "proposal", "senderVerified": true}}, &id, &deduped)
	wantLaneScope(t, "forged body, scope lacks sender", err)
}
