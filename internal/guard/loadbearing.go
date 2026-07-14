package guard

// loadbearing.go — recognition of LOAD-BEARING Pantheon infrastructure that must
// NOT be killed or starved while the system is working (PANTHEON_RULES A32,
// ADR-040). The canonical case is the local-model broker (`sirsi gemma serve`):
// it is the Tier-0 substrate the router, the reconcile, and gemma-the-builder all
// depend on. Killing it to "reclaim RAM" breaks Pantheon — the correct response
// to an oversized broker is to RIGHT-SIZE the Tier-0 model (swap to a smaller
// one), never to SIGKILL a serving process mid-work.
//
// This inverts the old posture where the broker registered itself as *governed*
// (consent-to-be-killed under pressure): the routine memory-reclaim paths
// (FindRunaway) now recognize the broker by its pidfile and skip it. A true
// last-resort emergency is a separate, explicit decision — never routine, and
// never something an agent does while working.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// loadBearingPidFiles are the pidfiles of infrastructure that must not be killed
// by routine reclaim. Relative to ~/.sirsi. The gemma broker + its worker are the
// Tier-0 substrate; add future load-bearing services here.
var loadBearingPidFiles = []string{
	"gemma-server.pid", // the warm local-model broker (`sirsi gemma serve`)
	"gemma-worker.pid", // the router-facing gemma worker
}

// LoadBearingPIDs returns the set of live PIDs Pantheon must not kill/starve
// while working. Dead PIDs are excluded (a stale pidfile never protects a reused
// PID — the PID-alive lesson).
func LoadBearingPIDs() map[int]bool {
	out := map[int]bool{}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	for _, f := range loadBearingPidFiles {
		b, err := os.ReadFile(filepath.Join(home, ".sirsi", f))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 0 {
			continue
		}
		if pidAlive(pid) {
			out[pid] = true
		}
	}
	return out
}

// IsLoadBearing reports whether a PID is protected load-bearing infrastructure.
// Every kill/suspend path MUST consult this before acting on a PID.
func IsLoadBearing(pid int) bool { return LoadBearingPIDs()[pid] }

// pidAlive reports whether pid exists (signal 0 probes without delivering).
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
