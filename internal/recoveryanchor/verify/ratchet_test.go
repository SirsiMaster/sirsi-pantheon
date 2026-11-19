package verify

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func marshalAuthorized(k testKey) []byte { return ssh.MarshalAuthorizedKey(k.signer.PublicKey()) }

const (
	ttl  = 60 * time.Second
	skew = 5 * time.Second
)

func TestTickAdvancesWithoutAdmissionsAndDetectsRollback(t *testing.T) {
	f := newFixture(t)
	f.clock.Set(t0 + 30)
	if err := Tick(f.store, f.clock, skew); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := f.store.EpochState(1); r != t0+30 {
		t.Fatalf("ratchet = %d, want %d (periodic durable tick with zero admissions)", r, t0+30)
	}
	f.clock.Set(t0 + 20) // 10s back, skew is 5
	err := Tick(f.store, f.clock, skew)
	wantReason(t, reasonOf(err), ReasonClockRollback)
	if r, _, _ := f.store.EpochState(1); r != t0+30 {
		t.Fatalf("ratchet moved on rollback: %d", r)
	}
	st, err := ReadState(f.store)
	if err != nil || st.Epoch != 1 || st.Ratchet != t0+30 {
		t.Fatalf("ReadState = %+v, %v", st, err)
	}
	wantReason(t, reasonOf(Tick(errStore{}, f.clock, skew)), ReasonStoreError)
	_, err = ReadState(errStore{})
	wantReason(t, reasonOf(err), ReasonStoreError)
}

// ADR-078 §1a R6-3 countermodel: max TTL 60s, skew 5s, envelope issued at 940
// with a 1s TTL (expires 941), pruning at 1006, restart at wall clock 940.
func TestPruneCountermodelR63(t *testing.T) {
	f := newFixture(t)
	f.clock.Set(940)
	f.store = NewMemoryStore(f.clock.Now())
	f.cfg.Store = f.store
	f.rebuild()
	e := f.env()
	e.IssuedAt, e.ExpiresAt = 940, 941
	r := f.req(e)
	if d := f.v.Admit(r, f.pol); !d.Allowed {
		t.Fatalf("setup: %s", d.Reason)
	}

	// Not yet eligible one second early: expires_at + maxTTL + skew = 1006.
	f.clock.Set(1005)
	if n, err := Prune(f.store, f.clock, ttl, skew); err != nil || n != 0 || f.store.ReservationCount() != 1 {
		t.Fatalf("early prune deleted %d rows (err %v); the routine itself must refuse", n, err)
	}

	// Reproduce the stalled ratchet the countermodel assumes, then prune at 1006.
	f.clock.Set(940)
	f.store = NewMemoryStore(time.Unix(940, 0))
	f.cfg.Store = f.store
	f.rebuild()
	if d := f.v.Admit(r, f.pol); !d.Allowed {
		t.Fatalf("setup 2: %s", d.Reason)
	}
	f.clock.Set(1006)
	n, err := Prune(f.store, f.clock, ttl, skew)
	if err != nil || n != 1 || f.store.ReservationCount() != 0 {
		t.Fatalf("prune at 1006 deleted %d (err %v)", n, err)
	}
	if rat, _, _ := f.store.EpochState(1); rat != 1006 {
		t.Fatalf("floor = %d, want trusted wall clock 1006 (not the row's expires_at 941)", rat)
	}

	// Negative control for the OLD formula: floor 941 would not have caught it.
	if rolledBack(940, 941, skew) {
		t.Fatal("test bug: old formula should NOT detect this rollback")
	}

	// Crash/restart at wall clock 940: replay row gone, mark gone, TTL passes.
	f.clock.Set(940)
	d := f.v.Admit(r, f.pol)
	wantReason(t, d.Reason, ReasonClockRollback)
}

func TestPruneRefusesUnderRollbackAndIsAtomic(t *testing.T) {
	f := newFixture(t)
	if d := f.admit(f.env()); !d.Allowed {
		t.Fatal(d.Reason)
	}
	f.clock.Set(t0 + 1000)
	if err := Tick(f.store, f.clock, skew); err != nil {
		t.Fatal(err)
	}

	// Crash before commit: rows survive, ratchet unchanged, and eligibility is
	// re-derived inside the next transaction, never from a stale list.
	f.store.FailNextCommit = true
	if _, err := Prune(f.store, f.clock, ttl, skew); err == nil {
		t.Fatal("expected injected commit failure")
	}
	if f.store.ReservationCount() != 1 {
		t.Fatal("a failed commit must not delete rows")
	}
	if n, err := Prune(f.store, f.clock, ttl, skew); err != nil || n != 1 {
		t.Fatalf("resume prune: n=%d err=%v", n, err)
	}

	f.clock.Set(t0)
	_, err := Prune(f.store, f.clock, ttl, skew)
	wantReason(t, reasonOf(err), ReasonClockRollback)
	_, err = Prune(errStore{}, f.clock, ttl, skew)
	wantReason(t, reasonOf(err), ReasonStoreError)
	f.store.FailNextCommit = true
	_, err = StartEpoch(f.store, f.clock)
	wantReason(t, reasonOf(err), ReasonStoreError)
}

