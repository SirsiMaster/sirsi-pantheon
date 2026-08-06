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
	disabledLabels func() (map[string]bool, error) // labels disabled in override DB
	enableLabel    func(domain, label string) error
	bootstrapPlist func(plistPath string) error
	uid            func() int
	isQuarantined  func() bool // true if the operator deliberately stopped the gemma broker
}

// quarantinedLabels are launchd labels this duty must never revive while the
// broker is quarantined. A deliberate operator stop (`sirsi gemma quarantine`)
// only guards RunGemmaLivenessDuty (PR #611) — it does not remove these plists
// from ~/Library/LaunchAgents, so without this check the very next kickstart
// tick silently undoes the stop, exactly the failure class PR #611 exists to
// close (codex-inference finding, 2026-08-06).
var quarantinedLabels = map[string]bool{
	"ai.sirsi.gemma-broker": true,
	"ai.sirsi.gemma-worker": true,
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
	disabledLabels: func() (map[string]bool, error) {
		uid := os.Getuid()
		out, err := exec.Command("launchctl", "print-disabled", fmt.Sprintf("gui/%d", uid)).Output()
		if err != nil {
			// print-disabled may be unavailable in some contexts — fail-open so the
			// rest of the duty (bootstrap missing labels) still runs.
			return map[string]bool{}, nil
		}
		disabled := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasSuffix(line, "=> disabled") {
				continue
			}
			start := strings.Index(line, `"`)
			end := strings.LastIndex(line, `"`)
			if start < 0 || end <= start {
				continue
			}
			disabled[line[start+1:end]] = true
		}
		return disabled, nil
	},
	enableLabel: func(domain, label string) error {
		out, err := exec.Command("launchctl", "enable", domain+"/"+label).CombinedOutput()
		if err != nil {
			return fmt.Errorf("enable %s: %v (%s)", label, err, strings.TrimSpace(string(out)))
		}
		return nil
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
	isQuarantined: func() bool {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(QuarantineMarkerPath(home))
		return err == nil
	},
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
// missing from launchd. If a label is also disabled in the override DB, it
// calls `launchctl enable` first — kickstart cannot recover a disabled+unloaded
// label (2026-07-31 fabric-loss post-mortem). Returns revived labels.
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
	// Read the override DB once; disabled labels require enable before bootstrap.
	// Fail-open: nil fn (e.g. tests that don't stub it) or exec error both yield
	// an empty map — bootstrap still runs and will fail on a truly disabled label,
	// surfacing the error rather than silently skipping the label.
	var disabled map[string]bool
	if deps.disabledLabels != nil {
		disabled, _ = deps.disabledLabels()
	}

	uid := deps.uid()
	domain := fmt.Sprintf("gui/%d", uid)

	quarantined := deps.isQuarantined != nil && deps.isQuarantined()

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
		if quarantined && quarantinedLabels[label] {
			continue // deliberate operator stop — never silently revive
		}
		// Enable before bootstrap when the override DB has disabled this label.
		// Without this step, bootstrap silently fails (Operation not permitted)
		// and the duty appears to succeed — the exact failure mode from 2026-07-31.
		if disabled[label] && deps.enableLabel != nil {
			if err := deps.enableLabel(domain, label); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
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
