package dashboard

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fromProxy is a request as the local tailscale proxy delivers it.
func fromProxy(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = "127.0.0.1:40000"
	return r
}

func TestTailnetEntranceAdmitsOnlySignedInSirsiUsers(t *testing.T) {
	var reached bool
	h := tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true }), nil, true)
	for _, tc := range []struct {
		name, login, method, site, origin string
		want                              int
	}{
		{"sirsi user reads", "owner@sirsi.ai", "GET", "", "", http.StatusOK},
		{"case-insensitive domain", "Owner@SIRSI.AI", "GET", "", "", http.StatusOK},
		{"no person (tagged node, keyboard-less peer)", "", "GET", "", "", http.StatusForbidden},
		{"other domain", "someone@gmail.com", "GET", "", "", http.StatusForbidden},
		{"look-alike domain", "x@sirsi.ai.evil.com", "GET", "", "", http.StatusForbidden},
		{"control from its own page", "owner@sirsi.ai", "POST", "same-origin", "", http.StatusOK},
		{"control from another site", "owner@sirsi.ai", "POST", "cross-site", "", http.StatusForbidden},
		{"control with own Origin, no fetch metadata", "owner@sirsi.ai", "POST", "", "https://m1.tail.ts.net", http.StatusOK},
		{"control with foreign Origin", "owner@sirsi.ai", "POST", "", "https://evil.example", http.StatusForbidden},
		{"control with no provenance at all", "owner@sirsi.ai", "POST", "", "", http.StatusForbidden},
	} {
		reached = false
		req := fromProxy(tc.method, "https://m1.tail.ts.net/api/run")
		if tc.login != "" {
			req.Header.Set(tailnetUserHeader, tc.login)
		}
		if tc.site != "" {
			req.Header.Set("Sec-Fetch-Site", tc.site)
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want || reached != (tc.want == http.StatusOK) {
			t.Errorf("%s: code %d reached %v, want %d", tc.name, rec.Code, reached, tc.want)
		}
	}
}

func TestTailnetHealthzIsReadableCrossOriginOnlyWhenAdmitted(t *testing.T) {
	h := tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) }), nil, true)
	req := fromProxy("GET", "/healthz")
	req.Header.Set(tailnetUserHeader, "owner@sirsi.ai")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("admitted healthz = %d, ACAO %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, fromProxy("GET", "/healthz"))
	if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unadmitted healthz = %d, ACAO %q; the fallback must not read it as live", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestKeyboardPortIsUnchangedAndBoardListsMacs(t *testing.T) {
	s := New(Config{})
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/macs", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a Mac console answers /api/macs (%d); only the hosted board lists Macs", rec.Code)
	}
	b := New(Config{CloudBoard: true, Macs: map[string]string{"M1": "https://m1.tail.ts.net"}})
	rec = httptest.NewRecorder()
	b.handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/macs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("board /api/macs = %d", rec.Code)
	}
	for _, p := range []string{"/api/run", "/mac/M1", "/classic"} {
		rec = httptest.NewRecorder()
		b.handler.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("board %s = %d; the board never relays Mac controls", p, rec.Code)
		}
	}
}

// SSA 050808 (1): the header-trusting handler must not believe anything that
// isn't the local proxy's single stamp.
func TestTailnetEntranceRefusesUnstampedProvenance(t *testing.T) {
	h := tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}), nil, true)
	for _, tc := range []struct {
		name   string
		remote string
		logins []string
	}{
		{"peer that is not the local proxy", "100.88.242.95:5000", []string{"owner@sirsi.ai"}},
		{"two identity headers", "127.0.0.1:40000", []string{"owner@sirsi.ai", "other@sirsi.ai"}},
		{"blank identity header", "127.0.0.1:40000", []string{"   "}},
	} {
		req := fromProxy("GET", "/api/router")
		req.RemoteAddr = tc.remote
		for _, l := range tc.logins {
			req.Header.Add(tailnetUserHeader, l)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: code %d, want 403", tc.name, rec.Code)
		}
	}
}

