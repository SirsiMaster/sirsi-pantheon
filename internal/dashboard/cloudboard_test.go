package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func cloudBoard(t *testing.T, macs map[string]string) *Server {
	t.Helper()
	return New(Config{CloudBoard: true, MacConsoles: macs, RouterFn: func() (RouterSnapshot, error) {
		return RouterSnapshot{Version: "test"}, nil
	}})
}

func serve(s *Server, method, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func withMac(name string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: macCookie, Value: name}) }
}

func TestCloudBoardServesTheRouterSurfaceItself(t *testing.T) {
	s := cloudBoard(t, nil)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/healthz", http.StatusOK},
		{"GET", "/", http.StatusOK},
		{"GET", "/router", http.StatusOK},
		{"GET", "/api/router", http.StatusOK},
		{"GET", "/api/router/v1/snapshot", http.StatusOK},
		{"GET", "/sw.js", http.StatusOK},
		{"GET", "/offline.html", http.StatusOK},
		{"GET", "/manifest.webmanifest", http.StatusOK},
		{"POST", "/api/router", http.StatusMethodNotAllowed},
		// A workstation path with no Mac chosen never runs in the cloud container.
		{"GET", "/api/stats", http.StatusConflict},
		{"POST", "/api/run", http.StatusConflict},
	} {
		if rec := serve(s, tc.method, tc.path, nil); rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestCloudBoardForwardsControlsToTheChosenMac(t *testing.T) {
	var got *http.Request
	var body string
	mac := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_, _ = w.Write([]byte(`{"status":"started"}`))
	}))
	defer mac.Close()
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request reached the Mac that was not chosen")
	}))
	defer other.Close()
	s := cloudBoard(t, map[string]string{"M1": mac.URL, "M5": other.URL})

	rec := serve(s, "POST", "/api/run", func(r *http.Request) {
		withMac("M1")(r)
		r.Body = io.NopCloser(strings.NewReader(`{"action":"scan"}`))
		r.ContentLength = int64(len(`{"action":"scan"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(iapEmailHeader, "accounts.google.com:owner@sirsi.ai")
		r.Header.Set(actorHeader, "forged@elsewhere")
	})
	if rec.Code != http.StatusOK || got == nil {
		t.Fatalf("POST /api/run on M1 = %d %s", rec.Code, rec.Body)
	}
	if got.URL.Path != "/api/run" || body != `{"action":"scan"}` {
		t.Fatalf("forwarded %s %q, want the request unchanged", got.URL.Path, body)
	}
	if a := got.Header.Get(actorHeader); a != "owner@sirsi.ai" {
		t.Fatalf("actor = %q; want the IAP identity, never a client-supplied one", a)
	}
	if got.Header.Get("Cookie") != "" {
		t.Fatalf("board cookies leaked to the Mac console: %q", got.Header.Get("Cookie"))
	}

	// No IAP identity: the forged actor is still discarded, not passed through.
	serve(s, "GET", "/api/stats", func(r *http.Request) { withMac("M1")(r); r.Header.Set(actorHeader, "forged@elsewhere") })
	if a := got.Header.Get(actorHeader); a != "" {
		t.Fatalf("forged actor reached the Mac: %q", a)
	}
	// A cookie naming an unconfigured Mac is not a route anywhere.
	if rec := serve(s, "GET", "/api/stats", withMac("evil.example")); rec.Code != http.StatusConflict {
		t.Fatalf("unknown Mac = %d, want 409", rec.Code)
	}
}

func TestCloudBoardMacSelection(t *testing.T) {
	s := cloudBoard(t, map[string]string{"M1": "https://m1.example.ts.net"})
	rec := serve(s, "GET", "/mac/M1", nil)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), macCookie+"=M1") {
		t.Fatalf("select M1 = %d, Set-Cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	for _, attr := range []string{"HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(rec.Header().Get("Set-Cookie"), attr) {
			t.Fatalf("selection cookie lacks %s: %q", attr, rec.Header().Get("Set-Cookie"))
		}
	}
	if rec := serve(s, "GET", "/mac/nope", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown Mac selection = %d, want 404", rec.Code)
	}
	var list struct {
		Macs     []map[string]string `json:"macs"`
		Selected string              `json:"selected"`
	}
	rec = serve(s, "GET", "/api/macs", withMac("M1"))
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Macs) != 1 || list.Selected != "M1" {
		t.Fatalf("/api/macs = %s (%v)", rec.Body, err)
	}
}

func TestCloudBoardFollowsMacsJoiningAndLeaving(t *testing.T) {
	macs := map[string]string{}
	s := New(Config{CloudBoard: true, MacConsolesFn: func() map[string]string { return macs }})
	if rec := serve(s, "GET", "/mac/m9", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("select before m9 joined = %d, want 404", rec.Code)
	}
	macs = map[string]string{"m9": "https://m9.example.ts.net"}
	if rec := serve(s, "GET", "/mac/m9", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("select after m9 joined = %d, want 303", rec.Code)
	}
	macs = map[string]string{}
	if rec := serve(s, "GET", "/api/stats", withMac("m9")); rec.Code != http.StatusConflict {
		t.Fatalf("control on a Mac that left = %d, want 409", rec.Code)
	}
}

func TestWorkstationDashboardStillServesEverythingOnLoopback(t *testing.T) {
	s := New(Config{})
	if s.srv.Addr != "127.0.0.1:9119" {
		t.Fatalf("default bind = %q, want loopback", s.srv.Addr)
	}
	if rec := serve(s, "GET", "/scan", nil); rec.Code == http.StatusNotFound {
		t.Fatalf("workstation /scan = 404; the cloud routing must apply only to the cloud board")
	}
	if rec := serve(s, "GET", "/api/macs", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("workstation /api/macs = %d; only the hosted board answers it", rec.Code)
	}
}
