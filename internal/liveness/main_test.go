package liveness

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain pins the store path for the whole package. Run() dispatches through
// the router store, so without this a test on a developer host could commit a
// synthetic "broker wedged" alert into the LIVE ~/.sirsi/router.db and page a
// real agent (the "test binaries reaching the user" class, PR #151).
//
// The path must be per-RUN, not a fixed /tmp name: a database that survives
// process exit is shared across repeated and concurrent invocations, so an
// earlier run's synthetic alert silently changes a later run's dedupe outcome.
// That is a stateful suite masquerading as a hermetic one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sirsi-liveness-test")
	if err != nil {
		panic("liveness TestMain: create temp store dir: " + err.Error())
	}
	os.Setenv("SIRSI_ROUTER_DB", filepath.Join(dir, "router.db"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
