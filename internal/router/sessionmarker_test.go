package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionAgentMarker_WriteReadRemove(t *testing.T) {
	dir := t.TempDir()
	sessionMarkerDirOverride = dir
	t.Cleanup(func() { sessionMarkerDirOverride = "" })

	const sid = "abc-123-session"
	if got := ReadSessionAgentMarker(sid); got != "" {
		t.Fatalf("expected no marker yet, got %q", got)
	}
	if err := WriteSessionAgentMarker(sid, "claude-pantheon"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := ReadSessionAgentMarker(sid); got != "claude-pantheon" {
		t.Fatalf("read = %q, want claude-pantheon", got)
	}
	// On-disk file named exactly by session id.
	if _, err := os.Stat(filepath.Join(dir, sid)); err != nil {
		t.Fatalf("marker file not at expected path: %v", err)
	}
	if err := RemoveSessionAgentMarker(sid); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := ReadSessionAgentMarker(sid); got != "" {
		t.Fatalf("marker should be gone, got %q", got)
	}
	// Remove is idempotent.
	if err := RemoveSessionAgentMarker(sid); err != nil {
		t.Fatalf("second remove must be a no-op, got %v", err)
	}
}

func TestSessionAgentMarker_EmptyIsNoOp(t *testing.T) {
	dir := t.TempDir()
	sessionMarkerDirOverride = dir
	t.Cleanup(func() { sessionMarkerDirOverride = "" })

	if err := WriteSessionAgentMarker("", "claude-pantheon"); err != nil {
		t.Fatalf("empty session id must be a no-op, got %v", err)
	}
	if err := WriteSessionAgentMarker("sid", ""); err != nil {
		t.Fatalf("empty agent id must be a no-op, got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("no marker should be written for empty inputs, found %d", len(entries))
	}
}

func TestSessionAgentMarker_SanitizeBlocksTraversal(t *testing.T) {
	dir := t.TempDir()
	sessionMarkerDirOverride = dir
	t.Cleanup(func() { sessionMarkerDirOverride = "" })

	// A crafted session id with path separators must be sanitized to a flat name
	// inside the marker dir — never escape it.
	if err := WriteSessionAgentMarker("../../etc/evil", "x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one flat marker file, found %d", len(entries))
	}
	if name := entries[0].Name(); filepath.Base(name) != name || name == "evil" {
		t.Fatalf("session id was not sanitized to a flat in-dir name: %q", name)
	}
}
