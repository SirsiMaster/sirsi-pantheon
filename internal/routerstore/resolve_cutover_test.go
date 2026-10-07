package routerstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A host must never fall back to the local file when the service env is
// missing; an explicit SIRSI_ROUTER_DB is still honored for tests/sandboxes.
func TestResolveRefusesLocalFileOnCutOverHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_DB", "")
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := Resolve(); err == nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal("plain host without explicit DB must refuse instead of creating a local file")
	}
	dbPath := filepath.Join(home, ".sirsi", "router.db")
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Fatalf("plain-host refusal must not create router.db: %v", statErr)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "router-service.env"), []byte("export SIRSI_ROUTER_URL=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Self-heal (2026-09-23) means this marker's bare "x" IS now recovered as
	// SIRSI_ROUTER_URL, so Resolve refuses for a different, legitimate reason
	// (x is not a spool:// URL and carries no token) rather than the original
	// "unset" message — a stronger proof of the actual invariant this test
	// protects, since it now exercises the RemoteStore/token path entirely and
	// never goes anywhere near LocalPath(). Assert the invariant directly: the
	// no local router.db is created or touched.
	if _, err := Resolve(); err == nil {
		t.Fatal("cut-over host without a usable env must refuse, got success")
	}
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Fatalf("cut-over refusal must leave router.db absent: %v", statErr)
	}
	t.Setenv("SIRSI_ROUTER_URL", "") // undo this test's own self-heal before the explicit-DB case below
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, "explicit.db"))
	if s, err := Resolve(); err != nil {
		t.Fatalf("explicit SIRSI_ROUTER_DB must still open: %v", err)
	} else {
		_ = s.Close()
	}
}

// Regression coverage the reviewer added independently (SSA 2026-09-10): a
// refusal creates no router.db; URL without token is still refused; URL+token
// selects RemoteStore without touching the network; an unreadable marker is a
// diagnostic, never a silent fallback.
func TestResolveCutOverEdges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_DB", "")
	dir := filepath.Join(home, ".sirsi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "router-service.env"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(); err == nil {
		t.Fatal("must refuse")
	}
	if _, err := os.Stat(filepath.Join(dir, "router.db")); !os.IsNotExist(err) {
		t.Fatal("refusal must not create router.db")
	}
	t.Setenv("SIRSI_ROUTER_URL", "https://router.invalid")
	if _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "SIRSI_ROUTER_TOKEN is empty") {
		t.Fatalf("URL without token must be refused: %v", err)
	}
	t.Setenv("SIRSI_ROUTER_TOKEN", "tok")
	s, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*RemoteStore); !ok {
		t.Fatalf("URL+token must select RemoteStore, got %T", s)
	}
	// Unreadable marker: a directory where the file should be is a stat success
	// on macOS, so simulate the error path with an unreadable parent.
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	if os.Geteuid() != 0 {
		if err := os.Chmod(dir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "cannot read cut-over marker") {
			t.Fatalf("unreadable marker must be a diagnostic, got %v", err)
		}
	}
}
