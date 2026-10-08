package routerboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testSurfaceHandler() http.Handler {
	b := New("/bin/sirsi", "/tmp/agents.json", "build1234")
	mux := http.NewServeMux()
	NewHandler(b, filepath.Join("ui")).Register(mux)
	return mux
}

func TestRouterSurfaceManifestIsDiscoverable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/router/v1/manifest", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	testSurfaceHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("manifest CORS origin = %q", got)
	}
	var manifest struct {
		Schema    string            `json:"schema"`
		Authority string            `json:"authority"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "router-surface.v1" || manifest.Authority != "ra" {
		t.Fatalf("manifest identity = %+v", manifest)
	}
	if manifest.Endpoints["snapshot"] != "/api/router/v1/snapshot" || manifest.Endpoints["stream"] != "/api/router/v1/stream" {
		t.Fatalf("manifest endpoints = %+v", manifest.Endpoints)
	}
}

func TestRouterSurfaceRejectsUnknownCORSOriginWithoutWildcard(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/router/v1/manifest", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	testSurfaceHandler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unknown origin received CORS permission %q", got)
	}
	if strings.Contains(rec.Header().Get("Access-Control-Allow-Origin"), "*") {
		t.Fatal("router surface must not use wildcard CORS")
	}
}

func TestRouterSurfaceDoesNotInventSnapshotBeforePoll(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/router/v1/snapshot", nil)
	rec := httptest.NewRecorder()
	testSurfaceHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("snapshot status = %d, want 503 before first poll", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no poll completed yet") {
		t.Fatalf("snapshot body = %q", rec.Body.String())
	}
}

func TestRouterSurfaceAliasesTheStandalonePage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/router?surface=nexus", nil)
	rec := httptest.NewRecorder()
	testSurfaceHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Sirsi fleet") {
		t.Fatalf("router page status/body = %d/%q", rec.Code, rec.Body.String()[:min(len(rec.Body.String()), 80)])
	}
}
