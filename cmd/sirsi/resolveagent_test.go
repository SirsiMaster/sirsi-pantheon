package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveCurrentAgentIgnoresUndeclaredMarker: a session marker naming an
// agent no longer declared in agents.json (a reverted verification stub) must
// not resolve — dispatch would refuse every write. A declared marker still
// resolves. Both directions (A35).
func TestResolveCurrentAgentIgnoresUndeclaredMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-1")
	root := t.TempDir()
	agents := `{"agents":{"ra":{"id":"ra","type":"claude","cwd":"/x","workstream":"router","wake":{"mechanism":"none"}}}}`
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	markerDir := filepath.Join(home, ".claude", "run", "agent-by-session")
	if err := os.MkdirAll(markerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mark := func(agent string) {
		if err := os.WriteFile(filepath.Join(markerDir, "sess-1"), []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mark("verify-m1-1790459290")
	if got, how := resolveCurrentAgent(root, ""); got != "" || !strings.Contains(how, "verify-m1-1790459290") {
		t.Fatalf("undeclared marker resolved: got %q (%s)", got, how)
	}

	mark("ra")
	if got, how := resolveCurrentAgent(root, ""); got != "ra" || how != "session marker" {
		t.Fatalf("declared marker: got %q (%s), want ra (session marker)", got, how)
	}

	// Explicit sources are never second-guessed here; dispatch judges them.
	if got, _ := resolveCurrentAgent(root, "anything"); got != "anything" {
		t.Fatalf("flag override: got %q", got)
	}
}
