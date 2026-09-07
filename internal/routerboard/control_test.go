package routerboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlEnvelopeUsesCanonicalBoardStateAndCapabilities(t *testing.T) {
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 4
	b.payload = []byte(`{"build":"test-build","generated_at":"2026-09-07T12:00:00Z","fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()

	body, version, err := b.SnapshotControl()
	if err != nil {
		t.Fatalf("SnapshotControl: %v", err)
	}
	if version != 4 {
		t.Fatalf("version = %d, want 4", version)
	}
	var got ControlEnvelope
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode control envelope: %v", err)
	}
	if got.Schema != ControlSchema || got.Authority != "canonical-routerstore" {
		t.Fatalf("identity = %q/%q, want %q/canonical-routerstore", got.Schema, got.Authority, ControlSchema)
	}
	if got.State.Build != "test-build" || got.GeneratedAt != got.State.GeneratedAt {
		t.Fatalf("state was not preserved: %+v", got)
	}
	if len(got.Capabilities) != 7 {
		t.Fatalf("capability count = %d, want 7", len(got.Capabilities))
	}
	seen := map[string]bool{}
	for _, capability := range got.Capabilities {
		if capability.Verb == "" || capability.Command == "" || seen[capability.Verb] {
			t.Fatalf("invalid or duplicate capability: %+v", capability)
		}
		seen[capability.Verb] = true
	}
}

func TestSnapshotControlRefusesUnpolledBoard(t *testing.T) {
	b := New("/bin/false", "", "test-build")
	body, version, err := b.SnapshotControl()
	if err != nil {
		t.Fatalf("SnapshotControl: %v", err)
	}
	if body != nil || version != 0 {
		t.Fatalf("unpolled board returned data: version=%d body=%q", version, body)
	}
}

func TestControlEndpointIsReadOnlyAndReturnsTheEnvelope(t *testing.T) {
	b := New("/bin/false", "", "test-build")
	b.mu.Lock()
	b.version = 1
	b.payload = []byte(`{"generated_at":"2026-09-07T12:00:00Z","fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	b.mu.Unlock()
	h := NewHandler(b, t.TempDir())
	mux := http.NewServeMux()
	h.Register(mux)

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/control", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", get.Code, get.Body.String())
	}
	var envelope ControlEnvelope
	if err := json.Unmarshal(get.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("GET body is not control JSON: %v", err)
	}
	if envelope.Schema != ControlSchema || envelope.Authority != "canonical-routerstore" {
		t.Fatalf("GET identity = %q/%q", envelope.Schema, envelope.Authority)
	}

	post := httptest.NewRecorder()
	mux.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/control", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", post.Code)
	}
	if post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST Allow = %q, want GET", post.Header().Get("Allow"))
	}
}
