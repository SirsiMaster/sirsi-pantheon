package liveness

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain pins the store path for the whole package. Run() now dispatches
// through the router store, so without this a test on a developer host could
// commit a synthetic "broker wedged" alert into the LIVE ~/.sirsi/router.db and
// page a real agent (the "test binaries reaching the user" class, PR #151).
// Individual tests still override HOME; this is the belt to that suspenders.
func TestMain(m *testing.M) {
	os.Setenv("SIRSI_ROUTER_DB", filepath.Join(os.TempDir(), "sirsi-liveness-hermetic-test.db"))
	os.Exit(m.Run())
}
