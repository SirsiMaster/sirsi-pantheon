package dashboard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMemoryVitalsExplicitReadRetainsRankedAndEmptyWire(t *testing.T) {
	for _, top := range []string{`[{"name":"fixture-worker","pid":123,"rss_bytes":2097152}]`, `[]`} {
		t.Run(top, func(t *testing.T) {
			calls := 0
			body := `{"command":"sirsi vitals","total_bytes":48,"used_bytes":32,"free_bytes":16,"pressure":"normal","pressure_source":"fixture","swap_used_bytes":2,"top":` + top + `}`
			s := New(Config{VitalsFn: func() ([]byte, error) { calls++; return []byte(body), nil }})
			if calls != 0 {
				t.Fatal("construction sampled memory")
			}
			stats := httptest.NewRecorder()
			s.handler.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/api/stats", nil))
			if calls != 0 {
				t.Fatal("periodic stats requested ranked diagnostics")
			}
			w := httptest.NewRecorder()
			s.handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/vitals", nil))
			if w.Code != http.StatusOK || calls != 1 || strings.TrimSpace(w.Body.String()) != body || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("explicit read=%d %s calls=%d", w.Code, w.Body, calls)
			}
		})
	}
}

func TestMemoryVitalsUnavailableErrorAndMethodRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		producer     func() ([]byte, error)
		want         int
	}{
		{"unavailable", http.MethodGet, nil, http.StatusServiceUnavailable},
		{"read-failed", http.MethodGet, func() ([]byte, error) { return nil, errors.New("PRIVATE_PATH_CANARY") }, http.StatusInternalServerError},
		{"malformed", http.MethodGet, func() ([]byte, error) { return []byte("not json"), nil }, http.StatusInternalServerError},
		{"mutation-refused", http.MethodPost, func() ([]byte, error) { t.Fatal("POST sampled memory"); return nil, nil }, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{VitalsFn: tc.producer})
			w := httptest.NewRecorder()
			s.handler.ServeHTTP(w, httptest.NewRequest(tc.method, "/api/vitals", nil))
			if w.Code != tc.want || strings.Contains(w.Body.String(), "PRIVATE_PATH_CANARY") {
				t.Fatalf("refusal=%d %s", w.Code, w.Body)
			}
		})
	}
}
