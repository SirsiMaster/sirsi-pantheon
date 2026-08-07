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
	dbDir, err := os.MkdirTemp("", "sirsi-router-tests-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dbDir)
	os.Setenv("SIRSI_ROUTER_DB", filepath.Join(dbDir, "router.db"))

	// Same class, third instance — the spawn-ceiling ledger (#639 C3). It
	// resolves to ~/.sirsi/dispatch/<agent>.log, so every dispatch test was
	// appending REAL entries under its fake agent ids. Two consequences, both
	// measured 2026-08-07 within an hour of #639 merging:
	//
	//   1. After 12 runs in an hour the tests tripped the very ceiling they
	//      exist to test. TestConsumerExitFreesTheDispatchSlot failed on
	//      pristine main in a clean clone at load 7.62 — the ledger lives in
	//      $HOME, so a fresh checkout does not escape a poisoned counter. That
	//      reddens main for anyone whose change touches internal/router,
	//      because the Ma'at pre-push gate tests changed packages.
	//   2. `ls ~/.sirsi/dispatch/` on this host showed flaky-agent.log,
	//      worker-agent.log, slow-agent.log and restart-agent.log sitting
	//      beside the real codex-home.log, each at exactly 12 entries. A test
	//      that used a REAL agent id would have injected fake dispatches into
	//      the live ceiling and silently rate-limited a production lane.
	//
	// Package-wide rather than per-test on purpose: the invariant is "no test
	// in this package writes production rate-limit state", which a future test
	// must inherit by construction rather than remember to opt into.
	dispatchDirTmp, err := os.MkdirTemp("", "sirsi-router-dispatch-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dispatchDirTmp)
	SetDispatchDir(dispatchDirTmp)

	os.Exit(m.Run())
}