func TestTailnetWhoamiReportsTheAdmittedSignIn(t *testing.T) {
	h := tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("whoami reached the console") }), nil, true)
	req := fromProxy("GET", "/whoami")
	req.Header.Set(tailnetUserHeader, "Owner@Sirsi.ai")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "owner@sirsi.ai" {
		t.Fatalf("whoami = %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, fromProxy("GET", "/whoami"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unadmitted whoami = %d", rec.Code)
	}
}

// SSA 050808 (2): a tailnet port that is also a plain console port would let
// `tailscale serve` reach the unguarded handler.
func TestTailnetPortMayNotShareAnUnguardedDoor(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		bad  bool
	}{
		{"own port", Config{Port: 9119, TailnetPort: 9120}, false},
		{"socket", Config{Port: 9119, TailnetSocket: "/x/c.sock"}, false},
		{"same as keyboard port", Config{Port: 9119, TailnetPort: 9119}, true},
		{"same as an alias port", Config{Port: 9119, AltPorts: []int{8734}, TailnetPort: 8734}, true},
		{"port and socket together", Config{Port: 9119, TailnetPort: 9120, TailnetSocket: "/x/c.sock"}, true},
	} {
		if err := tailnetConfigErr(tc.cfg); (err != nil) != tc.bad {
			t.Errorf("%s: err %v, want refused=%v", tc.name, err, tc.bad)
		}
	}
	s := New(Config{Port: freePort(t), AltPorts: []int{8734}, TailnetPort: 8734})
	if err := s.Start(); err == nil || !strings.Contains(err.Error(), "unguarded") {
		_ = s.Stop()
		t.Fatalf("Start with a colliding tailnet port = %v, want refusal", err)
	}
}

func TestStartFailsWhenTheTailnetEntranceCannotBind(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	primary := freePort(t)
	s := New(Config{Port: primary, TailnetPort: busy.Addr().(*net.TCPAddr).Port})
	err = s.Start()
	if err == nil {
		_ = s.Stop()
		t.Fatal("console started without its tailnet entrance")
	}
	if !strings.Contains(err.Error(), "tailnet entrance") {
		t.Fatalf("Start error %q is not the tailnet bind failure", err)
	}
	if s.IsRunning() {
		t.Fatal("reported running without the tailnet entrance")
	}
	// The primary listener was released, not left serving.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(primary)))
	if err != nil {
		t.Fatalf("primary port still held after the failed start: %v", err)
	}
	_ = l.Close()
}

func TestTailnetSocketIsPrivateAndServesTheEntrance(t *testing.T) {
	// Short path: macOS caps Unix socket paths near 104 bytes.
	base, err := os.MkdirTemp("/tmp", "sct")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	open := filepath.Join(base, "open")
	if err := os.Mkdir(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := listenTailnet(Config{TailnetSocket: filepath.Join(open, "c.sock")}); err == nil {
		t.Fatal("socket accepted in a group/world-readable directory")
	}
	sock := filepath.Join(base, "run", "c.sock")
	l, err := listenTailnet(Config{TailnetSocket: sock})
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket stat %v / %v, want 0600", fi, err)
	}
	srv := &http.Server{Handler: tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") }), nil, false)}
	go func() { _ = srv.Serve(l) }()
	defer srv.Close()
	c := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	get := func(login string) int {
		req, _ := http.NewRequest("GET", "http://console/healthz", nil)
		if login != "" {
			req.Header.Set(tailnetUserHeader, login)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := get("owner@sirsi.ai"); got != http.StatusOK {
		t.Fatalf("socket entrance admitted sirsi user = %d", got)
	}
	if got := get(""); got != http.StatusForbidden {
		t.Fatalf("socket entrance without a sign-in = %d", got)
	}
	// A rerun replaces its own stale socket but never a non-socket file.
	_ = srv.Close()
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenTailnet(Config{TailnetSocket: sock}); err == nil {
		t.Fatal("listenTailnet replaced a regular file")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
