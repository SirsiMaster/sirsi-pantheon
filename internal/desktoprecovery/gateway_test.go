package desktoprecovery

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/websocket"
)

type allowAuth struct{}

func (allowAuth) AuthorizeRecovery(_ context.Context, _ *http.Request, _ Node) (Principal, error) {
	return Principal{Login: "owner@example.test"}, nil
}

func testGateway(t *testing.T, address string) *Gateway {
	t.Helper()
	g, err := New(Config{Nodes: []Node{{ID: "m1", Address: address, Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: allowAuth{}, SessionTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGatewayRejectsUnboundedDestinations(t *testing.T) {
	for _, address := range []string{"example.com:5900", "8.8.8.8:5900", "100.88.242.95:22", "100.88.242.95"} {
		if _, err := New(Config{Nodes: []Node{{ID: "m1", Address: address, Origins: []string{"https://m5.example.ts.net"}}}, Authorizer: allowAuth{}}); err == nil {
			t.Fatalf("accepted unbounded address %q", address)
		}
	}
}

func TestTailnetHeaderAuthorizerRequiresLoopbackAndAllowlist(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-recovery-password"), 10)
	if err != nil {
		t.Fatal(err)
	}
	a := TailnetHeaderAuthorizer{AllowedLogins: map[string]struct{}{"owner@example.test": {}}, PasswordHashes: map[string]string{"owner@example.test": string(hash)}}
	req := httptest.NewRequest(http.MethodPost, "https://m5.example.ts.net/recovery", nil)
	req.RemoteAddr = "100.88.242.1:443"
	req.Header.Set("Tailscale-User-Login", "owner@example.test")
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{}); err == nil {
		t.Fatal("accepted spoofable non-loopback identity header")
	}
	req.RemoteAddr = "127.0.0.1:12345"
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{}); err == nil {
		t.Fatal("forged local header admitted without independent authentication")
	}
	req.SetBasicAuth("owner@example.test", "wrong")
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{}); err == nil {
		t.Fatal("wrong password admitted")
	}
	req.SetBasicAuth("owner@example.test", "synthetic-recovery-password")
	if _, err := a.AuthorizeRecovery(context.Background(), req, Node{}); err != nil {
		t.Fatalf("rejected loopback allowlisted identity: %v", err)
	}
}

func TestAdmissionRequiresExactOriginAndUsesCookieNotURLSecret(t *testing.T) {
	g := testGateway(t, "100.88.242.95:5900")
	ts := httptest.NewServer(g.Handler())
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
	req.Header.Set("Origin", "https://attacker.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong-origin admission = %v, %v", resp, err)
	}
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/recovery/v1/nodes/m1/sessions", nil)
	req.Header.Set("Origin", "https://m5.example.ts.net")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("admission = %v, %v", resp, err)
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

func TestFreshAndExpiredBrowserEntry(t *testing.T) {
	g := testGateway(t, "100.88.242.95:5900")
	handler := g.Handler()
	for _, expired := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/recovery/v1/nodes/m1/client", nil)
		if expired {
			admission := httptest.NewRequest(http.MethodPost, "/recovery/v1/nodes/m1/sessions", nil)
			admission.Header.Set("Origin", "https://m5.example.ts.net")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, admission)
			req.AddCookie(w.Result().Cookies()[0])
			g.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Recover your desktop") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("entry failed: %d", w.Code)
		}
	}
}
