package routerstore

// Opt-in TLS public-key pinning for the router service client (ADR-062 section 4, G6).
//
// The deploy publishes the service's SPKI hash in the release manifest, but until this file
// nothing enforced it: a redirected SIRSI_ROUTER_URL with any publicly valid certificate
// would have been trusted. With SIRSI_ROUTER_SPKI_PIN set, the client still verifies the
// certificate normally and then additionally requires that some certificate in the verified
// chain has a public key whose SHA-256 is one of the pins. Pinning a CA/intermediate (not the
// leaf) survives routine leaf renewal; several pins allow rotation without a flag day.
//
// Fail closed: a malformed pin, or a pin set against a non-https URL, makes every call fail
// with a clear error. It never silently falls back to unpinned.
//
// Where it is set: the process that opens the https connection, i.e. the per-host relay
// (`sirsi router relay serve`) or a node with SIRSI_ROUTER_URL=https://... directly. Lanes on
// spool:// talk to the relay and need nothing.

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// EnvSPKIPin is a comma-separated list of base64 SHA-256 digests of SubjectPublicKeyInfo, the
// form `openssl x509 -pubkey | openssl pkey -pubin -outform der | openssl dgst -sha256 -binary | base64`
// prints (and scripts/router-service/deploy.sh publishes).
const EnvSPKIPin = "SIRSI_ROUTER_SPKI_PIN"

// ErrPinMismatch means the server presented a certificate chain none of whose public keys is pinned.
var ErrPinMismatch = errors.New("routerstore: TLS public-key pin mismatch: the server's certificate chain contains no pinned key")

func parseSPKIPins(s string) ([][32]byte, error) {
	var pins [][32]byte
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(p)
		if err != nil || len(raw) != sha256.Size {
			return nil, fmt.Errorf("routerstore: %s entry %q is not a base64 SHA-256 digest (want 44 characters)", EnvSPKIPin, p)
		}
		var d [32]byte
		copy(d[:], raw)
		pins = append(pins, d)
	}
	return pins, nil
}

// pinnedTLSConfig layers the pin check over base (which still does normal verification).
func pinnedTLSConfig(base *tls.Config, pins [][32]byte) *tls.Config {
	cfg := base.Clone()
	cfg.VerifyPeerCertificate = func(_ [][]byte, chains [][]*x509.Certificate) error {
		return checkPins(chains, pins)
	}
	return cfg
}

func checkPins(chains [][]*x509.Certificate, pins [][32]byte) error {
	for _, chain := range chains {
		for _, c := range chain {
			sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
			for _, p := range pins {
				if sum == p {
					return nil
				}
			}
		}
	}
	return ErrPinMismatch
}

// spkiPin is the pin value for a certificate, for tests and operators.
func spkiPin(c *x509.Certificate) string {
	sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// failClosedTransport refuses every request with err.
type failClosedTransport struct{ err error }

func (t failClosedTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

// pinConfig returns the TLS configuration that enforces the pins in envValue on top of base,
// nil when pinning is not configured (default behavior unchanged), or an error when it is
// configured wrongly. Every https client in this package (the CLI/lane client and the
// per-host relay, which is the process that actually holds the host token) goes through it,
// so a pin set for one is enforced by all and a mistake in one is a mistake in all.
func pinConfig(envValue, base string, baseTLS *tls.Config) (*tls.Config, error) {
	if strings.TrimSpace(envValue) == "" {
		return nil, nil
	}
	pins, err := parseSPKIPins(envValue)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, fmt.Errorf("routerstore: %s is set but holds no pin", EnvSPKIPin)
	}
	if !strings.HasPrefix(strings.ToLower(base), "https://") {
		return nil, fmt.Errorf("routerstore: %s is set but the router URL is not https: refusing to talk to it unpinned", EnvSPKIPin)
	}
	return pinnedTLSConfig(baseTLS, pins), nil
}

// pinTransport returns the transport that enforces the pins from the environment value, nil
// when pinning is not configured, or a fail-closed transport when it is configured wrongly.
func pinTransport(envValue, base string) http.RoundTripper {
	cfg, err := pinConfig(envValue, base, &tls.Config{MinVersion: tls.VersionTLS12})
	if err != nil {
		return failClosedTransport{err}
	}
	if cfg == nil {
		return nil
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = cfg
	return t
}
