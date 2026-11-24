package verify

import (
	"slices"
	"testing"
)

// Reviewer reproducer (SHA/SSA, PR #1404): mutating a Grant returned by
// Lookup must not change the policy.
func TestLookupReturnsOwnedVerbs(t *testing.T) {
	f := newFixture(t)
	g, ok := f.pol.Lookup(f.key.fp)
	if !ok {
		t.Fatal("no grant")
	}
	g.Verbs[0] = VerbRemountVolume // attempt to widen the policy through the alias
	g2, _ := f.pol.Lookup(f.key.fp)
	if g2.Allows(VerbRemountVolume) || !g2.Allows(VerbRestartBroker) {
		t.Fatalf("policy changed through a returned slice: %v", g2.Verbs)
	}
	e := f.env()
	id := f.cfg.Verbs[VerbRemountVolume]
	e.Verb, e.Target, e.ConfigDigest = VerbRemountVolume, id.Target, id.ConfigDigest
	if d := f.admit(e); d.Allowed || d.Reason != ReasonVerbNotGranted {
		t.Fatalf("remount admitted after mutation: %+v", d)
	}
}

// Reviewer reproducer: mutating Config.Verbs after NewVerifier must not
// change the verifier.
func TestNewVerifierOwnsVerbMap(t *testing.T) {
	f := newFixture(t)
	e := f.env() // built from the approved identity
	req := f.req(e)
	f.cfg.Verbs[VerbRestartBroker] = VerbIdentity{Target: "ai.sirsi.attacker", ConfigDigest: e.ConfigDigest}
	delete(f.cfg.Verbs, VerbRemountVolume)
	if d := f.v.Admit(req, f.pol); !d.Allowed {
		t.Fatalf("caller mutation of Config.Verbs changed the verifier: %+v", d)
	}
}

// Other exported accessors that hand out slices must also be owned copies.
func TestExportedSlicesAreCopies(t *testing.T) {
	v := AllVerbs()
	v[0] = "x"
	if slices.Contains(AllVerbs(), "x") {
		t.Fatal("AllVerbs aliases shared storage")
	}
	r := DenyReasons()
	r[0] = "x"
	if slices.Contains(DenyReasons(), "x") {
		t.Fatal("DenyReasons aliases shared storage")
	}
}
