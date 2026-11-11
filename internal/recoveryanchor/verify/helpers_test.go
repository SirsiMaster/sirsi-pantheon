package verify

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // test signer needs a disallowed digest to prove rejection
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const t0 = int64(1_000_000)

var seenReasons sync.Map

func record(r Reason) { seenReasons.Store(r, true) }

func wantReason(t *testing.T, got, want Reason) {
	t.Helper()
	if got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
	record(want)
}

func reasonOf(err error) Reason {
	if de, ok := err.(*DenyError); ok {
		return de.Reason
	}
	if err == nil {
		return ReasonAdmitted
	}
	return "non-deny-error:" + Reason(err.Error())
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Set(unix int64) { c.mu.Lock(); defer c.mu.Unlock(); c.t = time.Unix(unix, 0) }

// --- throwaway keys, generated per test, never read from disk ---

type testKey struct {
	signer ssh.Signer
	fp     string
}

func wrap(t *testing.T, priv any) testKey {
	t.Helper()
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return testKey{signer: s, fp: ssh.FingerprintSHA256(s.PublicKey())}
}

func newEd25519(t *testing.T) testKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return wrap(t, priv)
}

func newECDSA(t *testing.T, c elliptic.Curve) testKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(c, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return wrap(t, priv)
}

func newRSA(t *testing.T) testKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return wrap(t, priv)
}

// --- independent SSHSIG builder (not the production code path) ---

type sigOpts struct {
	ns          string
	hashAlg     string
	version     uint32
	magic       string
	reserved    string
	trailing    []byte
	pubTrailing []byte
}

func wstr(b []byte, s []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

func digest(alg string, msg []byte) []byte {
	switch alg {
	case "sha256":
		d := sha256.Sum256(msg)
		return d[:]
	case "sha512":
		d := sha512.Sum512(msg)
		return d[:]
	case "sha1":
		d := sha1.Sum(msg) //nolint:gosec // see import
		return d[:]
	}
	return nil
}

func armorBlob(blob []byte) []byte {
	b64 := base64.StdEncoding.EncodeToString(blob)
	var sb strings.Builder
	sb.WriteString("-----BEGIN SSH SIGNATURE-----\n")
	for len(b64) > 70 {
		sb.WriteString(b64[:70] + "\n")
		b64 = b64[70:]
	}
	sb.WriteString(b64 + "\n-----END SSH SIGNATURE-----\n")
	return []byte(sb.String())
}

func signSSHSIG(t *testing.T, k testKey, msg []byte, o sigOpts) []byte {
	t.Helper()
	if o.ns == "" {
		o.ns = Namespace
	}
	if o.hashAlg == "" {
		o.hashAlg = "sha512"
	}
	if o.version == 0 {
		o.version = 1
	}
	if o.magic == "" {
		o.magic = "SSHSIG"
	}
	signed := []byte("SSHSIG")
	signed = wstr(signed, []byte(o.ns))
	signed = wstr(signed, nil)
	signed = wstr(signed, []byte(o.hashAlg))
	signed = wstr(signed, digest(o.hashAlg, msg))
	sig, err := k.signer.Sign(rand.Reader, signed)
	if err != nil {
		t.Fatal(err)
	}
	innerSig := ssh.Marshal(struct {
		Format string
		Blob   []byte
	}{sig.Format, sig.Blob})

	blob := []byte(o.magic)
	blob = binary.BigEndian.AppendUint32(blob, o.version)
	blob = wstr(blob, append(k.signer.PublicKey().Marshal(), o.pubTrailing...))
	blob = wstr(blob, []byte(o.ns))
	blob = wstr(blob, []byte(o.reserved))
	blob = wstr(blob, []byte(o.hashAlg))
	blob = wstr(blob, innerSig)
	blob = append(blob, o.trailing...)
	return armorBlob(blob)
}

// --- fixture ---

const defaultNode = "node-m1"

var reqCounter atomic.Uint64

func uuid4(n uint64) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func grantLine(k testKey, uid uint32, verbs string, override bool) string {
	s := fmt.Sprintf("%s uid=%d verbs=%s", k.fp, uid, verbs)
	if override {
		s += " override=true"
	}
	return s + "\n"
}

type fixture struct {
	t         *testing.T
	clock     *fakeClock
	store     *MemoryStore
	cfg       Config
	v         *Verifier
	key       testKey
	uid       uint32
	grantsSrc []byte
	restrSrc  []byte
	pol       *Policy
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, clock: &fakeClock{}, key: newEd25519(t), uid: 501}
	f.clock.Set(t0)
	f.store = NewMemoryStore(f.clock.Now())
	f.cfg = Config{
		NodeID: defaultNode,
		Verbs: map[Verb]VerbIdentity{
			VerbRestartBroker:        {"ai.sirsi.gemma-broker", strings.Repeat("a", 64)},
			VerbRemountVolume:        {"0F0E0D0C-0B0A-0908-0706-050403020100", strings.Repeat("b", 64)},
			VerbReArmSupervisor:      {"ai.sirsi.horus.supervise", strings.Repeat("c", 64)},
			VerbFetchKnownGoodConfig: {"known-good-config", strings.Repeat("d", 64)},
		},
		MaxTTL:  60 * time.Second,
		MaxSkew: 5 * time.Second,
		Clock:   f.clock,
		Store:   f.store,
	}
	f.setGrants([]byte(grantLine(f.key, f.uid, "restart-broker,re-arm-supervisor", false)), nil)
	f.rebuild()
	return f
}

func (f *fixture) rebuild() {
	f.t.Helper()
	v, err := NewVerifier(f.cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	f.v = v
}

func (f *fixture) setGrants(grants, restr []byte) {
	f.t.Helper()
	p, err := BuildPolicy(7, grants, restr)
	if err != nil {
		f.t.Fatal(err)
	}
	f.grantsSrc, f.restrSrc, f.pol = grants, restr, p
}

func (f *fixture) env() Envelope {
	id := f.cfg.Verbs[VerbRestartBroker]
	return Envelope{
		NodeID: defaultNode, Verb: VerbRestartBroker, Target: id.Target, ConfigDigest: id.ConfigDigest,
		RequestID: uuid4(reqCounter.Add(1)), IssuedAt: t0, ExpiresAt: t0 + 60, Epoch: 1,
	}
}

func (f *fixture) reqWith(k testKey, e Envelope, o sigOpts) Request {
	f.t.Helper()
	b := e.Canonical()
	return Request{Envelope: b, Signature: signSSHSIG(f.t, k, b, o), PeerUID: f.uid}
}

func (f *fixture) req(e Envelope) Request { return f.reqWith(f.key, e, sigOpts{}) }

func (f *fixture) admit(e Envelope) Decision { return f.v.Admit(f.req(e), f.pol) }
