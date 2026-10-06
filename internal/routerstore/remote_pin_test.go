package routerstore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mkCert(t *testing.T, cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, isCA bool) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: isCA, BasicConstraintsValid: true}
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c, key
}

func pinsOf(t *testing.T, certs ...*x509.Certificate) [][32]byte {
	t.Helper()
	var vals []string
	for _, c := range certs {
		vals = append(vals, spkiPin(c))
	}
	p, err := parseSPKIPins(strings.Join(vals, ","))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// A pin matches any key in the verified chain, so pinning the CA survives leaf renewal;
// a key that is not in the chain is rejected; so is an empty chain.
func TestPinMatchesAnyKeyInTheVerifiedChain(t *testing.T) {
	ca, caKey := mkCert(t, "ca", nil, nil, true)
	leaf, _ := mkCert(t, "leaf", ca, caKey, false)
	other, _ := mkCert(t, "other", nil, nil, true)
	chains := [][]*x509.Certificate{{leaf, ca}}
	if err := checkPins(chains, pinsOf(t, ca)); err != nil {
		t.Fatalf("a pinned CA key must be accepted: %v", err)
	}
	if err := checkPins(chains, pinsOf(t, leaf)); err != nil {
		t.Fatalf("a pinned leaf key must be accepted: %v", err)
	}
	if err := checkPins(chains, pinsOf(t, other, ca)); err != nil {
		t.Fatalf("one of several pins matching is enough (rotation): %v", err)
	}
	if err := checkPins(chains, pinsOf(t, other)); !errors.Is(err, ErrPinMismatch) {
		t.Fatalf("an unrelated key must be rejected, got %v", err)
	}
	if err := checkPins(nil, pinsOf(t, ca)); !errors.Is(err, ErrPinMismatch) {
		t.Fatalf("no verified chain must be rejected, got %v", err)
	}
}

// End to end over real TLS: only the right pin connects, and a mismatch never reaches the handler.
func TestPinnedClientConnectsOnlyWithTheRightPin(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); _, _ = w.Write([]byte("ok")) }))
	t.Cleanup(srv.Close)
	base := srv.Client().Transport.(*http.Transport).TLSClientConfig // trusts the test CA, as normal verification would
	other, _ := mkCert(t, "other", nil, nil, true)

	good := &http.Client{Transport: &http.Transport{TLSClientConfig: pinnedTLSConfig(base, pinsOf(t, srv.Certificate()))}}
	resp, err := good.Get(srv.URL)
	if err != nil {
		t.Fatalf("the right pin must connect: %v", err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("handler hits = %d, want 1", hits.Load())
	}

	bad := &http.Client{Transport: &http.Transport{TLSClientConfig: pinnedTLSConfig(base, pinsOf(t, other))}}
	if _, err := bad.Get(srv.URL); err == nil || !strings.Contains(err.Error(), "pin mismatch") {
		t.Fatalf("a wrong pin must fail with a pin mismatch, got %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("a pin mismatch must stop before the handler (hits = %d)", hits.Load())
	}

	// And the pin is on top of normal verification, never instead of it: a server the base
	// config does not trust is refused even though its key IS pinned.
	untrusting := &http.Client{Transport: &http.Transport{TLSClientConfig: pinnedTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}, pinsOf(t, srv.Certificate()))}}
	if _, err := untrusting.Get(srv.URL); err == nil {
		t.Fatal("pinning must not bypass certificate verification")
	}
}

// Misconfiguration refuses to connect; it never degrades to unpinned.
func TestPinMisconfigurationFailsClosed(t *testing.T) {
	if pinTransport("", "https://x") != nil || pinTransport("   ", "https://x") != nil {
		t.Fatal("no pin configured must leave the default transport alone")
	}
	good := spkiPin(func() *x509.Certificate { c, _ := mkCert(t, "c", nil, nil, true); return c }())
	for name, c := range map[string]struct{ env, base string }{
		"not base64":    {"%%%not-base64%%%", "https://x"},
		"wrong length":  {"AAAA", "https://x"},
		"only commas":   {" , ,", "https://x"},
		"http url":      {good, "http://x"},
		"missing https": {good, "x.example"},
	} {
		rt := pinTransport(c.env, c.base)
		if _, ok := rt.(failClosedTransport); !ok {
			t.Errorf("%s: want a fail-closed transport, got %T", name, rt)
			continue
		}
		if _, err := rt.RoundTrip(httptest.NewRequest("GET", "https://x/", nil)); err == nil {
			t.Errorf("%s: a fail-closed transport must error", name)
		}
	}
	if _, ok := pinTransport(good, "https://x").(*http.Transport); !ok {
		t.Fatal("a valid pin on an https URL must yield a real pinned transport")
	}
}

// Through the real constructor: the environment variable is what turns pinning on.
func TestRemoteStoreHonorsTheEnvironmentPin(t *testing.T) {
	t.Setenv(EnvSPKIPin, "")
	if rs := NewRemoteStore("https://router.example", "tok"); rs.client.Transport != nil {
		t.Fatal("with no pin configured the transport must be the untouched default")
	}
	t.Setenv(EnvSPKIPin, "AAAA")
	rs := NewRemoteStore("https://router.example", "tok")
	if _, ok := rs.client.Transport.(failClosedTransport); !ok {
		t.Fatalf("a malformed env pin must fail closed, got %T", rs.client.Transport)
	}
	if _, err := rs.Get("anything"); err == nil || !strings.Contains(err.Error(), EnvSPKIPin) {
		t.Fatalf("calls must fail naming %s, got %v", EnvSPKIPin, err)
	}
}
