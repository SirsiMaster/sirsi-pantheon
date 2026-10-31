package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudBoardServesOnlyTheRouterSurface(t *testing.T) {
	s := New(Config{CloudBoard: true, RouterFn: func() (RouterSnapshot, error) {
		return RouterSnapshot{Version: "test"}, nil
	}})
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/healthz", http.StatusOK},
		{"GET", "/", http.StatusOK},
		{"GET", "/router", http.StatusOK},
		{"GET", "/api/router", http.StatusOK},
		{"GET", "/api/router/v1/snapshot", http.StatusOK},
		{"GET", "/api/router/v1/manifest", http.StatusOK},
		// Workstation endpoints act on the host; the board's host is its container.
		{"GET", "/api/stats", http.StatusNotFound},
		{"GET", "/api/vault/search", http.StatusNotFound},
		{"GET", "/api/ghosts", http.StatusNotFound},
		{"GET", "/scan", http.StatusNotFound},
		{"POST", "/api/run", http.StatusMethodNotAllowed},
		{"POST", "/api/router", http.StatusMethodNotAllowed},
	} {
		rec := httptest.NewRecorder()
		s.handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestWorkstationDashboardStillServesEverythingOnLoopback(t *testing.T) {
	s := New(Config{})
	if s.srv.Addr != "127.0.0.1:9119" {
		t.Fatalf("default bind = %q, want loopback", s.srv.Addr)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, httptest.NewRequest("GET", "/scan", nil))
	if rec.Code == http.StatusNotFound {
		t.Fatalf("workstation /scan = 404; the allowlist must apply only to the cloud board")
	}
}
