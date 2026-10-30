package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentSessionIDProviderBindings(t *testing.T) {
	for _, tc := range []struct {
		name, claude, session, thread, want string
	}{
		{"absent", "", "", "", ""},
		{"claude preserved", "claude-native", "codex-native", "codex-native", "claude-native"},
		{"codex session", "", "codex-native", "", "codex-native"},
		{"codex thread", "", "", "codex-native", "codex-native"},
		{"matching codex aliases", "", "codex-native", "codex-native", "codex-native"},
		{"current codex thread preferred to shared root", "", "parent-root", "current-thread", "current-thread"},
		{"trimmed matching aliases", " ", " native-session ", "native-session", "native-session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(SessionIDEnv, tc.claude)
			t.Setenv("CODEX_SESSION_ID", tc.session)
			t.Setenv("CODEX_THREAD_ID", tc.thread)
			if got := CurrentSessionID(); got != tc.want {
				t.Fatalf("CurrentSessionID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckedCurrentSessionIDPrefersThreadAndFallsBackToRoot(t *testing.T) {
	t.Setenv(SessionIDEnv, "")
	t.Setenv("CODEX_SESSION_ID", "parent-session")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	if sid, err := CheckedCurrentSessionID(); err != nil || sid != "current-thread" {
		t.Fatalf("current Codex thread = %q, %v; want current-thread", sid, err)
	}
	if CurrentSessionID() != "current-thread" {
		t.Fatal("compatibility resolver must use the current thread")
	}
	t.Setenv("CODEX_THREAD_ID", "")
	if sid, err := CheckedCurrentSessionID(); err != nil || sid != "parent-session" {
		t.Fatalf("legacy root-session fallback = %q, %v", sid, err)
	}
	t.Setenv(SessionIDEnv, "explicit-claude-session")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	if sid, err := CheckedCurrentSessionID(); err != nil || sid != "explicit-claude-session" {
		t.Fatalf("existing explicit Claude binding changed: %q %v", sid, err)
	}
}

func TestCodexCurrentThreadDoesNotAdoptRootOrSiblingMarkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(SessionIDEnv, "")
	t.Setenv("CODEX_SESSION_ID", "parent-root")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	for _, marker := range []struct{ id, agent, thread string }{
		{"parent-root", "codex-parent", "parent-thread"},
		{"sibling-thread", "codex-sibling", "sibling-thread-record"},
	} {
		if err := WriteSessionAgentMarker(marker.id, marker.agent); err != nil {
			t.Fatal(err)
		}
		if err := WriteSessionThreadMarker(marker.id, marker.thread); err != nil {
			t.Fatal(err)
		}
	}
	if got := CurrentSessionID(); got != "current-thread" {
		t.Fatalf("current marker key = %q, want current-thread", got)
	}
	if got := ReadSessionAgentMarker(CurrentSessionID()); got != "" {
		t.Fatalf("current thread adopted foreign agent marker %q", got)
	}
	if got := ReadSessionThreadMarker(CurrentSessionID()); got != "" {
		t.Fatalf("current thread adopted foreign thread marker %q", got)
	}
	if err := WriteSessionAgentMarker(CurrentSessionID(), "codex-current"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionThreadMarker(CurrentSessionID(), "registered-current-thread"); err != nil {
		t.Fatal(err)
	}
	if got := ReadSessionAgentMarker(CurrentSessionID()); got != "codex-current" {
		t.Fatalf("current agent marker = %q", got)
	}
	if got := ReadSessionThreadMarker(CurrentSessionID()); got != "registered-current-thread" {
		t.Fatalf("current thread marker = %q", got)
	}
}

func TestCodexSessionUsesSameAgentAndThreadMarkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(SessionIDEnv, "")
	t.Setenv("CODEX_SESSION_ID", "codex-native")
	t.Setenv("CODEX_THREAD_ID", "codex-native")
	if err := WriteSessionAgentMarker(CurrentSessionID(), "codex-apollo"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionThreadMarker(CurrentSessionID(), "thr-native"); err != nil {
		t.Fatal(err)
	}
	if ReadSessionAgentMarker(CurrentSessionID()) != "codex-apollo" || ReadSessionThreadMarker(CurrentSessionID()) != "thr-native" {
		t.Fatal("Codex registration must use the same session key for both marker types")
	}
	if err := RemoveSessionAgentMarker(CurrentSessionID()); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSessionThreadMarker(CurrentSessionID()); err != nil {
		t.Fatal(err)
	}
	if ReadSessionAgentMarker(CurrentSessionID()) != "" || ReadSessionThreadMarker(CurrentSessionID()) != "" {
		t.Fatal("close must remove only this session's markers")
	}
}

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

// The session→thread marker mirrors session→agent: write, read, remove, and
// empty-input no-ops (ADR-062 20b.2).
func TestSessionThreadMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	prev := sessionMarkerDirOverride
	sessionMarkerDirOverride = dir
	t.Cleanup(func() { sessionMarkerDirOverride = prev })

	if ReadSessionThreadMarker("sess-1") != "" {
		t.Fatal("absent marker must read empty")
	}
	if err := WriteSessionThreadMarker("sess-1", "thr-xyz"); err != nil {
		t.Fatal(err)
	}
	if got := ReadSessionThreadMarker("sess-1"); got != "thr-xyz" {
		t.Fatalf("read = %q, want thr-xyz", got)
	}
	// empty session or thread is a no-op, not an error.
	if err := WriteSessionThreadMarker("", "thr-xyz"); err != nil {
		t.Fatalf("empty session must be a no-op: %v", err)
	}
	if err := WriteSessionThreadMarker("sess-2", ""); err != nil {
		t.Fatalf("empty thread must be a no-op: %v", err)
	}
	if ReadSessionThreadMarker("sess-2") != "" {
		t.Fatal("a no-op write must leave no marker")
	}
	if err := RemoveSessionThreadMarker("sess-1"); err != nil {
		t.Fatal(err)
	}
	if ReadSessionThreadMarker("sess-1") != "" {
		t.Fatal("marker must be gone after remove")
	}
	if err := RemoveSessionThreadMarker("sess-1"); err != nil {
		t.Fatalf("remove is idempotent: %v", err)
	}
	// the thread marker lives in its own subdir, not the agent-marker dir.
	_ = WriteSessionAgentMarker("sess-3", "ra")
	_ = WriteSessionThreadMarker("sess-3", "thr-3")
	if ReadSessionAgentMarker("sess-3") != "ra" || ReadSessionThreadMarker("sess-3") != "thr-3" {
		t.Fatal("agent and thread markers must not collide")
	}
}
