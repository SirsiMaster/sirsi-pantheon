package router

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// TestMain makes the session-reaper duty inert for EVERY test in this package
// by default. The duty's real body shells `ps` and SIGTERMs processes it
// judges superseded — any test that executes the duty table (SuperviseOnce,
// runSupervisorDuties) without a stub would fire that on the developer's
// machine and could reap a REAL session, silently, while the test still
// passes (the #151 test-side-effects class; the first run of the reaper's
// own suite did exactly this before the stub existed). A test that needs the
// real body must opt in explicitly via setSessionReaperImpl.
func TestMain(m *testing.M) {
	setSessionReaperImpl(func(routerRoot, repoRoot string) error { return nil })

	// Hermetic-store law (the #99/#151 host-state class, store-cutover edition):
	// the host cutover marker ~/.sirsi/store-wake makes StoreWake() true for
	// EVERY process on a cut-over machine — including this test binary — so
	// fixture-rooted tests silently read the LIVE ~/.sirsi/router.db (observed:
	// TotalPending tracking the machine's real queue, WakePass seeing the
	// host's 25+ armed agents). Pin both knobs so local == CI regardless of
	// host state: store-wake off (CI's marker-less default), and the db seam
	// pointed at a scratch path in case any test flips wake on without setting
	// its own db. Tests that exercise the store path opt in via t.Setenv.
	os.Setenv(routercfg.StoreWakeEnv, "0")
	os.Setenv("SIRSI_ROUTER_DB", filepath.Join(os.TempDir(), "sirsi-router-hermetic-test.db"))

	os.Exit(m.Run())
}
