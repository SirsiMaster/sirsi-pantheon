package routerstore

import (
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
