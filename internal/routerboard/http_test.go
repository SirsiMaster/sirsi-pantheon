package routerboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	if manifest.Schema != "router-board.v1" || manifest.Authority != "ra" {
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

func TestRouterSurfaceSliceRejectsUnknownCORSOriginWithoutWildcard(t *testing.T) {
	// Regression: slice (serving /api/router/v1/ledger and /tasks) used to set
	// Access-Control-Allow-Origin: * unconditionally after surfaceHeaders ran,
	// clobbering the explicit allowlist the manifest/snapshot routes already
	// enforced. The manifest-only test above never exercised this path.
	for _, path := range []string{"/api/router/v1/ledger", "/api/router/v1/tasks"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		testSurfaceHandler().ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("%s: unknown origin received CORS permission %q", path, got)
		}
	}
}

// TestRouterSurfaceFailsClosedWhenProducerPollFails guards the 2026-10-08
// defect: a failed authoritative (ledger) read still bumped the payload
// version, so a dead producer rendered a fabricated all-zero 200 OK board
// instead of a 503 — a dead fleet looked identical to an empty one.
func TestRouterSurfaceFailsClosedWhenProducerPollFails(t *testing.T) {
	tmp := t.TempDir()
	cli := filepath.Join(tmp, "false-cli")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(tmp, "missing-agents.json")

	b := New(cli, agents, "build1234")
	b.Poll(context.Background())
	if b.Valid() {
		t.Fatal("board reports valid after every producer call failed")
	}

	mux := http.NewServeMux()
	NewHandler(b, filepath.Join("ui")).Register(mux)

	for _, path := range []string{"/api/router/v1/snapshot", "/api/router/v1/ledger", "/api/router/v1/tasks", "/api/router/v1/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503 after a failed poll (version=%d)", path, rec.Code, b.Version())
		}
	}
}

// TestRouterSurfaceSnapshotStateIsAtomic guards PR#1042's successor defect:
// Snapshot() and Valid() were separate RLock acquisitions, so a Poll landing
// between them could pair a stale/invalid body with a valid flag from the
// NEXT poll (or vice versa), serving 200 with a fabricated body. SnapshotState
// must read payload+version+valid under one lock.
func TestRouterSurfaceSnapshotStateIsAtomic(t *testing.T) {
	tmp := t.TempDir()
	cli := filepath.Join(tmp, "false-cli")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	b := New(cli, filepath.Join(tmp, "missing-agents.json"), "build1234")
	b.Poll(context.Background())

	_, _, valid := b.SnapshotState()
	if valid {
		t.Fatal("SnapshotState reports valid after every producer call failed")
	}

	// Snapshot() and Valid() must never be called separately by a gating
	// caller again: lock in the contract that SnapshotState is the only way
	// to read payload+version+valid, so the two can't drift apart on a poll
	// landing mid-read.
	for _, path := range []string{"/api/router/v1/snapshot", "/api/router/v1/ledger", "/api/router/v1/tasks", "/api/router/v1/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux := http.NewServeMux()
		NewHandler(b, filepath.Join("ui")).Register(mux)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503 when SnapshotState reports invalid", path, rec.Code)
		}
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
