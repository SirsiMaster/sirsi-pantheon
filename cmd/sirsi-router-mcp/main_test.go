package main

import (
	"path/filepath"
	"testing"
)

// resolveAgent must prefer an explicit arg, fall back to SIRSI_AGENT_ID, and
// error rather than guess an inbox that is not the caller's (ADR-068: one
// identity per instance).
func TestResolveAgent(t *testing.T) {
	t.Run("arg wins over env", func(t *testing.T) {
		t.Setenv("SIRSI_AGENT_ID", "from-env")
		got, err := resolveAgent(map[string]interface{}{"agent": "from-arg"})
		if err != nil || got != "from-arg" {
			t.Fatalf("got (%q,%v), want (from-arg,nil)", got, err)
		}
	})
	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("SIRSI_AGENT_ID", "ra")
		got, err := resolveAgent(map[string]interface{}{})
		if err != nil || got != "ra" {
			t.Fatalf("got (%q,%v), want (ra,nil)", got, err)
		}
	})
	t.Run("neither → error, never a guess", func(t *testing.T) {
		t.Setenv("SIRSI_AGENT_ID", "")
		got, err := resolveAgent(map[string]interface{}{"agent": "  "})
		if err == nil {
			t.Fatalf("want error when no agent, got %q", got)
		}
	})
}

// resolveRoots must honor SIRSI_ROUTER_REPO (so a client can spawn the server
// outside the repo tree) and derive routerRoot as <repo>/.agents/idea-router.
func TestResolveRootsHonorsEnvOverride(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_REPO", "/tmp/some-repo")
	repoRoot, routerRoot, err := resolveRoots()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoRoot != "/tmp/some-repo" {
		t.Fatalf("repoRoot = %q, want /tmp/some-repo", repoRoot)
	}
	if want := filepath.Join("/tmp/some-repo", ".agents", "idea-router"); routerRoot != want {
		t.Fatalf("routerRoot = %q, want %q", routerRoot, want)
	}
}
