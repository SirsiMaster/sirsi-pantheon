package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The 2026-09-15 incident: SIRSI_ROUTER_DB left set in an interactive shell on
// a host already cut over to the router service silently redirects every
// router call to a private local file instead of the shared service — no
// error, no warning, the item just never reaches anyone else. This is the
// read-only doctor check that would have caught it immediately.
func TestRouterDBOnCutoverHostWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("not cut over: SIRSI_ROUTER_DB is the normal path, no warning", func(t *testing.T) {
		t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, "test.db"))
		if got := routerDBOnCutoverHostWarning(); got != "" {
			t.Fatalf("warned on a non-cut-over host: %q", got)
		}
	})

	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, ".sirsi", "router-service.env")
	if err := os.WriteFile(marker, []byte("export SIRSI_ROUTER_URL=https://example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("cut over, SIRSI_ROUTER_DB unset: no warning (negative control)", func(t *testing.T) {
		t.Setenv("SIRSI_ROUTER_DB", "")
		if got := routerDBOnCutoverHostWarning(); got != "" {
			t.Fatalf("warned with SIRSI_ROUTER_DB unset: %q", got)
		}
	})

	t.Run("cut over, SIRSI_ROUTER_DB set: warns and names both paths", func(t *testing.T) {
		dbPath := filepath.Join(home, "stray.db")
		t.Setenv("SIRSI_ROUTER_DB", dbPath)
		got := routerDBOnCutoverHostWarning()
		if got == "" {
			t.Fatal("did not warn: a cut-over host with SIRSI_ROUTER_DB set writes a private, invisible local ledger — this is exactly the 2026-09-15 split-brain")
		}
		for _, want := range []string{dbPath, marker} {
			if !contains(got, want) {
				t.Errorf("warning missing %q: %s", want, got)
			}
		}
	})
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
