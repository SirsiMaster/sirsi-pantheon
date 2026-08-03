package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// ── LWCR refresh: re-derive launch constraints after a binary re-sign ─────────
//
// macOS launchd pins a Lightweight Code Requirement (LWCR) to each job at
// bootstrap time. Re-signing a binary a job runs (every `make install`, every
// ad-hoc re-sign) changes its cdhash, and launchd then SIGKILLs the job with
// "Launch Constraint Violation" on every start — a crash-loop `kickstart -k`
// cannot clear, because the stale constraint survives kickstart. The only
// clean reset is bootout + bootstrap per job, which re-derives the constraint
// from the binary as currently signed. (Router item 20260716-150734: 531
// crash-loops on ai.sirsi.conduit.tick after a re-sign.)

const (
	bootstrapAttempts   = 10
	bootstrapRetryDelay = 300 * time.Millisecond
)

// sleepFn indirects the retry delay so tests run instantly (Rule A16).
var sleepFn = time.Sleep

// RefreshSirsiLaunchAgents boots out and re-bootstraps every ai.sirsi.* job in
// ~/Library/LaunchAgents. Run after any sirsi binary re-sign/deploy. macOS only.
func RefreshSirsiLaunchAgents() (refreshed, problems []string) {
	if runtime.GOOS != "darwin" {
		return nil, []string{"launchd refresh is macOS only"}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, []string{"cannot resolve home: " + err.Error()}
	}
	return refreshSirsiLaunchAgentsIn(filepath.Join(home, "Library", "LaunchAgents"), os.Getuid())
}

// refreshSirsiLaunchAgentsIn is the injectable-path variant (Rule A16): the
// tests point it at a temp dir and stub launchctlExecFn.
func refreshSirsiLaunchAgentsIn(dir string, uid int) (refreshed, problems []string) {
	plists, _ := filepath.Glob(filepath.Join(dir, "ai.sirsi.*.plist"))
	sort.Strings(plists) // deterministic report order
	for _, plist := range plists {
		// Glob patterns end at ".plist" so .bak/.quarantined copies never match,
		// but guard anyway: bootstrapping a stale backup would resurrect it.
		if !strings.HasSuffix(plist, ".plist") {
			continue
		}
		label := strings.TrimSuffix(filepath.Base(plist), ".plist")
		// ENABLE FIRST. `disabled` lives in launchd's PERSISTENT override
		// database, not in the plist and not in the loaded-job set — so bootout
		// and bootstrap cannot clear it, and a disabled label fails bootstrap
		// with a bare "5: Input/output error" forever. Observed live 2026-08-03:
		// 23 of 28 ai.sirsi labels were disabled, refresh reported 22 failures
		// with that opaque errno on every run, and the entire agent fabric sat
		// down for ~15 hours with no consumer.
		//
		// `enable` is idempotent and harmless on an already-enabled label, so it
		// costs nothing on the healthy path and is the only thing that clears the
		// unhealthy one. Best-effort: a failure here still lets bootstrap try and
		// report its own error rather than masking it.
		_ = runLaunchctl("enable", fmt.Sprintf("gui/%d/%s", uid, label))
		// Bootout is best-effort: "not loaded" is fine — the point is that no
		// stale LWCR survives into the bootstrap below.
		_ = runLaunchctl("bootout", fmt.Sprintf("gui/%d/%s", uid, label))
		// Bootout of a RUNNING job tears down asynchronously; bootstrapping
		// before the old instance exits fails with EIO (verified live: only the
		// loaded KeepAlive wake loops failed, every unloaded job succeeded).
		// Retry across the teardown window instead of reporting a false failure.
		var err error
		for attempt := 0; attempt < bootstrapAttempts; attempt++ {
			if err = runLaunchctl("bootstrap", fmt.Sprintf("gui/%d", uid), plist); err == nil {
				break
			}
			sleepFn(bootstrapRetryDelay)
		}
		if err != nil {
			// Name the most likely cause instead of surfacing a bare errno. EIO
			// from bootstrap is almost always either a still-tearing-down job
			// (which the retry loop above already absorbed) or a persistent
			// disable we just tried to clear — and "Input/output error" alone
			// sent this diagnosis down a memory-pressure dead end for an hour.
			problems = append(problems, fmt.Sprintf(
				"%s: bootstrap failed: %v (if this is 'Input/output error', check `launchctl print-disabled gui/$UID` — a persistently disabled label cannot be bootstrapped)",
				label, err))
			continue
		}
		refreshed = append(refreshed, label)
	}
	return refreshed, problems
}
