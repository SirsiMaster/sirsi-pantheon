package main_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyntheticRegistrationEnvDropsEveryProviderAlias(t *testing.T) {
	t.Parallel()
	base := []string{"PATH=/fixture/bin", "GIT_DIR=/foreign/repo", "PWD=/foreign/cwd",
		"SIRSI_AGENT_ID=foreign-agent", "SIRSI_THREAD_ID=foreign-thread",
		"CLAUDE_CODE_SESSION_ID=foreign-claude", "CODEX_SESSION_ID=foreign-codex", "CODEX_THREAD_ID=foreign-codex"}
	got := sirsiTestEnvFrom(base, "/fixture/repo", "/fixture/store")
	for _, kv := range got {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "SIRSI_AGENT_ID", "SIRSI_THREAD_ID", "CLAUDE_CODE_SESSION_ID", "CODEX_SESSION_ID", "CODEX_THREAD_ID", "GIT_DIR":
			t.Errorf("ambient identity/routing escaped: %s", key)
		}
	}
	// Deliberately supplied fixture identities still win; this seam strips
	// inheritance, not an explicit test input or production identity policy.
	got = sirsiTestEnvFrom(base, "/fixture/repo", "/fixture/store", "CODEX_THREAD_ID=explicit-fixture")
	if got[len(got)-1] != "CODEX_THREAD_ID=explicit-fixture" {
		t.Fatal("explicit fixture input was removed")
	}
	if base[2] != "PWD=/foreign/cwd" {
		t.Fatal("environment constructor mutated caller input")
	}
}

func TestSyntheticRegistrationPreservesPrivateProviderCanary(t *testing.T) {
	// Real CLI registration, but only private inert HOME, store and canaries.
	// Never inspect or snapshot the real operator's marker files or identity.
	for _, alias := range []string{"CLAUDE_CODE_SESSION_ID", "CODEX_SESSION_ID", "CODEX_THREAD_ID"} {
		t.Run(alias, func(t *testing.T) {
			home, repo := t.TempDir(), setupTempRouter(t)
			var paths []string
			for _, dir := range []string{"agent-by-session", "thread-by-session"} {
				path := filepath.Join(home, ".claude", "run", dir, "private-provider-canary")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("preserved-fixture\n"), 0600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, testBinary, "thread", "register", "--agent", "test-provider-isolation", "--surface", "codex", "--repo", repo, "--anchor-pid", "1", "--json")
			cmd.Dir = repo
			base := append(os.Environ(), alias+"=private-provider-canary")
			cmd.Env = sirsiTestEnvFrom(base, repo, filepath.Join(repo, "owned.db"), "HOME="+home, "SIRSI_ROUTER_STORE_WAKE=0")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("private registration: %v\n%s", err, out)
			}
			for _, path := range paths {
				body, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(body, []byte("preserved-fixture\n")) {
					t.Fatalf("synthetic registration changed private canary: %s: %v", filepath.Base(filepath.Dir(path)), err)
				}
			}
		})
	}
}
