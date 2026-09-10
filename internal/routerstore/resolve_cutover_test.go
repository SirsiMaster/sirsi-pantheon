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
