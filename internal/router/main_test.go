package router

import (
	"os"
	"testing"
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
	os.Exit(m.Run())
}
