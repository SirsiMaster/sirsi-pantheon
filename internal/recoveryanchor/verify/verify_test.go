package verify

import (
	"crypto/elliptic"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type errStore struct{}

func (errStore) Update(func(Tx) error) error { return errors.New("disk on fire") }

type panicStore struct{}

func (panicStore) Update(func(Tx) error) error { panic("boom") }

func TestAdmitDecisions(t *testing.T) {
	mustAdmit := func(f *fixture, e Envelope) {
		f.t.Helper()
		if d := f.admit(e); !d.Allowed {
			f.t.Fatalf("setup admit failed: %s (%s)", d.Reason, d.Detail)
		}
	}
	cases := []struct {
		name string
		want Reason
		run  func(f *fixture) Decision
	}{
		{"valid envelope admitted", ReasonAdmitted, func(f *fixture) Decision { return f.admit(f.env()) }},
		{"ecdsa-p256 signer admitted", ReasonAdmitted, func(f *fixture) Decision {
			k := newECDSA(f.t, elliptic.P256())
			f.setGrants([]byte(grantLine(k, f.uid, "restart-broker", false)), nil)
			return f.v.Admit(f.reqWith(k, f.env(), sigOpts{hashAlg: "sha256"}), f.pol)
		}},
		{"wrong namespace", ReasonWrongNamespace, func(f *fixture) Decision {
			return f.v.Admit(f.reqWith(f.key, f.env(), sigOpts{ns: "sirsi-something-else"}), f.pol)
		}},
		{"rsa key type", ReasonKeyTypeNotAllowed, func(f *fixture) Decision {
			return f.v.Admit(f.reqWith(newRSA(f.t), f.env(), sigOpts{}), f.pol)
		}},
		{"sha1 digest", ReasonHashNotAllowed, func(f *fixture) Decision {
			return f.v.Admit(f.reqWith(f.key, f.env(), sigOpts{hashAlg: "sha1"}), f.pol)
		}},
		{"unknown signer", ReasonUnknownSigner, func(f *fixture) Decision {
			return f.v.Admit(f.reqWith(newEd25519(f.t), f.env(), sigOpts{}), f.pol)
		}},
		{"grant removed from source", ReasonUnknownSigner, func(f *fixture) Decision {
			f.setGrants(nil, nil)
			return f.admit(f.env())
		}},
		{"grant restricted by authorized_keys, no override", ReasonUnknownSigner, func(f *fixture) Decision {
			line := `from="10.0.0.9" ` + string(marshalAuthorized(f.key))
			f.setGrants([]byte(grantLine(f.key, f.uid, "restart-broker", false)), []byte(line))
			return f.admit(f.env())
		}},
		{"restricted but override=true admitted", ReasonAdmitted, func(f *fixture) Decision {
			line := `from="10.0.0.9" ` + string(marshalAuthorized(f.key))
			f.setGrants([]byte(grantLine(f.key, f.uid, "restart-broker", true)), []byte(line))
			return f.admit(f.env())
		}},
		{"wrong host", ReasonWrongNode, func(f *fixture) Decision {
			e := f.env()
			e.NodeID = "node-m5"
			return f.admit(e)
		}},
		{"wrong UID for the signing key", ReasonUIDMismatch, func(f *fixture) Decision {
			r := f.req(f.env())
			r.PeerUID = 502
			return f.v.Admit(r, f.pol)
		}},
		{"UID holds a different key's grant, not the signer's", ReasonUIDMismatch, func(f *fixture) Decision {
			other := newEd25519(f.t)
			f.setGrants([]byte(grantLine(f.key, 501, "restart-broker", false)+grantLine(other, 502, "restart-broker", false)), nil)
			r := f.req(f.env())
			r.PeerUID = 502
			return f.v.Admit(r, f.pol)
		}},
		{"two keys on one UID both work", ReasonAdmitted, func(f *fixture) Decision {
			second := newEd25519(f.t)
			f.setGrants([]byte(grantLine(f.key, 501, "restart-broker", false)+grantLine(second, 501, "re-arm-supervisor", false)), nil)
			mustAdmit(f, f.env())
			e := f.env()
			e.Verb = VerbReArmSupervisor
			id := f.cfg.Verbs[VerbReArmSupervisor]
			e.Target, e.ConfigDigest = id.Target, id.ConfigDigest
			return f.v.Admit(f.reqWith(second, e, sigOpts{}), f.pol)
		}},
		{"verb not in this key's grant", ReasonVerbNotGranted, func(f *fixture) Decision {
			e := f.env()
			e.Verb = VerbRemountVolume
			id := f.cfg.Verbs[VerbRemountVolume]
			e.Target, e.ConfigDigest = id.Target, id.ConfigDigest
			return f.admit(e)
		}},
		{"verb outside the fixed four", ReasonVerbUnknown, func(f *fixture) Decision {
			e := f.env()
			e.Verb = "wipe-disk"
			f.setGrants([]byte(grantLine(f.key, f.uid, "restart-broker", false)), nil)
			return f.admit(e)
		}},
		{"verb with no approved identity on this node", ReasonVerbNotConfigured, func(f *fixture) Decision {
			e := f.env()
			delete(f.cfg.Verbs, VerbRestartBroker)
			f.rebuild()
			return f.admit(e)
		}},
		{"target is not the approved identity", ReasonTargetMismatch, func(f *fixture) Decision {
			e := f.env()
			e.Target = "ai.sirsi.something-else"
			return f.admit(e)
		}},
		{"config_digest is not the approved digest", ReasonDigestMismatch, func(f *fixture) Decision {
			e := f.env()
			e.ConfigDigest = strings.Repeat("e", 64)
			return f.admit(e)
		}},
		{"expired", ReasonExpired, func(f *fixture) Decision {
			f.clock.Set(t0 + 60)
			return f.admit(f.env())
		}},
		{"not yet valid", ReasonNotYetValid, func(f *fixture) Decision {
			e := f.env()
			e.IssuedAt, e.ExpiresAt = t0+6, t0+30
			return f.admit(e)
		}},
		{"issued within the skew allowance admitted", ReasonAdmitted, func(f *fixture) Decision {
			e := f.env()
			e.IssuedAt, e.ExpiresAt = t0+5, t0+30
			return f.admit(e)
		}},
		{"TTL longer than the server maximum", ReasonTTLTooLong, func(f *fixture) Decision {
			e := f.env()
			e.ExpiresAt = e.IssuedAt + 61
			return f.admit(e)
		}},
		{"expires_at not after issued_at", ReasonTTLInvalid, func(f *fixture) Decision {
			e := f.env()
			e.ExpiresAt = e.IssuedAt
			return f.admit(e)
		}},
		{"clock rolled back past the ratchet", ReasonClockRollback, func(f *fixture) Decision {
			f.clock.Set(t0 + 1000)
			if err := Tick(f.store, f.clock, f.cfg.MaxSkew); err != nil {
				f.t.Fatal(err)
			}
			f.clock.Set(t0)
			return f.admit(f.env())
		}},
		{"clock rollback inside the skew allowance is tolerated", ReasonAdmitted, func(f *fixture) Decision {
			f.clock.Set(t0 + 4)
			if err := Tick(f.store, f.clock, f.cfg.MaxSkew); err != nil {
				f.t.Fatal(err)
			}
			f.clock.Set(t0)
			return f.admit(f.env())
		}},
		{"envelope epoch retired by an override", ReasonEpochRetired, func(f *fixture) Decision {
			if _, err := StartEpoch(f.store, f.clock); err != nil {
				f.t.Fatal(err)
			}
			return f.admit(f.env()) // epoch 1
		}},
		{"envelope epoch ahead of the daemon", ReasonEpochFuture, func(f *fixture) Decision {
			e := f.env()
			e.Epoch = 3
			return f.admit(e)
		}},
		{"replay of identical content", ReasonReplaySame, func(f *fixture) Decision {
			r := f.req(f.env())
			if d := f.v.Admit(r, f.pol); !d.Allowed {
				f.t.Fatal("first admit failed")
			}
			return f.v.Admit(r, f.pol)
		}},
		{"same request_id with altered content", ReasonReplayConflict, func(f *fixture) Decision {
			e := f.env()
			mustAdmit(f, e)
			e.ExpiresAt--
			return f.admit(e)
		}},
		{"issued_at behind the high-water mark", ReasonBehindMark, func(f *fixture) Decision {
			e := f.env()
			e.IssuedAt, e.ExpiresAt = t0+3, t0+30
			mustAdmit(f, e)
			return f.admit(f.env()) // issued_at = t0 < t0+3
		}},
		{"two distinct requests in the same issued_at second", ReasonAdmitted, func(f *fixture) Decision {
			mustAdmit(f, f.env())
			return f.admit(f.env())
		}},
		{"signature over different bytes", ReasonSignatureInvalid, func(f *fixture) Decision {
			signedFor := f.env()
			other := f.env()
			r := f.req(signedFor)
			r.Envelope = other.Canonical() // valid canonical envelope, but not what was signed
			return f.v.Admit(r, f.pol)
		}},
		{"envelope oversize", ReasonEnvelopeOversize, func(f *fixture) Decision {
			r := f.req(f.env())
			r.Envelope = []byte(strings.Repeat("a", MaxEnvelopeBytes+1))
			return f.v.Admit(r, f.pol)
		}},
		{"envelope duplicate field", ReasonEnvelopeDuplicateField, func(f *fixture) Decision {
			b := []byte(strings.Replace(string(f.env().Canonical()), `{"`, `{"epoch":1,"`, 1))
			return f.v.Admit(Request{Envelope: b, Signature: signSSHSIG(f.t, f.key, b, sigOpts{}), PeerUID: f.uid}, f.pol)
		}},
		{"envelope unknown field", ReasonEnvelopeUnknownField, func(f *fixture) Decision {
			b := []byte(strings.Replace(string(f.env().Canonical()), `{"`, `{"lane":"x","`, 1))
			return f.v.Admit(Request{Envelope: b, Signature: signSSHSIG(f.t, f.key, b, sigOpts{}), PeerUID: f.uid}, f.pol)
		}},
		{"envelope missing field", ReasonEnvelopeMissingField, func(f *fixture) Decision {
			b := []byte(strings.Replace(string(f.env().Canonical()), `"epoch":1,`, ``, 1))
			return f.v.Admit(Request{Envelope: b, Signature: signSSHSIG(f.t, f.key, b, sigOpts{}), PeerUID: f.uid}, f.pol)
		}},
		{"envelope malformed", ReasonEnvelopeMalformed, func(f *fixture) Decision {
			return f.v.Admit(Request{Envelope: []byte("{"), Signature: nil, PeerUID: f.uid}, f.pol)
		}},
		{"envelope not canonical", ReasonEnvelopeNonCanonical, func(f *fixture) Decision {
			b := []byte(strings.Replace(string(f.env().Canonical()), `"epoch":1`, `"epoch": 1`, 1))
			return f.v.Admit(Request{Envelope: b, Signature: signSSHSIG(f.t, f.key, b, sigOpts{}), PeerUID: f.uid}, f.pol)
		}},
		{"signature malformed", ReasonSignatureMalformed, func(f *fixture) Decision {
			r := f.req(f.env())
			r.Signature = []byte("not a signature")
			return f.v.Admit(r, f.pol)
		}},
		{"signature oversize", ReasonSignatureOversize, func(f *fixture) Decision {
			r := f.req(f.env())
			r.Signature = []byte(strings.Repeat("A", MaxSignatureArmorBytes+1))
			return f.v.Admit(r, f.pol)
		}},
		{"store failure fails closed", ReasonStoreError, func(f *fixture) Decision {
			f.cfg.Store = errStore{}
			f.rebuild()
			return f.admit(f.env())
		}},
		{"store failure after allow decision is not durable", ReasonStoreError, func(f *fixture) Decision {
			f.store.FailNextCommit = true
			return f.admit(f.env())
		}},
		{"panic fails closed", ReasonInternal, func(f *fixture) Decision {
			f.cfg.Store = panicStore{}
			f.rebuild()
			return f.admit(f.env())
		}},
		{"no policy snapshot", ReasonPolicyInvalid, func(f *fixture) Decision {
			return f.v.Admit(f.req(f.env()), nil)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			d := c.run(f)
			wantReason(t, d.Reason, c.want)
			if d.Allowed != (c.want == ReasonAdmitted) {
				t.Fatalf("Allowed=%v for %s", d.Allowed, d.Reason)
			}
			if d.Allowed != (d.Admitted != nil) {
				t.Fatal("Admitted record must be present exactly when allowed")
			}
		})
	}
}

func TestDenyLeavesStoreUntouched(t *testing.T) {
	f := newFixture(t)
	ratchet0, mark0, _ := f.store.EpochState(1)
	for _, mut := range []func(*Envelope){
		func(e *Envelope) { e.NodeID = "elsewhere" },
		func(e *Envelope) { e.Target = "nope" },
		func(e *Envelope) { e.Verb = VerbRemountVolume },
	} {
		e := f.env()
		e.IssuedAt, e.ExpiresAt = t0+4, t0+30 // would advance the mark if admitted
		mut(&e)
		if d := f.admit(e); d.Allowed {
			t.Fatal("expected deny")
		}
	}
	r, m, _ := f.store.EpochState(1)
	if f.store.ReservationCount() != 0 || r != ratchet0 || m != mark0 {
		t.Fatalf("deny mutated store: rows=%d ratchet %d->%d mark %d->%d", f.store.ReservationCount(), ratchet0, r, mark0, m)
	}
}

func TestAdmitRecordFields(t *testing.T) {
	f := newFixture(t)
	e := f.env()
	d := f.admit(e)
	if !d.Allowed {
		t.Fatal(d.Reason)
	}
	a := d.Admitted
	if a.Envelope != e || a.Fingerprint != f.key.fp || a.UID != f.uid || a.Generation != f.pol.Generation {
		t.Fatalf("admitted record wrong: %+v", a)
	}
}

func TestConcurrentSameRequestIDOnlyOneWins(t *testing.T) {
	f := newFixture(t)
	base := f.env()
	var reqs []Request
	for i := 0; i < 16; i++ {
		e := base
		e.ExpiresAt = base.IssuedAt + 10 + int64(i) // different content, same request_id
		reqs = append(reqs, f.req(e))
	}
	var wg sync.WaitGroup
	results := make([]Decision, len(reqs))
	for i := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = f.v.Admit(reqs[i], f.pol)
		}()
	}
	wg.Wait()
	won := 0
	for _, d := range results {
		if d.Allowed {
			won++
		} else {
			wantReason(t, d.Reason, ReasonReplayConflict)
		}
	}
	if won != 1 || f.store.ReservationCount() != 1 {
		t.Fatalf("winners=%d rows=%d, want exactly 1", won, f.store.ReservationCount())
	}
}

