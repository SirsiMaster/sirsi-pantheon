package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ownerRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:9119"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set("Origin", "http://127.0.0.1:9119")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Content-Type", "application/json")
	return r
}
func TestOwnerHTTPWritesAreScopedAndValidated(t *testing.T) {
	calls := 0
	s := New(Config{Port: 9119, RouterActionFn: func(_ context.Context, r RouterActionRequest) (RouterActionResult, error) {
		calls++
		return RouterActionResult{ItemID: r.ItemID, Action: r.Action, Status: "closed"}, nil
	}})
	body := `{"action":"close","item_id":"id","expected":"` + strings.Repeat("a", 64) + `","message":"owner disposition"}`
	cases := []struct {
		name   string
		mutate func(*http.Request)
		body   string
		want   int
	}{
		{"valid", func(r *http.Request) {}, body, 200},
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://sirsi.ai") }, body, 403},
		{"cross-site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, body, 403},
		{"no origin", func(r *http.Request) { r.Header.Del("Origin") }, body, 403},
		{"DNS rebinding", func(r *http.Request) { r.Host = "evil.test:9119"; r.Header.Set("Origin", "http://evil.test:9119") }, body, 403},
		{"nonloopback client", func(r *http.Request) { r.RemoteAddr = "10.0.0.1:111" }, body, 403},
		{"missing token", func(r *http.Request) { r.Header.Del("X-Sirsi-Owner-Token") }, body, 403},
		{"actor injection", func(r *http.Request) {}, strings.TrimSuffix(body, "}") + `,"actor":"ra"}`, 400},
		{"trailing JSON", func(r *http.Request) {}, body + `{}`, 400},
		{"oversized", func(r *http.Request) {}, strings.Replace(body, "owner disposition", strings.Repeat("x", 70000), 1), 400},
		{"wrong media", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, body, 415},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ownerRequest("POST", "/api/router/v1/action", tc.body)
			r.Header.Set("X-Sirsi-Owner-Token", s.ownerCSRF)
			tc.mutate(r)
			w := httptest.NewRecorder()
			s.apiRouterOwnerAction(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
		})
	}
	if calls != 1 {
		t.Fatalf("rejected requests mutated router: calls=%d", calls)
	}
}
func TestOwnerSessionIsNotReadableAcrossOrigins(t *testing.T) {
	s := New(Config{Port: 9119, RouterActionFn: func(context.Context, RouterActionRequest) (RouterActionResult, error) {
		return RouterActionResult{}, nil
	}})
	r := ownerRequest("GET", "/api/router/v1/owner-session", "")
	w := httptest.NewRecorder()
	s.apiRouterOwnerSession(w, r)
	var session map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &session)
	if w.Code != 200 || session["actor"] != "owner" || len(session["token"]) != 64 {
		t.Fatal("no owner session")
	}
	r.Header.Set("Origin", "https://sirsi.ai")
	w = httptest.NewRecorder()
	s.apiRouterOwnerSession(w, r)
	if w.Code != 403 {
		t.Fatal("Nexus read origin received owner token")
	}
}
