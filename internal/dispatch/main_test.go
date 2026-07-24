package dispatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// TestMain pins the store-cutover knobs so this package's tests behave the
// same on a cut-over host as on marker-less CI (the #99/#151 host-state
// class, store-cutover edition). Without this, the host marker
// ~/.sirsi/store-wake makes routercfg.StoreWake() true inside the test
// binary, flipping facade behavior (e.g. Send stops writing the items/<id>.md
// audit view) and failing fixture tests that are green on CI. Tests that
// exercise cutover-on behavior opt in via t.Setenv(routercfg.StoreWakeEnv, "1").
func TestMain(m *testing.M) {
	os.Setenv(routercfg.StoreWakeEnv, "0")
	os.Setenv("SIRSI_ROUTER_DB", filepath.Join(os.TempDir(), "sirsi-dispatch-hermetic-test.db"))
	os.Exit(m.Run())
}
