// server_lift_test.go covers the Mirror web-UI server paths that the
// original server_test.go left out: the Serve loop, the full scan flow,
// browse listings/errors, and partial hashing. Everything is deterministic —
// the only sockets are loopback listeners on random ports (same as
// httptest), and all files live in t.TempDir().
package mirror

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ─── Serve ─────────────────────────────────────────────────────────────────

func TestServe_RespondsAndStopsOnListenerClose(t *testing.T) {
	srv, err := NewServerWith(&platform.Mock{})
	if err != nil {
		t.Fatalf("NewServerWith: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()

	// Poll /api/status until the mux is serving (bounded).
	var resp *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(srv.URL() + "/api/status")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server never came up: %v", err)
	}
	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	resp.Body.Close()
	if status["scanning"] != false {
		t.Errorf("scanning = %v, want false", status["scanning"])
	}

	// Closing the listener unblocks Serve with a non-clean error.
	srv.listener.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Serve returned nil after listener close, want error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after listener close")
	}
}

// ─── handleScan full flow ──────────────────────────────────────────────────

func TestHandleScan_FullFlow(t *testing.T) {
	dir := t.TempDir()
	// Two duplicates + one unique file.
	content := strings.Repeat("mirror-dedup-payload ", 100)
	for _, name := range []string{"a.dat", "b.dat"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "unique.dat"), []byte("solo"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := &Server{platform: &platform.Mock{}}
	body := fmt.Sprintf(`{"paths": [%q], "min_size": 1}`, dir)
	req := httptest.NewRequest("POST", "/api/scan", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleScan(w, req)

	var started map[string]string
	if err := json.NewDecoder(w.Result().Body).Decode(&started); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if started["status"] != "started" {
		t.Fatalf("status = %q, want started", started["status"])
	}

	// Wait for the scan goroutine to finish (bounded).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		scanning := srv.scanning
		srv.mu.Unlock()
		if !scanning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.scanning {
		t.Fatal("scan never completed")
	}
	if srv.result == nil {
		t.Fatal("scan produced no result")
	}
	if len(srv.result.Groups) != 1 {
		t.Errorf("duplicate groups = %d, want 1", len(srv.result.Groups))
	}
}

// ─── handleBrowse listing + error branches ─────────────────────────────────

func TestHandleBrowse_FiltersEntries(t *testing.T) {
	// Build real DirEntry values from a real temp dir, then serve them
	// through the mock so the handler's filtering logic is exercised.
	src := t.TempDir()
	for _, d := range []string{"Visible", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(src, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "plain.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}

	m := &platform.Mock{DirEntries: map[string][]os.DirEntry{"/fake/root": entries}}
	srv := &Server{platform: m}
	req := httptest.NewRequest("GET", "/api/browse?path=/fake/root", nil)
	w := httptest.NewRecorder()

	srv.handleBrowse(w, req)

	var body struct {
		Current string `json:"current"`
		Parent  string `json:"parent"`
		Entries []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Dir  bool   `json:"dir"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Parent != "/fake" {
		t.Errorf("parent = %q, want /fake", body.Parent)
	}
	if len(body.Entries) != 1 {
		t.Fatalf("entries = %d, want 1 (hidden dirs and files filtered): %+v", len(body.Entries), body.Entries)
	}
	if body.Entries[0].Name != "Visible" || !body.Entries[0].Dir {
		t.Errorf("entry = %+v, want Visible dir", body.Entries[0])
	}
	if body.Entries[0].Path != "/fake/root/Visible" {
		t.Errorf("entry path = %q, want /fake/root/Visible", body.Entries[0].Path)
	}
}

// errReadDirPlatform wraps the mock to force ReadDir failures —
// platform.Mock itself never errors on ReadDir.
type errReadDirPlatform struct {
	*platform.Mock
}

func (p *errReadDirPlatform) ReadDir(string) ([]os.DirEntry, error) {
	return nil, fmt.Errorf("permission denied")
}

func TestHandleBrowse_ReadDirError(t *testing.T) {
	srv := &Server{platform: &errReadDirPlatform{&platform.Mock{}}}
	req := httptest.NewRequest("GET", "/api/browse?path=/locked", nil)
	w := httptest.NewRecorder()

	srv.handleBrowse(w, req)

	var body map[string]string
	json.NewDecoder(w.Result().Body).Decode(&body)
	if !strings.Contains(body["error"], "permission denied") {
		t.Errorf("error = %q, want permission denied", body["error"])
	}
}

// ─── handlePickFolder branches ─────────────────────────────────────────────

func TestHandlePickFolder_Canceled(t *testing.T) {
	m := &platform.Mock{PickFolderError: fmt.Errorf("user canceled")}
	srv := &Server{platform: m}
	w := httptest.NewRecorder()
	srv.handlePickFolder(w, httptest.NewRequest("GET", "/api/pick-folder", nil))

	var body map[string]string
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["error"] != "canceled" {
		t.Errorf("error = %q, want canceled", body["error"])
	}
}

func TestHandlePickFolder_TrimsTrailingSlash(t *testing.T) {
	m := &platform.Mock{PickFolderPath: "/picked/dir/"}
	srv := &Server{platform: m}
	w := httptest.NewRecorder()
	srv.handlePickFolder(w, httptest.NewRequest("GET", "/api/pick-folder", nil))

	var body map[string]string
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["path"] != "/picked/dir" {
		t.Errorf("path = %q, want /picked/dir (trailing slash trimmed)", body["path"])
	}
}
