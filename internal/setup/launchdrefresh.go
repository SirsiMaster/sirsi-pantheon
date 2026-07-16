package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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
		// Bootout is best-effort: "not loaded" is fine — the point is that no
		// stale LWCR survives into the bootstrap below.
		_ = runLaunchctl("bootout", fmt.Sprintf("gui/%d/%s", uid, label))
		if err := runLaunchctl("bootstrap", fmt.Sprintf("gui/%d", uid), plist); err != nil {
			problems = append(problems, fmt.Sprintf("%s: bootstrap failed: %v", label, err))
			continue
		}
		refreshed = append(refreshed, label)
	}
	return refreshed, problems
}
