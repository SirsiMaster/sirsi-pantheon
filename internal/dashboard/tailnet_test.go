package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTailnetEntranceAdmitsOnlySignedInSirsiUsers(t *testing.T) {
	var reached bool
	h := tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true }), nil)
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
		req := httptest.NewRequest(tc.method, "https://m1.tail.ts.net/api/run", nil)
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
	h := tailnetOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) }), nil)
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set(tailnetUserHeader, "owner@sirsi.ai")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("admitted healthz = %d, ACAO %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
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