// ADR-078 §1a R7-2 / R8-2: the override mints a new epoch whose own ratchet
// and mark start from real time and may be lower than the retired epoch's.
func TestOverrideEpochScopedRatchetAndMark(t *testing.T) {
	f := newFixture(t)

	// Epoch 1 runs under a false-future clock.
	f.clock.Set(t0 + 1000)
	e1 := f.env()
	e1.IssuedAt, e1.ExpiresAt = t0+1000, t0+1060
	if d := f.admit(e1); !d.Allowed {
		t.Fatal(d.Reason)
	}

	// Real time is t0. Owner-gated override.
	f.clock.Set(t0)
	ep, err := StartEpoch(f.store, f.clock)
	if err != nil || ep != 2 {
		t.Fatalf("StartEpoch = %d, %v", ep, err)
	}
	if r, m, ok := f.store.EpochState(1); !ok || r != t0+1000 || m != t0+1000 {
		t.Fatalf("retired epoch state mutated: ratchet=%d mark=%d", r, m)
	}
	if r, m, _ := f.store.EpochState(2); r != t0 || m != t0 {
		t.Fatalf("new epoch must start from the trusted observation: ratchet=%d mark=%d", r, m)
	}

	// Fresh, correctly-epoched request below the retired epoch's mark: admitted.
	fresh := f.env()
	fresh.Epoch = 2
	if d := f.admit(fresh); !d.Allowed {
		t.Fatalf("post-override request denied: %s (%s)", d.Reason, d.Detail)
	}

	// The retired epoch's envelope stays dead regardless of its own expiry.
	wantReason(t, f.admit(e1).Reason, ReasonEpochRetired)

	// Below the new epoch's own mark: ordering-floor denial, not the ratchet.
	behind := f.env()
	behind.Epoch, behind.IssuedAt, behind.ExpiresAt = 2, t0-1, t0+30
	wantReason(t, f.admit(behind).Reason, ReasonBehindMark)

	// Independent ratchet control: the clock itself behind the new floor.
	f.clock.Set(t0 - 100)
	ctl := f.env()
	ctl.Epoch, ctl.IssuedAt, ctl.ExpiresAt = 2, t0-100, t0-40
	wantReason(t, f.admit(ctl).Reason, ReasonClockRollback)
}

func TestFirstEverAdmissionIsCheckedAgainstTheGlobalRatchet(t *testing.T) {
	f := newFixture(t)
	f.clock.Set(t0 + 500)
	if err := Tick(f.store, f.clock, skew); err != nil {
		t.Fatal(err)
	}
	f.clock.Set(t0)
	// A fingerprint never seen before gets no exemption.
	fresh := newEd25519(t)
	f.setGrants([]byte(grantLine(fresh, f.uid, "restart-broker", false)), nil)
	d := f.v.Admit(f.reqWith(fresh, f.env(), sigOpts{}), f.pol)
	wantReason(t, d.Reason, ReasonClockRollback)
}

func TestMemoryStoreUpdateIsAllOrNothing(t *testing.T) {
	s := NewMemoryStore(time.Unix(t0, 0))
	err := s.Update(func(tx Tx) error {
		tx.Reserve("fp", "id", [32]byte{1}, 5)
		tx.SetMark(99)
		return deny(ReasonInternal, "abort")
	})
	if err == nil || s.ReservationCount() != 0 {
		t.Fatal("aborted Update leaked effects")
	}
	if _, m, _ := s.EpochState(1); m != t0 {
		t.Fatalf("mark leaked: %d", m)
	}
}

func TestDenyReasonsAreUniqueAndNonEmpty(t *testing.T) {
	seen := map[Reason]bool{}
	for _, r := range DenyReasons() {
		if r == "" || seen[r] || strings.ContainsAny(string(r), " \t") {
			t.Fatalf("bad or duplicate reason %q", r)
		}
		seen[r] = true
	}
	if seen[ReasonAdmitted] || seen[ReasonRevalidated] {
		t.Fatal("allow codes must not be deny codes")
	}
}
