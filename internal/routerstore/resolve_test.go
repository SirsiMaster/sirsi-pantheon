package routerstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-062 §1: a node pointed at the service has no local ledger of record.
// Both the write constructor and the read-only path resolver must refuse,
// never fall back to a local file behind the service's back.
func TestResolveAndLocalPathRefuseWhenServiceURLSet(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "https://router.example.test")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	if _, err := LocalPath(); err == nil || !strings.Contains(err.Error(), "SIRSI_ROUTER_URL") {
		t.Fatalf("LocalPath with SIRSI_ROUTER_URL set: want refusal naming the variable, got err=%v", err)
	}
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	if s, err := Resolve(); err == nil || s != nil {
		t.Fatalf("Resolve with URL but no token: want (nil, err), got (%v, %v)", s, err)
	}
	t.Setenv("SIRSI_ROUTER_TOKEN", "tok")
	s, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve with URL+token: %v", err)
	}
	if _, ok := s.(*RemoteStore); !ok {
		t.Fatalf("Resolve with URL+token: want *RemoteStore, got %T", s)
	}
}

// Resolve honors SIRSI_ROUTER_DB and creates the parent directory, so a fresh
// HOME (or a fresh temp dir) is not an error.
func TestResolveOpensSIRSIRouterDBAndCreatesParent(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "")
	path := filepath.Join(t.TempDir(), "nested", "dir", "router.db")
	t.Setenv("SIRSI_ROUTER_DB", path)

	s, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer func() { _ = s.Close() }()
	got, err := LocalPath()
	if err != nil || got != path {
		t.Fatalf("LocalPath = %q, %v; want %q", got, err, path)
	}
}

// setHomeWithMarker points HOME at a fresh temp dir containing
// ~/.sirsi/router-service.env with the given content, returning that file's
// path. Also pre-registers SIRSI_ROUTER_URL/SIRSI_RELAY_TRUST_GROUP with
// t.Setenv so testing.T restores them after the test even though
// loadCutOverEnv mutates them via a raw os.Setenv (which t alone would not
// know to undo).
func setHomeWithMarker(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_RELAY_TRUST_GROUP", "")
	t.Setenv("SIRSI_ROUTER_DB", "")
	dir := filepath.Join(home, ".sirsi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	p := filepath.Join(dir, "router-service.env")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	// t.Setenv above ran before the value existed; re-clear now the file is in
	// place so the very next os.Getenv reads see "" as the test body expects.
	os.Unsetenv("SIRSI_ROUTER_URL")
	os.Unsetenv("SIRSI_RELAY_TRUST_GROUP")
	return p
}

// 2026-09-23: a live Codex session on the M5, spawned outside any shell that
// sources ~/.zshenv, hit "SIRSI_ROUTER_URL is unset" even though the cut-over
// marker this host wrote names the real service — the owner's "the router
// relay failed" report. Resolve must recover from the marker itself rather
// than requiring a shell to have sourced it, using the exact file content a
// real cut-over M5 host has (comment included) as the fixture.
func TestResolveSelfHealsFromCutOverMarkerWhenEnvUnset(t *testing.T) {
	setHomeWithMarker(t, "export SIRSI_ROUTER_URL='spool:///var/sirsipantheon/relay'\n"+
		"export SIRSI_RELAY_TRUST_GROUP=\"_sirsipantheon\"  # added by claude-io 2026-09-15: without this...\n")

	s, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: want self-heal from marker, got err=%v", err)
	}
	defer func() { _ = s.Close() }()
	if got := os.Getenv("SIRSI_ROUTER_URL"); got != "spool:///var/sirsipantheon/relay" {
		t.Fatalf("SIRSI_ROUTER_URL after Resolve = %q, want the marker's URL", got)
	}
	if got := os.Getenv("SIRSI_RELAY_TRUST_GROUP"); got != "_sirsipantheon" {
		t.Fatalf("SIRSI_RELAY_TRUST_GROUP after Resolve = %q, want _sirsipantheon (comment stripped)", got)
	}
}

// An operator's explicit env always wins over the marker file — the marker is
// a recovery source, never an override.
func TestResolveMarkerNeverOverridesExplicitEnv(t *testing.T) {
	setHomeWithMarker(t, "export SIRSI_ROUTER_URL='spool:///wrong/path'\n")
	t.Setenv("SIRSI_ROUTER_URL", "spool:///var/sirsipantheon/relay")

	s, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer func() { _ = s.Close() }()
	if got := os.Getenv("SIRSI_ROUTER_URL"); got != "spool:///var/sirsipantheon/relay" {
		t.Fatalf("SIRSI_ROUTER_URL = %q, want the explicit value unchanged", got)
	}
}

// A marker that exists but carries no recognizable SIRSI_ROUTER_URL line
// still fails closed — self-heal only recovers a real pointer, it never
// invents one or falls through to the local file.
func TestResolveFailsClosedWhenMarkerHasNoURL(t *testing.T) {
	setHomeWithMarker(t, "# nothing useful here\nexport SOMETHING_ELSE=1\n")

	s, err := Resolve()
	if err == nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal("Resolve: want refusal when the marker has no SIRSI_ROUTER_URL, got success")
	}
	if !strings.Contains(err.Error(), "SIRSI_ROUTER_URL") {
		t.Fatalf("Resolve err = %v, want it to name SIRSI_ROUTER_URL", err)
	}
}
