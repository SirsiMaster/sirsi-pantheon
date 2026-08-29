package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// UninstallTarget is one item `sirsi uninstall` manages.
type UninstallTarget struct {
	Kind   string // LaunchAgent | binary | config | app | tcc
	Path   string // filesystem path, or bundle id for tcc
	Action string // remove | trash | tccutil-reset
	Exists bool
}

// uninstallExecFn is the injectable exec seam (A16), mutex-guarded (A21) so a
// test swap never races. It runs the few system commands uninstall needs
// (launchctl, tccutil, osascript) without the tests touching the real system.
var (
	uninstallExecMu sync.RWMutex
	uninstallExecFn = func(name string, args ...string) error { return exec.Command(name, args...).Run() }
)

func getUninstallExec() func(string, ...string) error {
	uninstallExecMu.RLock()
	defer uninstallExecMu.RUnlock()
	return uninstallExecFn
}

func setUninstallExec(fn func(string, ...string) error) {
	uninstallExecMu.Lock()
	defer uninstallExecMu.Unlock()
	uninstallExecFn = fn
}

func targetExists(p string) bool { _, err := os.Stat(p); return err == nil }

// PlanUninstall returns the deterministic list of Pantheon's *runtime* footprint
// that `sirsi uninstall` owns. It deliberately does NOT manage:
//   - Homebrew-installed files — that's `brew uninstall --cask` (surfaced as guidance)
//   - anything under /Applications/*.app — Rule A19 forbids automated removal
//     (surfaced as a manual step)
//
// The TCC entry is always listed (we reset the bundle id regardless of file state).
func PlanUninstall() []UninstallTarget {
	home, _ := os.UserHomeDir()
	var ts []UninstallTarget
	add := func(kind, path, action string) {
		ts = append(ts, UninstallTarget{Kind: kind, Path: path, Action: action, Exists: targetExists(path)})
	}

	// LaunchAgents created by setup (menubar + supervisor).
	add("LaunchAgent", menubarPlistPath(), "remove")
	add("LaunchAgent", supervisorPlistPath(), "remove")

	// Dev-installed binaries under ~/.local/bin (brew's copies are brew's to remove).
	for _, b := range []string{"sirsi", "sirsi-menubar", "sirsi-gemma", "sirsi-agent"} {
		add("binary", filepath.Join(home, ".local", "bin", b), "remove")
	}

	// Config + state directories.
	for _, c := range []string{"pantheon", "sirsi", "anubis"} {
		add("config", filepath.Join(home, ".config", c), "remove")
	}

	// The menubar .app in the USER Applications folder → Trash (recoverable).
	// (Past and current names; /Applications/*.app stays manual per A19.)
	add("app", filepath.Join(home, "Applications", "Sirsi Pantheon.app"), "trash")
	add("app", filepath.Join(home, "Applications", "Sirsi Menubar.app"), "trash")

	// Full Disk Access grant — resettable because it's keyed on the stable bundle
	// id. (Bare-binary path-keyed grants can't be reset by tccutil; macOS requires
	// the user to remove those in System Settings — surfaced as guidance.)
	add("tcc", menubarPlistLabel, "tccutil-reset")

	return ts
}

// Uninstall executes the plan. dryRun=true returns the would-remove list and
// touches nothing. Apps go to Trash (recoverable); the FDA grant is reset; files
// are removed. Returns what was acted on and any non-fatal errors.
func Uninstall(dryRun bool) (acted []UninstallTarget, errs []string) {
	for _, t := range PlanUninstall() {
		if t.Kind != "tcc" && !t.Exists {
			continue // nothing on disk for this target
		}
		if dryRun {
			acted = append(acted, t)
			continue
		}
		var err error
		switch t.Action {
		case "remove":
			if t.Kind == "LaunchAgent" && runtime.GOOS == "darwin" {
				label := strings.TrimSuffix(filepath.Base(t.Path), ".plist")
				target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
				_ = getUninstallExec()("launchctl", "bootout", target)
				_ = getUninstallExec()("launchctl", "disable", target)
			}
			err = os.RemoveAll(t.Path)
		case "trash":
			err = trashPath(t.Path)
		case "tccutil-reset":
			if runtime.GOOS == "darwin" {
				err = getUninstallExec()("tccutil", "reset", "SystemPolicyAllFiles", t.Path)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", t.Action, t.Path, err))
			continue
		}
		acted = append(acted, t)
	}
	return acted, errs
}

// trashPath moves a path to the Trash via Finder (recoverable) rather than a hard
// delete — keeps uninstall reversible and is the only A19-respecting way to clear
// an .app bundle. Falls back to os.RemoveAll off darwin.
func trashPath(p string) error {
	if runtime.GOOS != "darwin" {
		return os.RemoveAll(p)
	}
	script := fmt.Sprintf(`tell application "Finder" to delete (POSIX file %q)`, p)
	return getUninstallExec()("osascript", "-e", script)
}
