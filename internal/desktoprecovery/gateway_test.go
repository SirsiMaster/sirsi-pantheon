package desktoprecovery

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

type allowAuth struct{}

func (allowAuth) AuthorizeRecovery(_ context.Context, _ *http.Request, _ Node) (Principal, error) {
	return Principal{Login: "owner@example.test"}, nil
}

func testGateway(t *testing.T, address string) *Gateway {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(Config{Nodes: []Node{{ID: "m1", Address: address, Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": private.Public().(ed25519.PublicKey)}}, SessionTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {})
	t.Setenv("PANTHEON_TEST_RECOVERY_CAPABILITY", signedCapability(t, private, "m1", "https://m5.example.ts.net"))
	return g
}

func signedCapability(t *testing.T, private ed25519.PrivateKey, nodeID, origin string) string {
	t.Helper()
	payload, err := json.Marshal(CapabilityClaims{KeyID: "test", Login: "owner@example.test", NodeID: nodeID, Origin: origin, Purpose: "pantheon.desktop-recovery", ExpiresAt: time.Now().Add(time.Minute).Unix(), Nonce: "single-use-test"})
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payload))
}

func testCapability(t *testing.T) string {
	t.Helper()
	return os.Getenv("PANTHEON_TEST_RECOVERY_CAPABILITY")
}

func TestGatewayRejectsUnboundedDestinations(t *testing.T) {
	for _, address := range []string{"example.com:5900", "8.8.8.8:5900", "100.88.242.95:22", "100.88.242.95"} {
		if _, err := New(Config{Nodes: []Node{{ID: "m1", Address: address, Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: allowAuth{}}); err == nil {
			t.Fatalf("accepted unbounded address %q", address)
		}
	}
}

func TestCapabilityAuthorizerRejectsForgedTailnetHeader(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": public}}
	req := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery", nil)
	req.Header.Set("Origin", "https://m5.example.ts.net")
	req.Header.Set("Tailscale-User-Login", "owner@example.test")
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{ID: "m1"}); err == nil {
		t.Fatal("accepted a forged Tailscale identity header without a signed capability")
	}
	req.Header.Set("Authorization", "Bearer "+signedCapability(t, private, "m1", "https://m5.example.ts.net"))
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{ID: "m1"}); err != nil {
		t.Fatalf("rejected valid signed recovery capability: %v", err)
	}
}

func TestAdmissionRequiresExactOriginAndUsesCookieNotURLSecret(t *testing.T) {
	g := testGateway(t, "100.88.242.95:5900")
	ts := httptest.NewServer(g.Handler())
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
	req.Header.Set("Origin", "https://attacker.example")
	req.Header.Set("Authorization", "Bearer "+testCapability(t))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong-origin admission = %v, %v", resp, err)
	}
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
	req.Header.Set("Origin", "https://m5.example.ts.net")
	req.Header.Set("Authorization", "Bearer "+testCapability(t))
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("admission = %v, %v", resp, err)
	}
	replay, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
	replay.Header.Set("Origin", "https://m5.example.ts.net")
	replay.Header.Set("Authorization", "Bearer "+testCapability(t))
	replayResponse, replayErr := http.DefaultClient.Do(replay)
	if replayErr != nil || replayResponse.StatusCode != http.StatusConflict {
		t.Fatalf("replayed capability = %v, %v", replayResponse, replayErr)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || strings.Contains(resp.Request.URL.RawQuery, cookies[0].Value) {
		t.Fatalf("recovery session was not a secure HttpOnly cookie: %#v", cookies)
	}
	client, _ := http.NewRequest(http.MethodGet, ts.URL+"/recovery/v1/nodes/m1/client", nil)
	client.AddCookie(cookies[0])
	noRedirect := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err = noRedirect.Do(client)
	if err != nil || resp.StatusCode != http.StatusFound || strings.Contains(resp.Header.Get("Location"), cookies[0].Value) {
		t.Fatalf("client redirect leaked cookie or failed: %v, %v", resp, err)
	}
}

func TestFreshClientEntryGuidesOperatorAndFormAdmission(t *testing.T) {
	g := testGateway(t, "100.88.242.95:5900")
	ts := httptest.NewServer(g.Handler())
	defer ts.Close()
	noRedirect := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Get(ts.URL + "/recovery/v1/nodes/m1/client")
	if err != nil || resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/recovery/v1/nodes/m1/entry" {
		t.Fatalf("fresh recovery client entry = %v, %v", resp, err)
	}
	resp, err = noRedirect.Get(ts.URL + "/recovery/v1/nodes/m1/entry")
	if err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "Recovery admission") {
		t.Fatalf("entry page = %v, %v", resp, err)
	}
	form := url.Values{"capability": {testCapability(t)}}.Encode()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", strings.NewReader(form))
	req.Header.Set("Origin", "https://m5.example.ts.net")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	resp, err = noRedirect.Do(req)
	if err != nil || resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/recovery/v1/nodes/m1/client" || len(resp.Cookies()) != 1 {
		t.Fatalf("form admission = %v, %v", resp, err)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGatewayProxiesOnlyAdmittedNodeAndCleansSessionOnDisconnect(t *testing.T) {
	rfbListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer rfbListener.Close()
	_, port, _ := net.SplitHostPort(rfbListener.Addr().String())
	// Loopback is not a production node address. The test uses an injected dialer
	// while preserving an allowlisted Tailscale address in the public config.
	g := testGateway(t, "100.88.242.95:5900")
	g.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}
	go func() {
		conn, err := rfbListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("RFB 003.889\n"))
		_, _ = io.Copy(conn, conn)
	}()
	ts := httptest.NewServer(g.Handler())
	defer ts.Close()
	admit, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
	admit.Header.Set("Origin", "https://m5.example.ts.net")
	admit.Header.Set("Authorization", "Bearer "+testCapability(t))
	resp, err := http.DefaultClient.Do(admit)
	if err != nil {
		t.Fatal(err)
	}
	cookie := resp.Cookies()[0]
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/recovery/v1/nodes/m1/ws"
	config, err := websocket.NewConfig(wsURL, "https://m5.example.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	config.Header.Set("Cookie", cookie.String())
	ws, err := websocket.DialConfig(config)
	if err != nil {
		t.Fatalf("websocket admission failed: %v", err)
	}
	buf := make([]byte, 12)
	if _, err := io.ReadFull(ws, buf); err != nil || string(buf) != "RFB 003.889\n" {
		t.Fatalf("RFB banner = %q, %v", buf, err)
	}
	_ = ws.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		count := len(g.sessions)
		g.mu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("recovery session survived disconnect")
}

