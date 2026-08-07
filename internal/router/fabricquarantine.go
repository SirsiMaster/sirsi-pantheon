// fabricquarantine.go — the fabric-wide operator off-switch (R7/G6).
//
// Incident, 2026-08-06: turning on 21 lanes drove load average to 50.4 and
// locked the owner's machine. Three teardown attempts were reverted by the
// fabric fighting back: `launchctl bootout` re-bootstrapped within 40s
// (liveness-watch/fabric-watchdog), `launchctl disable` was silently cleared
// (`print-disabled` showed nothing after), and `horus.agent-router` restarted
// via KeepAlive and reinstalled all 24 lanes. The only thing that held was a
// marker file plus `wake.mechanism=none` in the registry.
//
// QuarantineMarkerPath (gemmaliveness.go) already solved exactly this problem
// for one broker: a marker every restorer checks BEFORE reviving/dispatching,
// generalized here so ANY dispatcher — the per-agent wake-loop's consumer
// dispatch (wake.go) and the dead-label kickstart duty (launchdkickstart.go)
// — refuses to bring up new work fabric-wide, not just gemma's lane.
//
// `sirsi router quarantine` / `sirsi router unquarantine` are the sanctioned
// way to set or clear it — never hand-edit the file.
package router

import (
	"os"
	"path/filepath"
	"sync"
)

// FabricQuarantineMarkerPath is the file whose presence tells every dispatcher
// in the fabric to STAND DOWN: no new lane, no new consumer, no revived label.
func FabricQuarantineMarkerPath(home string) string {
	return filepath.Join(home, ".sirsi", "fabric-quarantine")
}

// fabricQuarantineMu guards the injected seam (Rule A16/A21) so tests can
// swap it without touching the real filesystem or racing other duties.
var (
	fabricQuarantineMu    sync.RWMutex
	isFabricQuarantinedFn = func(home string) bool {
		_, err := os.Stat(FabricQuarantineMarkerPath(home))
		return err == nil
	}
)

// SetFabricQuarantinedFn installs a test double for the fabric quarantine
// check. Passing nil restores the real os.Stat-backed default.
func SetFabricQuarantinedFn(fn func(home string) bool) {
	fabricQuarantineMu.Lock()
	defer fabricQuarantineMu.Unlock()
	if fn != nil {
		isFabricQuarantinedFn = fn
		return
	}
	isFabricQuarantinedFn = func(home string) bool {
		_, err := os.Stat(FabricQuarantineMarkerPath(home))
		return err == nil
	}
}

func getFabricQuarantinedFn() func(home string) bool {
	fabricQuarantineMu.RLock()
	defer fabricQuarantineMu.RUnlock()
	return isFabricQuarantinedFn
}

// IsFabricQuarantined reports whether the operator has stood the whole fabric
// down. home is normally os.UserHomeDir(); callers that already resolved it
// pass it through rather than re-resolving.
func IsFabricQuarantined(home string) bool {
	return getFabricQuarantinedFn()(home)
}
