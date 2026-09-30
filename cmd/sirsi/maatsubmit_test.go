package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSubmitPolicy(t *testing.T) {
	cases := []struct {
		name          string
		repo          string
		requester     string
		wantDeterm    string
		wantWhyHasAny string
	}{
		{"policy match", "SirsiMaster/sirsi-hermes", "hermes", "grant", "matches repo policy"},
		{"policy mismatch", "SirsiMaster/sirsi-hermes", "claude-pantheon", "refuse", "not in repo policy"},
		{"photon policy match", "SirsiMaster/sirsi-photon", "hermes", "grant", "matches repo policy"},
		{"unlisted repo", "SirsiMaster/sirsi-pantheon", "claude-pantheon", "grant", "no policy defined"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := checkSubmitPolicy(c.repo, c.requester)
			if got != c.wantDeterm {
				t.Errorf("determination = %q, want %q", got, c.wantDeterm)
			}
			if !strings.Contains(why, c.wantWhyHasAny) {
				t.Errorf("why = %q, want substring %q", why, c.wantWhyHasAny)
			}
		})
	}
}

func TestResolveSubmitRequester(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("no session id refuses", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_SESSION_ID", "")
		if _, err := resolveSubmitRequester(); err == nil {
			t.Fatal("expected error with no session id, got nil")
		}
	})

	t.Run("unregistered session refuses", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_SESSION_ID", "sid-unregistered")
		if _, err := resolveSubmitRequester(); err == nil {
			t.Fatal("expected error for unregistered session, got nil")
		}
	})

	t.Run("registered session resolves", func(t *testing.T) {
		const sid = "sid-registered"
		t.Setenv("CLAUDE_CODE_SESSION_ID", sid)
		dir := filepath.Join(home, ".claude", "run", "agent-by-session")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sid), []byte("hermes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := resolveSubmitRequester()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hermes" {
			t.Errorf("requester = %q, want %q", got, "hermes")
		}
	})
}