func TestCapabilityReplayUsesVerifiedNonceUntilExpiry(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	claims := CapabilityClaims{KeyID: "test", Login: "owner@example.test", NodeID: "m1", Origin: "https://m5.example.ts.net", Purpose: "pantheon.desktop-recovery", ExpiresAt: now.Add(10 * time.Minute).Unix(), Nonce: "nonce-one"}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(payload []byte) string {
		return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payload))
	}
	token := sign(payload)
	g, err := New(Config{Nodes: []Node{{ID: "m1", Address: "100.88.242.95:5900", Origins: []string{claims.Origin}}}, Authorizer: CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": public}, Now: func() time.Time { return now }}, Now: func() time.Time { return now }, SessionTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	admit := func(token string) int {
		r := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery/v1/nodes/m1/sessions", nil)
		r.Header.Set("Origin", claims.Origin)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		g.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if got := admit(token); got != http.StatusCreated {
		t.Fatalf("first admission: %d", got)
	}
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, token[len(token)-1])
	alternate := token[:len(token)-1] + string(alphabet[last+1])
	parts := strings.Split(token, ".")
	altParts := strings.Split(alternate, ".")
	canonicalBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
	alternateBytes, _ := base64.RawURLEncoding.DecodeString(altParts[1])
	if string(canonicalBytes) != string(alternateBytes) {
		t.Fatal("fixture must preserve signature bytes")
	}
	if got := admit(alternate); got != http.StatusUnauthorized {
		t.Fatalf("noncanonical signature admission: %d", got)
	}
	if got := admit(parts[0] + ".\n" + parts[1]); got != http.StatusUnauthorized {
		t.Fatalf("newline encoding admission: %d", got)
	}
	// A second valid signature over differently formatted JSON is the same nonce.
	if got := admit(sign(append([]byte(" "), payload...))); got != http.StatusConflict {
		t.Fatalf("semantic nonce replay: %d", got)
	}
	// Disconnect and cookie expiry must not clear the capability replay fence.
	g.mu.Lock()
	clear(g.sessions)
	g.mu.Unlock()
	now = now.Add(2 * time.Minute)
	if got := admit(token); got != http.StatusConflict {
		t.Fatalf("replay after session expiry/disconnect: %d", got)
	}
	now = now.Add(8 * time.Minute)
	if got := admit(token); got != http.StatusUnauthorized {
		t.Fatalf("expired capability: %d", got)
	}
}

func TestCapabilityLifetimeAndCredentialBounds(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	for _, tc := range []struct {
		name     string
		lifetime time.Duration
		status   int
	}{
		{"shorter than cookie", 30 * time.Second, http.StatusCreated},
		{"maximum", maxSessionTTL, http.StatusCreated},
		{"too long", maxSessionTTL + time.Second, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := CapabilityClaims{KeyID: "test", Login: "owner@example.test", NodeID: "m1", Origin: "https://m5.example.ts.net", Purpose: "pantheon.desktop-recovery", Nonce: tc.name, ExpiresAt: now.Add(tc.lifetime).Unix()}
			payload, _ := json.Marshal(claims)
			token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payload))
			g, err := New(Config{Nodes: []Node{{ID: "m1", Address: "100.88.242.95:5900", Origins: []string{claims.Origin}}}, Authorizer: CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": public}, Now: func() time.Time { return now }}, Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery/v1/nodes/m1/sessions", nil)
			req.Header.Set("Origin", claims.Origin)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			g.Handler().ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("admission=%d", w.Code)
			}
			for _, cookie := range w.Result().Cookies() {
				if cookie.Expires.After(time.Unix(claims.ExpiresAt, 0)) {
					t.Fatal("cookie outlives signed authority")
				}
			}
		})
	}
	for _, tc := range []struct{ name, body, header, query string }{
		{"header and form", "capability=second", "Bearer first", ""},
		{"duplicate form", "capability=first&capability=second", "", ""},
		{"query credential", "capability=first", "", "?capability=second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery"+tc.query, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if _, err := admissionRequest(req); err == nil {
				t.Fatal("ambiguous or URL credential accepted")
			}
		})
	}
}
