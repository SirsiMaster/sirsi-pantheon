package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// launchctlExecFn indirects `launchctl` so tests can stub it without touching
// the real system (mirrors uninstall.go's uninstallExecFn pattern). Production
// shells the real binary.
var launchctlExecFn = func(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, detail)
}

func runLaunchctl(args ...string) error { return launchctlExecFn(args...) }

// ── Router automation: ONE backstop, not three ────────────────────────────────
//
// The backlog ruling (20260629-230327) is explicit: fold background router
// duties into the resident supervisor — a SINGLE backstop — instead of
// accreting LaunchAgents. The dispatch pump, hourly sweep, and registry-police
// passes now run inside `sirsi horus supervise` (see
// internal/router/supervisorduties.go), cadence-gated and error-isolated.
//
// `sirsi router install-daemons` therefore does two things:
//
//  1. Ensures the ONE supervisor LaunchAgent (ai.sirsi.horus.agent-router) is
//     installed and loaded — reusing InstallSupervisor, the same installer
//     `sirsi setup` runs.
//  2. MIGRATES AWAY the three legacy per-duty LaunchAgents when present
//     (com.sirsi.idea-router, com.sirsi.idea-router-sweep,
//     ai.sirsi.registry-police): each is unloaded from launchd and its plist
//     removed, and the migration is reported so the operator sees exactly
//     what changed. Idempotent — a second run finds nothing to migrate.

// statusWord renders an InstallResult Status for a composed report line.
func statusWord(s Status) string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusMissing:
		return "missing"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	}
	return fmt.Sprintf("status-%d", int(s))
}

// legacyRouterDaemonLabels are the three per-duty LaunchAgents the supervisor
// fold replaces. Order is the report order.
var legacyRouterDaemonLabels = []string{
	"com.sirsi.idea-router",
	"com.sirsi.idea-router-sweep",
	"ai.sirsi.registry-police",
}

// launchAgentPath returns the ~/Library/LaunchAgents plist path for a label.
func launchAgentPath(label string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// installSupervisorFn indirects InstallSupervisor so tests can exercise the
// migration flow without writing a real LaunchAgent or shelling launchctl.
var installSupervisorFn = InstallSupervisor

// RouterAutomationHealthy reports whether the single-backstop shape holds:
// the supervisor LaunchAgent is installed AND no legacy per-duty plist
// remains. macOS only (false elsewhere).
func RouterAutomationHealthy() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if !SupervisorInstalled() {
		return false
	}
	for _, label := range legacyRouterDaemonLabels {
		if fileExists(launchAgentPath(label)) {
			return false
		}
	}
	return true
}

// migrateLegacyRouterDaemons unloads and removes any legacy per-duty plists.
// Returns the labels migrated and any problems encountered. An unload failure
// alone is tolerated (the agent may simply not be loaded); a plist that cannot
// be removed is a real problem because the legacy job would keep firing.
func migrateLegacyRouterDaemons() (migrated []string, problems []string) {
	for _, label := range legacyRouterDaemonLabels {
		plist := launchAgentPath(label)
		if !fileExists(plist) {
			continue
		}
		// Unload first so launchd forgets the job; error is fine when it was
		// not loaded to begin with.
		_ = runLaunchctl("unload", plist)
		if err := os.Remove(plist); err != nil {
			problems = append(problems, fmt.Sprintf("%s: unloaded but plist removal failed: %v", label, err))
			continue
		}
		migrated = append(migrated, label)
	}
	return migrated, problems
}

// InstallRouterDaemons ensures the single-backstop router automation: the ONE
// supervisor LaunchAgent installed and loaded, and the three legacy per-duty
// LaunchAgents migrated away when present. Idempotent; macOS only.
func InstallRouterDaemons() InstallResult {
	res := InstallResult{Surface: SurfaceRouterDaemons}
	if runtime.GOOS != "darwin" {
		res.Status, res.Message = StatusSkipped, "router automation is macOS only"
		return res
	}

	sup := installSupervisorFn()
	migrated, problems := migrateLegacyRouterDaemons()

	var parts []string
	parts = append(parts, "supervisor: "+statusWord(sup.Status)+" ("+sup.Message+")")
	switch {
	case len(migrated) > 0:
		parts = append(parts, fmt.Sprintf("migrated away %d legacy agent(s): %s",
			len(migrated), strings.Join(migrated, ", ")))
	default:
		parts = append(parts, "no legacy agents to migrate")
	}
	if len(problems) > 0 {
		parts = append(parts, strings.Join(problems, "; "))
	}
	res.Message = strings.Join(parts, " — ")

	switch {
	case sup.Status == StatusFailed:
		res.Status = StatusFailed
	case len(problems) > 0:
		if len(migrated) > 0 {
			res.Status = StatusOK // partial migration is still forward progress
		} else {
			res.Status = StatusFailed
		}
	default:
		res.Status = StatusOK
	}
	return res
}