func TestNewVerifierRejectsBadConfig(t *testing.T) {
	good := newFixture(t).cfg
	for name, mut := range map[string]func(*Config){
		"no node":       func(c *Config) { c.NodeID = "" },
		"bad node":      func(c *Config) { c.NodeID = "a b" },
		"zero ttl":      func(c *Config) { c.MaxTTL = 0 },
		"negative skew": func(c *Config) { c.MaxSkew = -time.Second },
		"no clock":      func(c *Config) { c.Clock = nil },
		"no store":      func(c *Config) { c.Store = nil },
		"unknown verb":  func(c *Config) { c.Verbs = map[Verb]VerbIdentity{"wipe-disk": {"x", strings.Repeat("a", 64)}} },
		"bad target":    func(c *Config) { c.Verbs = map[Verb]VerbIdentity{VerbRestartBroker: {"a/b", strings.Repeat("a", 64)}} },
		"bad digest":    func(c *Config) { c.Verbs = map[Verb]VerbIdentity{VerbRestartBroker: {"ok", "xyz"}} },
	} {
		cfg := good
		cfg.Verbs = map[Verb]VerbIdentity{}
		for k, v := range good.Verbs {
			cfg.Verbs[k] = v
		}
		mut(&cfg)
		if _, err := NewVerifier(cfg); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

// ---- Pre-dispatch revalidation ----

func (f *fixture) current(t *testing.T) Current {
	t.Helper()
	st, err := ReadState(f.store)
	if err != nil {
		t.Fatal(err)
	}
	live, lerr := BuildPolicy(f.pol.Generation, f.grantsSrc, f.restrSrc)
	return Current{
		Epoch: st.Epoch, RatchetFloor: st.Ratchet, PublishedGeneration: f.pol.Generation,
		PublishedHashes: f.pol.Hashes, Live: live, LiveErr: lerr,
	}
}

func TestRevalidate(t *testing.T) {
	const skew = 5 * time.Second
	grantsWith := func(f *fixture, uid uint32, verbs string) []byte {
		return []byte(grantLine(f.key, uid, verbs, false))
	}
	cases := []struct {
		name string
		want Reason
		now  int64
		mut  func(f *fixture, c *Current)
	}{
		{"nothing changed", ReasonRevalidated, t0 + 1, func(*fixture, *Current) {}},
		{"epoch retired since admission", ReasonEpochRetired, t0 + 1, func(_ *fixture, c *Current) { c.Epoch = 2 }},
		{"epoch ahead", ReasonEpochFuture, t0 + 1, func(_ *fixture, c *Current) { c.Epoch = 0 }},
		{"dirty flag set", ReasonPolicyDirty, t0 + 1, func(_ *fixture, c *Current) { c.Dirty = true }},
		{"synchronous validate failed", ReasonPolicyInvalid, t0 + 1, func(_ *fixture, c *Current) {
			c.Live, c.LiveErr = nil, ErrPolicyInvalid
		}},
		{"validate returned no policy", ReasonPolicyInvalid, t0 + 1, func(_ *fixture, c *Current) { c.Live = nil }},
		{"source changed same length, no republish yet", ReasonPolicyDirty, t0 + 1, func(f *fixture, c *Current) {
			c.Live, _ = BuildPolicy(f.pol.Generation, grantsWith(f, 502, "restart-broker,re-arm-supervisor"), nil)
		}},
		{"clock rolled back since admission", ReasonClockRollback, t0 + 1, func(_ *fixture, c *Current) { c.RatchetFloor = t0 + 1000 }},
		{"expired while queued", ReasonExpired, t0 + 60, func(*fixture, *Current) {}},
		{"generation fence moved, grant unchanged", ReasonGenerationMoved, t0 + 1, func(_ *fixture, c *Current) { c.PublishedGeneration++ }},
		{"grant removed and republished", ReasonGrantRevoked, t0 + 1, func(f *fixture, c *Current) {
			c.Live, _ = BuildPolicy(f.pol.Generation, nil, nil)
			c.PublishedHashes = c.Live.Hashes
		}},
		{"grant UID changed and republished", ReasonGrantRevoked, t0 + 1, func(f *fixture, c *Current) {
			c.Live, _ = BuildPolicy(f.pol.Generation, grantsWith(f, 999, "restart-broker"), nil)
			c.PublishedHashes = c.Live.Hashes
		}},
		{"verb narrowed and republished", ReasonGrantRevoked, t0 + 1, func(f *fixture, c *Current) {
			c.Live, _ = BuildPolicy(f.pol.Generation, grantsWith(f, 501, "re-arm-supervisor"), nil)
			c.PublishedHashes = c.Live.Hashes
		}},
		{"key restricted by authorized_keys and republished", ReasonGrantRevoked, t0 + 1, func(f *fixture, c *Current) {
			c.Live, _ = BuildPolicy(f.pol.Generation, f.grantsSrc, []byte(`from="1.2.3.4" `+string(marshalAuthorized(f.key))))
			c.PublishedHashes = c.Live.Hashes
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			d := f.admit(f.env())
			if !d.Allowed {
				t.Fatal(d.Reason)
			}
			cur := f.current(t)
			c.mut(f, &cur)
			got := Revalidate(d.Admitted, cur, time.Unix(c.now, 0), skew)
			wantReason(t, got.Reason, c.want)
			if got.Allowed != (c.want == ReasonRevalidated) {
				t.Fatalf("Allowed=%v for %s", got.Allowed, got.Reason)
			}
		})
	}
	t.Run("nil admitted request", func(t *testing.T) {
		wantReason(t, Revalidate(nil, Current{}, time.Unix(t0, 0), skew).Reason, ReasonInternal)
	})
}

// The ADR's core race: admit, then an override mints a new epoch before
// dispatch. The queued request must die at revalidation, not only at admission.
func TestRevalidateAfterOverrideRetiresQueuedRequest(t *testing.T) {
	f := newFixture(t)
	d := f.admit(f.env())
	if !d.Allowed {
		t.Fatal(d.Reason)
	}
	if _, err := StartEpoch(f.store, f.clock); err != nil {
		t.Fatal(err)
	}
	got := Revalidate(d.Admitted, f.current(t), f.clock.Now(), 5*time.Second)
	wantReason(t, got.Reason, ReasonEpochRetired)
}
