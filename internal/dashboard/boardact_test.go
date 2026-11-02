package dashboard

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testAud = "/projects/123/locations/us-central1/services/sirsi-router-board"

func signIAP(t *testing.T, key *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := enc(map[string]string{"alg": "ES256", "kid": kid}) + "." + enc(claims)
	d := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, d[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

type boardFixture struct {
	srv   *Server
	key   *ecdsa.PrivateKey
	calls []string
}

func newBoardFixture(t *testing.T, audience string) *boardFixture {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &boardFixture{key: key}
	f.srv = New(Config{CloudBoard: true, BoardIAPAudience: audience,
		RouterFn: func() (RouterSnapshot, error) { return RouterSnapshot{}, nil },
		BoardActFn: func(_ context.Context, actor, verb string, _ []byte) (any, error) {
			if verb == "nope" {
				return nil, ErrBoardVerb
			}
			f.calls = append(f.calls, actor+" "+verb)
			return map[string]bool{"ok": true}, nil
		}})
	f.srv.iap.fetch = func() (map[string]*ecdsa.PublicKey, error) {
		return map[string]*ecdsa.PublicKey{"k1": &key.PublicKey}, nil
	}
	return f
}

func goodClaims() map[string]any {
	now := time.Now()
	return map[string]any{"iss": iapIssuer, "aud": testAud, "email": "owner@sirsi.ai", "hd": "sirsi.ai",
		"iat": now.Unix(), "exp": now.Add(10 * time.Minute).Unix()}
}

func (f *boardFixture) do(method, path, jwt string, csrf bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://board.example"+path, strings.NewReader(`{"id":"x"}`))
	if jwt != "" {
		req.Header.Set(iapHeader, jwt)
	}
	if csrf {
		req.Header.Set("Origin", "https://board.example")
		req.Header.Set("X-Sirsi-Board", "1")
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.srv.handler.ServeHTTP(rec, req)
	return rec
}

func TestBoardActRequiresVerifiedSirsiIdentity(t *testing.T) {
	f := newBoardFixture(t, testAud)
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	with := func(k string, v any) map[string]any { c := goodClaims(); c[k] = v; return c }
	for _, tc := range []struct {
		name string
		jwt  string
		want int
	}{
		{"good", signIAP(t, f.key, "k1", goodClaims()), http.StatusOK},
		{"missing", "", http.StatusUnauthorized},
		{"wrong audience", signIAP(t, f.key, "k1", with("aud", "/projects/999/x")), http.StatusUnauthorized},
		{"wrong domain", signIAP(t, f.key, "k1", with("email", "owner@gmail.com")), http.StatusUnauthorized},
		{"lookalike domain", signIAP(t, f.key, "k1", with("email", "owner@evilsirsi.ai")), http.StatusUnauthorized},
		{"wrong hd", signIAP(t, f.key, "k1", with("hd", "gmail.com")), http.StatusUnauthorized},
		{"expired", signIAP(t, f.key, "k1", with("exp", time.Now().Add(-time.Hour).Unix())), http.StatusUnauthorized},
		{"wrong issuer", signIAP(t, f.key, "k1", with("iss", "https://evil")), http.StatusUnauthorized},
		{"unknown key", signIAP(t, f.key, "k2", goodClaims()), http.StatusUnauthorized},
		{"forged signature", signIAP(t, other, "k1", goodClaims()), http.StatusUnauthorized},
	} {
		if rec := f.do("POST", boardActPrefix+"close", tc.jwt, true); rec.Code != tc.want {
			t.Errorf("%s: POST close = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
	if len(f.calls) != 1 || f.calls[0] != "board:owner@sirsi.ai close" {
		t.Fatalf("actions run = %v, want exactly the good one attributed to the signed-in account", f.calls)
	}
}

func TestBoardActRefusesCrossOriginAndUnknownVerbs(t *testing.T) {
	f := newBoardFixture(t, testAud)
	jwt := signIAP(t, f.key, "k1", goodClaims())
	if rec := f.do("POST", boardActPrefix+"close", jwt, false); rec.Code != http.StatusForbidden {
		t.Errorf("POST without origin/header = %d, want 403", rec.Code)
	}
	req := httptest.NewRequest("POST", "https://board.example"+boardActPrefix+"close", strings.NewReader(`{}`))
	req.Header.Set(iapHeader, jwt)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("X-Sirsi-Board", "1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.srv.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST from another origin = %d, want 403", rec.Code)
	}
	if rec := f.do("POST", boardActPrefix+"nope", jwt, true); rec.Code != http.StatusNotFound {
		t.Errorf("unknown verb = %d, want 404", rec.Code)
	}
	if rec := f.do("GET", boardActPrefix+"whoami", jwt, false); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "owner@sirsi.ai") {
		t.Errorf("whoami = %d %s", rec.Code, rec.Body)
	}
	// Other POSTs on the board stay refused: the edit API is the only write path.
	if rec := f.do("POST", "/api/router", jwt, true); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/router = %d, want 405", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Fatalf("refused requests ran actions: %v", f.calls)
	}
}

// Fail closed: with no audience configured, even a validly signed sirsi.ai assertion edits nothing.
func TestBoardActWithoutAudienceRefusesEverything(t *testing.T) {
	f := newBoardFixture(t, "")
	if rec := f.do("POST", boardActPrefix+"close", signIAP(t, f.key, "k1", goodClaims()), true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST with unset audience = %d, want 401", rec.Code)
	}
}

// The published key set parses into the keys verify uses; non-P-256 and off-curve entries are dropped.
func TestParseIAPKeys(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	raw, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	bad := append([]byte(nil), raw[33:]...)
	bad[0] ^= 1
	set, _ := json.Marshal(map[string]any{"keys": []map[string]string{
		{"kid": "good", "kty": "EC", "crv": "P-256", "x": b64(raw[1:33]), "y": b64(raw[33:])},
		{"kid": "offcurve", "kty": "EC", "crv": "P-256", "x": b64(raw[1:33]), "y": b64(bad)},
		{"kid": "rsa", "kty": "RSA", "crv": "", "x": "", "y": ""},
	}})
	keys, err := parseIAPKeys(set)
	if err != nil || len(keys) != 1 || keys["good"] == nil {
		t.Fatalf("parseIAPKeys = %v, %v; want only the good key", keys, err)
	}
	v := newIAPVerifier(testAud, "sirsi.ai")
	v.fetch = func() (map[string]*ecdsa.PublicKey, error) { return keys, nil }
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(iapHeader, signIAP(t, key, "good", goodClaims()))
	if email, err := v.verify(req); err != nil || email != "owner@sirsi.ai" {
		t.Fatalf("verify with parsed key = %q, %v", email, err)
	}
}

// The workstation dashboard never mounts the edit API.
func TestWorkstationDashboardHasNoBoardActAPI(t *testing.T) {
	s := New(Config{})
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, httptest.NewRequest("POST", boardActPrefix+"close", strings.NewReader(`{}`)))
	if rec.Code == http.StatusOK {
		t.Fatalf("workstation dashboard served the board edit API")
	}
}
