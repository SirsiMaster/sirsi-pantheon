package main

import (
	"errors"
	"path/filepath"
	"testing"
)

// The mutate gate must fail closed: no registered thread → a clear, actionable
// error, never a silent success (ADR-068 §3). A registered thread → the agent.
func TestRegistrationGate(t *testing.T) {
	t.Run("unregistered, no error → actionable message", func(t *testing.T) {
		r := &registration{claimed: map[string]bool{}}
		if _, err := r.gate(); err == nil {
			t.Fatal("want error when unregistered, got nil (mutate would silently proceed)")
		}
	})
	t.Run("unregistered with recorded error → wraps it", func(t *testing.T) {
		r := &registration{claimed: map[string]bool{}}
		r.setErr(errors.New("SIRSI_AGENT_ID is unset"))
		_, err := r.gate()
		if err == nil || err.Error() == "" {
			t.Fatal("want the recorded registration error surfaced")
		}
	})
	t.Run("registered → returns agent", func(t *testing.T) {
		r := &registration{claimed: map[string]bool{}}
		r.set("thr-abc", "ra")
		got, err := r.gate()
		if err != nil || got != "ra" {
			t.Fatalf("got (%q,%v), want (ra,nil)", got, err)
		}
	})
}

// router_close is session-ownership bound: an item is closable only after THIS
// instance claimed it (ADR-068).
func TestClaimOwnership(t *testing.T) {
	r := &registration{claimed: map[string]bool{}}
	if r.ownsClaim("item-1") {
		t.Fatal("un-claimed item must not be owned")
	}
	r.recordClaim("item-1")
	if !r.ownsClaim("item-1") {
		t.Fatal("claimed item must be owned")
	}
	if r.ownsClaim("item-2") {
		t.Fatal("a different item must not be owned")
	}
}

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
