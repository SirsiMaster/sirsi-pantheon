package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

// Keystone (ADR-069, 2026-09-26): on a cut-over host, SIRSI_ROUTER_DB pointing
// at the CANONICAL local ledger must NOT bypass the cut-over refusal — that is
// the split-brain strand, not a sandbox. A genuinely different db path still
// bypasses. All directions asserted (A35: a guard never shown red is untested).
func TestCutOverMarkerRefusesCanonicalLocalDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_URL", "") // ensure the marker/db path is the one under test
	sirsiDir := filepath.Join(home, ".sirsi")
	if err := os.MkdirAll(sirsiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(sirsiDir, "router-service.env")
	if err := os.WriteFile(marker, []byte(`export SIRSI_ROUTER_URL="spool:///tmp/relay"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	canon := filepath.Join(sirsiDir, "router.db")

	// (a) canonical local db on a cut-over host → NOT bypassed: returns the
	//     marker so Resolve self-heals to the service (the strand is refused).
	t.Setenv("SIRSI_ROUTER_DB", canon)
	if p, err := cutOverMarker(); err != nil || p != marker {
		t.Fatalf("canonical local db on a cut-over host must NOT bypass; got p=%q err=%v (want %q)", p, err, marker)
	}

	// (b) a genuinely different sandbox db path → still bypasses (deliberate).
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, "sandbox-test.db"))
	if p, err := cutOverMarker(); err != nil || p != "" {
		t.Fatalf("a sandbox db path must still bypass the marker; got p=%q err=%v (want \"\")", p, err)
	}

	// (c) no SIRSI_ROUTER_DB, marker present → marker returned (unchanged).
	t.Setenv("SIRSI_ROUTER_DB", "")
	if p, err := cutOverMarker(); err != nil || p != marker {
		t.Fatalf("cut-over host with no db must return the marker; got p=%q err=%v (want %q)", p, err, marker)
	}

	// (d) canonical local db but NO marker (host not cut over) → "" (local is
	//     legitimate here); negative control that we don't over-refuse.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_DB", canon)
	if p, err := cutOverMarker(); err != nil || p != "" {
		t.Fatalf("canonical db with no cut-over marker must NOT refuse; got p=%q err=%v (want \"\")", p, err)
	}
}
