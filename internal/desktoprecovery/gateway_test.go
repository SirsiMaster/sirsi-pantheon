package desktoprecovery

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
	g, err := New(Config{Nodes: []Node{{ID: "m1", Address: address, Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": private.Public().(ed25519.PublicKey)}}, Claims: NewMemoryAdmissionStore(), SessionTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {})
	t.Setenv("PANTHEON_TEST_RECOVERY_CAPABILITY", signedCapability(t, private, "m1", "https://m5.example.ts.net"))
	return g
}

func signedCapability(t *testing.T, private ed25519.PrivateKey, nodeID, origin string) string {
	return signedCapabilityAt(t, private, nodeID, origin, "single-use-test", time.Now().Add(time.Minute))
}

func signedCapabilityAt(t *testing.T, private ed25519.PrivateKey, nodeID, origin, nonce string, expiresAt time.Time) string {
	t.Helper()
	payload, err := json.Marshal(CapabilityClaims{KeyID: "test", Login: "owner@example.test", NodeID: nodeID, Origin: origin, Purpose: "pantheon.desktop-recovery", ExpiresAt: expiresAt.Unix(), Nonce: nonce})
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
		if _, err := New(Config{Nodes: []Node{{ID: "m1", Address: address, Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: allowAuth{}, Claims: NewMemoryAdmissionStore()}); err == nil {
			t.Fatalf("accepted unbounded address %q", address)
		}
	}
}

func TestGatewayRequiresAdmissionStore(t *testing.T) {
	if _, err := New(Config{Nodes: []Node{{ID: "m1", Address: "100.88.242.95:5900", Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: allowAuth{}}); err == nil {
		t.Fatal("accepted a recovery gateway without durable admission storage")
	}
}

func TestFileAdmissionStoreSurvivesRestartAndRefusesSymlinkRoot(t *testing.T) {
	root := t.TempDir()
	first, err := NewFileAdmissionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	id := sha256.Sum256([]byte("signed-key-and-nonce"))
	if err := first.Claim(id, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewFileAdmissionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.Claim(id, time.Now().Add(time.Minute)); !errors.Is(err, ErrAdmissionAlreadyUsed) {
		t.Fatalf("claim replay after restart = %v, want ErrAdmissionAlreadyUsed", err)
	}
	link := filepath.Join(t.TempDir(), "claim-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileAdmissionStore(link); err == nil {
		t.Fatal("accepted symlinked admission claim directory")
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

func TestCapabilityAuthorizerRejectsEquivalentNonCanonicalBase64Signature(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	capability := signedCapability(t, private, "m1", "https://m5.example.ts.net")
	parts := strings.Split(capability, ".")
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var nonCanonical string
	for _, candidate := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_" {
		altered := parts[1][:len(parts[1])-1] + string(candidate)
		value, decodeErr := base64.RawURLEncoding.DecodeString(altered)
		if decodeErr == nil && altered != parts[1] && bytes.Equal(value, decoded) {
			nonCanonical = altered
			break
		}
	}
	if nonCanonical == "" {
		t.Fatal("could not construct an equivalent noncanonical base64url signature")
	}
	a := CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": public}}
	req := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery", nil)
	req.Header.Set("Origin", "https://m5.example.ts.net")
	req.Header.Set("Authorization", "Bearer "+parts[0]+"."+nonCanonical)
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{ID: "m1"}); err == nil {
		t.Fatal("accepted equivalent noncanonical base64url signature")
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
	body := readBody(t, resp)
	if err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(body, "Recovery admission") || !strings.Contains(body, "Need a resolution?") || !strings.Contains(body, "Sirsi <span>Pantheon</span>") {
		t.Fatalf("entry page = %v, %v", resp, err)
	}
	invalid := url.Values{"capability": {"not-a-capability"}}.Encode()
	invalidRequest, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", strings.NewReader(invalid))
	invalidRequest.Header.Set("Origin", "https://m5.example.ts.net")
	invalidRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalidRequest.Header.Set("Accept", "text/html")
	invalidResponse, invalidErr := noRedirect.Do(invalidRequest)
	if invalidErr != nil || invalidResponse.StatusCode != http.StatusUnauthorized || !strings.Contains(readBody(t, invalidResponse), "Recovery needs a fresh admission.") {
		t.Fatalf("invalid form guidance = %v, %v", invalidResponse, invalidErr)
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

func TestAdmissionReplayRetainedThroughSignedExpiryAndSessionDeadlineCapped(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	capabilityExpires := now.Add(20 * time.Second)
	g, err := New(Config{
		Nodes:      []Node{{ID: "m1", Address: "100.88.242.95:5900", Origins: []string{"https://m5.example.ts.net"}}},
		Authorizer: CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": public}, Now: func() time.Time { return now }},
		Claims:     NewMemoryAdmissionStore(),
		SessionTTL: time.Minute,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	capability := signedCapabilityAt(t, private, "m1", "https://m5.example.ts.net", "ttl-bound-nonce", capabilityExpires)
	ts := httptest.NewServer(g.Handler())
	defer ts.Close()
	admit := func() *http.Response {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
		req.Header.Set("Origin", "https://m5.example.ts.net")
		req.Header.Set("Authorization", "Bearer "+capability)
		resp, requestErr := http.DefaultClient.Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return resp
	}
	first := admit()
	if first.StatusCode != http.StatusCreated || len(first.Cookies()) != 1 || !first.Cookies()[0].Expires.Equal(capabilityExpires) {
		t.Fatalf("signed expiry did not cap session: status=%d cookies=%#v", first.StatusCode, first.Cookies())
	}
	first.Body.Close()
	now = now.Add(3 * time.Second)
	replay := admit()
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusConflict {
		t.Fatalf("replay after shorter session TTL = %d, want %d", replay.StatusCode, http.StatusConflict)
	}
}

func TestCapabilityLifetimeAndDuplicateTransportsAreRejected(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	a := CapabilityAuthorizer{PublicKeys: map[string]ed25519.PublicKey{"test": public}, Now: func() time.Time { return now }}
	req := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery", strings.NewReader("capability=duplicate"))
	req.Header.Set("Origin", "https://m5.example.ts.net")
	req.Header.Set("Authorization", "Bearer "+signedCapabilityAt(t, private, "m1", "https://m5.example.ts.net", "long-lived", now.Add(maxCapabilityTTL+time.Second)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := admissionRequest(req); err == nil {
		t.Fatal("accepted duplicate recovery credential transports")
	}
	req = httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery", nil)
	req.Header.Set("Origin", "https://m5.example.ts.net")
	req.Header.Set("Authorization", "Bearer "+signedCapabilityAt(t, private, "m1", "https://m5.example.ts.net", "long-lived", now.Add(maxCapabilityTTL+time.Second)))
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{ID: "m1"}); err == nil {
		t.Fatal("accepted capability beyond maximum lifetime")
	}
}

func TestNodeIDRejectsHTMLAndRouteDelimiters(t *testing.T) {
	for _, id := range []string{`m1"><script>`, "m1/entry", "-m1", "m1-", strings.Repeat("m", 65)} {
		if validNodeID(id) {
			t.Fatalf("unsafe node id accepted: %q", id)
		}
	}
	for _, id := range []string{"m1", "m1-recovery", "m5a"} {
		if !validNodeID(id) {
			t.Fatalf("safe node id rejected: %q", id)
		}
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
