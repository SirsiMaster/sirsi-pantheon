// launchdkickstart.go — the "dead label" duty (local sovereignty P1, owner
// directive 2026-07-23): every ai.sirsi.* LaunchAgent plist on disk must be
// LOADED in launchd. The morning's reboot proved the failure mode — a label
// that exits at boot (or was never bootstrapped after an edit) stays dead
// until a human or a CLOUD agent notices. This duty is deterministic shell-out
// supervision: no LLM, no network, runs on the resident supervisor.
//
// Scope deliberately narrow (A32 do-no-harm): it BOOTSTRAPS plists that are
// on disk but absent from launchd. It never kills, never kickstarts a loaded
// label (a loaded label's own RunAtLoad/StartInterval/KeepAlive policy owns
// its process lifecycle), never touches labels outside ai.sirsi.* /
// actions.runner.*, and skips .bak/.quarantined/edited-suffix files.
package router

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// launchdDeps are the OS seams (Rule A16) — tests stub these.
type launchdDeps struct {
	listLabels     func() (map[string]bool, error) // labels currently known to launchd
	bootstrapPlist func(plistPath string) error
	uid            func() int
}

var launchdOS = launchdDeps{
	listLabels: func() (map[string]bool, error) {
		out, err := exec.Command("launchctl", "list").Output()
		if err != nil {
			return nil, err
		}
		labels := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 3 {
				labels[fields[2]] = true
			}
		}
		return labels, nil
	},
	bootstrapPlist: func(plistPath string) error {
		uid := os.Getuid()
		out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), plistPath).CombinedOutput()
		if err != nil {
			return fmt.Errorf("bootstrap %s: %v (%s)", filepath.Base(plistPath), err, strings.TrimSpace(string(out)))
		}
		return nil
	},
	uid: os.Getuid,
}

// managedPlist reports whether a LaunchAgents entry is one of ours and a real
// plist (not a backup/quarantine artifact).
func managedPlist(name string) bool {
	if !strings.HasSuffix(name, ".plist") {
		return false // .bak-*, .quarantined, editor droppings
	}
	return strings.HasPrefix(name, "ai.sirsi.") || strings.HasPrefix(name, "actions.runner.")
}

// labelForPlist derives the launchd label from the plist filename. Sirsi and
// actions-runner plists are named exactly <label>.plist by convention.
func labelForPlist(name string) string {
	return strings.TrimSuffix(name, ".plist")
}

// KickstartDeadLabels bootstraps every managed plist on disk whose label is
// missing from launchd. Returns the labels it revived (for the run report).
func KickstartDeadLabels(agentsDir string, deps launchdDeps) ([]string, error) {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	loaded, err := deps.listLabels()
	if err != nil {
		return nil, fmt.Errorf("launchctl list: %w", err)
	}
	var revived []string
	var firstErr error
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !managedPlist(name) {
			continue
		}
		label := labelForPlist(name)
		if loaded[label] {
			continue
		}
		if err := deps.bootstrapPlist(filepath.Join(agentsDir, name)); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		revived = append(revived, label)
	}
	return revived, firstErr
}

// Kickstart wiring follows the gemma-liveness seam pattern (Rule A16/A21):
// INERT by default so library consumers and tests never shell real launchctl;
// cmd/sirsi wires RunLaunchdKickstartDuty at init.
var (
	kickstartMu sync.RWMutex
	kickstartFn = func(routerRoot, repoRoot string) error { return nil }
)

// SetLaunchdKickstartFn installs the real kickstart pass.
func SetLaunchdKickstartFn(fn func(routerRoot, repoRoot string) error) {
	kickstartMu.Lock()
	defer kickstartMu.Unlock()
	if fn != nil {
		kickstartFn = fn
	}
}

func getLaunchdKickstartFn() func(string, string) error {
	kickstartMu.RLock()
	defer kickstartMu.RUnlock()
	return kickstartFn
}

// RunLaunchdKickstartDuty is the real duty pass: revive dead labels and
// record each revival as an owner-visible heal.
func RunLaunchdKickstartDuty(routerRoot, repoRoot string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	revived, err := KickstartDeadLabels(filepath.Join(home, "Library", "LaunchAgents"), launchdOS)
	for _, label := range revived {
		RecordHeal(fmt.Sprintf("background service %s was not running — reloaded", label))
	}
	return err
}
