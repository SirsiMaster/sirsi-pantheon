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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// launchdDeps are the OS seams (Rule A16) — tests stub these.
type launchdDeps struct {
	listLabels     func() (map[string]bool, error) // labels currently known to launchd
	disabledLabels func() (map[string]bool, error) // labels disabled in override DB
	enableLabel    func(domain, label string) error
	bootstrapPlist func(plistPath string) error
	uid            func() int
	isQuarantined  func() bool // true if the operator deliberately stopped the gemma broker
	// isFabricQuarantined, when true, blocks reviving EVERY label — the R7/G6
	// generalization of isQuarantined (which only ever covered quarantinedLabels).
	// nil is treated as "not quarantined" so existing callers/tests are unaffected.
	isFabricQuarantined func() bool
}

// ManagedLaunchdRecovery is the factual result of one bounded recovery pass.
// Enabled labels may already be loaded; Bootstrapped labels were absent from
// launchd and were loaded from an exact managed plist. Keeping those outcomes
// separate prevents the UI and activity journal from calling an enable-only
// repair a process restart.
type ManagedLaunchdRecovery struct {
	Enabled      []string
	Bootstrapped []string
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
		// Space-joined keys are split, or a quarantined label reads as enabled
		// and bootstrap fails with "Operation not permitted".
		disabled := map[string]bool{}
		for _, label := range platform.ParseDisabledLabels(string(out)) {
			disabled[label] = true
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
	isFabricQuarantined: func() bool {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		return IsFabricQuarantined(home)
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

// recoverManagedLaunchd restores each managed, on-disk plist in two explicit
// stages: clear a disabled override first, even if the label remains loaded;
// then bootstrap only labels absent from launchd. It never revives quarantined
// Gemma labels or anything outside the managed filename allowlist.
func recoverManagedLaunchd(agentsDir string, deps launchdDeps) (ManagedLaunchdRecovery, error) {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return ManagedLaunchdRecovery{}, nil
		}
		return ManagedLaunchdRecovery{}, err
	}
	// Fabric-wide stand-down (R7/G6): honored BEFORE touching launchd at all,
	// so a quarantined fabric reads as zero revivals rather than "everything
	// except the labels this duty happens to know about."
	if deps.isFabricQuarantined != nil && deps.isFabricQuarantined() {
		return ManagedLaunchdRecovery{}, nil
	}

	loaded, err := deps.listLabels()
	if err != nil {
		return ManagedLaunchdRecovery{}, fmt.Errorf("launchctl list: %w", err)
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

	var recovery ManagedLaunchdRecovery
	var firstErr error
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !managedPlist(name) {
			continue
		}
		plistPath := filepath.Join(agentsDir, name)
		info, statErr := os.Lstat(plistPath)
		if statErr != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			if firstErr == nil {
				if statErr != nil {
					firstErr = fmt.Errorf("inspect managed plist %s: %w", name, statErr)
				} else {
					firstErr = fmt.Errorf("managed plist %s is not a regular non-symlink file", name)
				}
			}
			continue
		}
		label := labelForPlist(name)
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
			recovery.Enabled = append(recovery.Enabled, label)
		}
		if loaded[label] {
			continue // enabled above if needed; its own launchd policy owns its process
		}
		if err := deps.bootstrapPlist(plistPath); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		recovery.Bootstrapped = append(recovery.Bootstrapped, label)
	}
	return recovery, firstErr
}

// KickstartDeadLabels is the supervisor-compatible wrapper. Its return value
// remains limited to labels that were absent and bootstrapped, so historical
// callers cannot mistake clearing a disabled override on a loaded process for
// a reload.
func KickstartDeadLabels(agentsDir string, deps launchdDeps) ([]string, error) {
	recovery, err := recoverManagedLaunchd(agentsDir, deps)
	return recovery.Bootstrapped, err
}

// RestoreManagedLaunchAgents is the explicit, operator-confirmed repair used
// by `sirsi liveness-watch restore-disabled`. It shares exactly the same
// managed-plist, quarantine, enable-before-bootstrap rules as the resident
// supervisor; it adds no broad launchctl authority.
func RestoreManagedLaunchAgents() (ManagedLaunchdRecovery, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ManagedLaunchdRecovery{}, err
	}
	return recoverManagedLaunchd(filepath.Join(home, "Library", "LaunchAgents"), launchdOS)
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
