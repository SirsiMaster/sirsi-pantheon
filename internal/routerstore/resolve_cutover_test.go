package routerstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A host that has been cut over (router-service.env present) must never fall
// back to the local file when the service env is missing; an explicit
// SIRSI_ROUTER_DB is still honored (tests, sandboxes).
func TestResolveRefusesLocalFileOnCutOverHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_DB", "")
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := Resolve(); err != nil { // not cut over: local file is fine
		t.Fatalf("plain host must resolve locally: %v", err)
	} else {
		_ = s.Close()
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "router-service.env"), []byte("export SIRSI_ROUTER_URL=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve()
	if err == nil || !strings.Contains(err.Error(), "cut over") {
		t.Fatalf("cut-over host without env must refuse the local file, got %v", err)
	}
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
