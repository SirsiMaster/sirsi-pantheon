package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestHandleRouterWait_MissingAgent(t *testing.T) {
	res, err := handleRouterWait(map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected an error result when 'agent' is missing, got %+v", res)
	}
}

// newTestRouter builds a router rooted at a fresh temp dir with the minimal
// idea-router layout, so waitForInbox can be exercised without the real repo.
func newTestRouter(t *testing.T) *router.Router {
	t.Helper()
	root := t.TempDir()
	rdir := filepath.Join(root, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(rdir, "items"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// PollInbox reads state.json; seed a minimal valid one (mirrors router_test.go).
	if err := os.WriteFile(filepath.Join(rdir, "state.json"), []byte(`{
		"version": 1,
		"active_topics": [],
		"completed_topics": [],
		"last_codex_read": null,
		"last_claude_read": null,
		"rules": {}
	}`), 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}
	r, err := router.New(root)
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return r
}

func TestWaitForInbox_TimesOutCleanly(t *testing.T) {
	r := newTestRouter(t)
	start := time.Now()
	pending, err := waitForInbox(r, "claude", time.Now().Add(200*time.Millisecond))
	if err != nil {
		t.Fatalf("waitForInbox: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected empty inbox, got %d items", len(pending))
	}
	// Must return near the deadline, never hang.
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("waitForInbox blocked too long on an empty inbox: %v", elapsed)
	}
}

func TestWaitForInbox_AlreadyPastDeadline(t *testing.T) {
	r := newTestRouter(t)
	// A deadline in the past returns immediately (one poll), never blocks.
	pending, err := waitForInbox(r, "claude", time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("waitForInbox: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected empty, got %d", len(pending))
	}
}
