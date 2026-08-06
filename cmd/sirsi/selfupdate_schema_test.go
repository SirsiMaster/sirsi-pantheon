package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// candidate returns a probe payload for a binary declaring the given ceiling.
func candidate(ceiling int) modversion.Info {
	return modversion.Info{RouterSchemaMax: ceiling}
}

// liveStoreAt creates a real router store and returns its path and live schema.
func liveStoreAt(t *testing.T) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "router.db")
	s, err := routerstore.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if closeErr := s.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	live, err := routerstore.ReadSchemaVersion(path)
	if err != nil {
		t.Fatalf("ReadSchemaVersion: %v", err)
	}
	return path, live
}

// TestSchemaGateRejectsCandidateBelowLive is the fleet-outage case: on
// 2026-08-06 a binary built from a branch whose migration ceiling was below the
// live store replaced the installed one and fail-closed every agent at once.
func TestSchemaGateRejectsCandidateBelowLive(t *testing.T) {
	path, live := liveStoreAt(t)
	t.Setenv("SIRSI_ROUTER_DB", path)

	err := schemaCompatibilityGate(candidate(live - 1))
	if !errors.Is(err, selfupdate.ErrSchemaIncompatible) {
		t.Fatalf("candidate below live must be rejected, got %v", err)
	}
}

// TestSchemaGateAllowsEqualAndNewerCandidate proves the gate is not simply
// refusing everything — equal and newer candidates reach the confirmation
// boundary. A gate that never passes is indistinguishable from a broken one.
func TestSchemaGateAllowsEqualAndNewerCandidate(t *testing.T) {
	path, live := liveStoreAt(t)
	t.Setenv("SIRSI_ROUTER_DB", path)

	for _, ceiling := range []int{live, live + 1} {
		if err := schemaCompatibilityGate(candidate(ceiling)); err != nil {
			t.Fatalf("candidate ceiling %d vs live %d must pass, got %v", ceiling, live, err)
		}
	}
}

// TestSchemaGateTreatsMissingStoreAsZero covers the regression codex-home
// blocked #550 on: a host that has never initialized Pantheon routing has no
// live schema to protect, and must still be able to heal CLI drift. Reading a
// nonexistent path as an error would lock fresh installs out of self-update
// permanently.
func TestSchemaGateTreatsMissingStoreAsZero(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nonexistent", "router.db")
	t.Setenv("SIRSI_ROUTER_DB", missing)

	live, err := resolveLiveSchema(missing)
	if err != nil {
		t.Fatalf("missing store must resolve cleanly, got %v", err)
	}
	if live != 0 {
		t.Fatalf("missing store must read as live schema 0, got %d", live)
	}
	if err := schemaCompatibilityGate(candidate(1)); err != nil {
		t.Fatalf("any positive ceiling must pass against an absent store, got %v", err)
	}
}

// TestSchemaGateFailsClosedOnUnreadableStore is the other half of the pair: a
// store that exists but cannot be read is NOT the same fact as one that does
// not exist. There is something to protect and the gate is blind to it, so it
// must refuse rather than fall back to zero.
func TestSchemaGateFailsClosedOnUnreadableStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_DB", path)

	if _, err := resolveLiveSchema(path); err == nil {
		t.Fatal("a corrupt store must not resolve to a usable live schema")
	}
	if err := schemaCompatibilityGate(candidate(9999)); err == nil {
		t.Fatal("gate must fail closed on a corrupt store, even for a very new candidate")
	}
}

// TestSchemaGateRejectsCandidateWithNoDeclaredCeiling keeps pre-contract
// binaries out: a candidate that cannot state a ceiling cannot prove it is safe.
func TestSchemaGateRejectsCandidateWithNoDeclaredCeiling(t *testing.T) {
	path, _ := liveStoreAt(t)
	t.Setenv("SIRSI_ROUTER_DB", path)

	if err := schemaCompatibilityGate(candidate(0)); !errors.Is(err, selfupdate.ErrSchemaIncompatible) {
		t.Fatalf("zero ceiling must fail closed, got %v", err)
	}
}

// TestResolveRouterDBPathPrefersEnvOverride pins the override contract the gate
// and the conduit heal share.
func TestResolveRouterDBPathPrefersEnvOverride(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_DB", "/custom/router.db")
	got, err := resolveRouterDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/custom/router.db" {
		t.Fatalf("env override ignored, got %s", got)
	}

	t.Setenv("SIRSI_ROUTER_DB", "")
	userHomeDirFn = func() (string, error) { return "/home/test", nil }
	t.Cleanup(func() { userHomeDirFn = os.UserHomeDir })

	got, err = resolveRouterDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/home/test", ".sirsi", "router.db"); got != want {
		t.Fatalf("default path = %s, want %s", got, want)
	}
}
