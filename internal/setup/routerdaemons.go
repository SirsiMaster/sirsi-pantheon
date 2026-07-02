package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// launchctlExecFn indirects `launchctl` so tests can stub it without touching the
// real system (mirrors uninstall.go's uninstallExecFn pattern). Production shells
// the real binary.
var launchctlExecFn = func(args ...string) error {
	return exec.Command("launchctl", args...).Run()
}

func runLaunchctl(args ...string) error { return launchctlExecFn(args...) }

// ── Router LaunchAgents (idea-router, sweep, registry-police) ─────────────────
//
// node-status reports three router helper LaunchAgents that Pantheon relies on
// to relay work between agents while no session is open:
//
//   - com.sirsi.idea-router        FSEvents-driven dispatch: watches the router
//     state/items/proposals and runs one dispatch pass on any change (zero idle
//     process).
//   - com.sirsi.idea-router-sweep  hourly reconciliation sweep of the queue.
//   - ai.sirsi.registry-police     periodic thread-registry reap of OS-dead PIDs.
//
// Each is a thin LaunchAgent that runs a script shipped in the repo under
// .agents/idea-router/. When node-status shows one as NOT installed (or installed
// but program_found=false), work silently strands. `sirsi router install-daemons`
// (re)writes the missing plists pointing at the real script paths for THIS clone
// and loads them. Permissions/installs belong in setup, so it also runs from
// `sirsi setup` on macOS (feedback_permissions_in_install).
//
// Idempotent: an already-correct plist is left untouched (reported "ok"); a
// missing or drifted one is rewritten and (re)loaded. macOS only.

// routerDaemonSpec describes one router LaunchAgent Pantheon installs.
type routerDaemonSpec struct {
	Label      string // launchd label / plist basename
	Role       string // human role (matches node-status LaunchAgentHealth.Role)
	ScriptRel  string // script path relative to the router root (.agents/idea-router)
	LogBase    string // log basename stem under .agents/idea-router/logs
	Trigger    string // "watch" (WatchPaths+FSEvents) or "interval" (StartInterval)
	IntervalS  int    // StartInterval seconds when Trigger=="interval"
	RunAtLoad  bool   // launch immediately on load
	ThrottleS  int    // ThrottleInterval seconds (watch-throttle), 0 = omit
	WatchPaths []string
}

// routerDaemonSpecs returns the three router helpers, resolved against a router
// root (.agents/idea-router). WatchPaths and script paths are absolute for that
// clone. Mirrors the shapes the manually-installed daemons already use so a
// re-install is byte-stable and node-status flips program_found → true.
func routerDaemonSpecs(routerRoot string) []routerDaemonSpec {
	return []routerDaemonSpec{
		{
			Label:     "com.sirsi.idea-router",
			Role:      "router-watchpaths",
			ScriptRel: "run-on-event.sh",
			LogBase:   "launchd.idea-router",
			Trigger:   "watch",
			ThrottleS: 10,
			RunAtLoad: false,
			WatchPaths: []string{
				filepath.Join(routerRoot, "state.json"),
				filepath.Join(routerRoot, "items"),
				filepath.Join(routerRoot, "proposals"),
			},
		},
		{
			Label:     "com.sirsi.idea-router-sweep",
			Role:      "router-sweep",
			ScriptRel: "sweep.sh",
			LogBase:   "launchd.sweep",
			Trigger:   "interval",
			IntervalS: 3600,
			RunAtLoad: true,
		},
		{
			Label:     "ai.sirsi.registry-police",
			Role:      "registry-police",
			ScriptRel: filepath.Join("police", "registry-police.sh"),
			LogBase:   "launchd.police",
			Trigger:   "interval",
			IntervalS: 600,
			RunAtLoad: true,
		},
	}
}

// routerDaemonPlist renders the LaunchAgent for a router helper. The script is
// run directly (it is the real dispatch/sweep/police script shipped in the repo),
// with an explicit PATH so the script's `sirsi` invocations resolve under
// launchd's minimal environment. All interpolated values are XML-escaped.
func routerDaemonPlist(spec routerDaemonSpec, scriptPath, workDir, logDir, path string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "  <key>Label</key><string>%s</string>\n", xmlEscape(spec.Label))
	// ProgramArguments on separate lines so router.LaunchAgentProgram (node-status's
	// program_found probe) can resolve the executable — its parser only recognizes
	// the <string> when the <key>ProgramArguments</key> / <array> / <string> tags
	// are line-separated. Inline (as the older hand-installed plists wrote it) reads
	// as program_found=false forever even when the script exists.
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(scriptPath))
	b.WriteString("  </array>\n")
	fmt.Fprintf(&b, "  <key>WorkingDirectory</key><string>%s</string>\n", xmlEscape(workDir))
	fmt.Fprintf(&b, "  <key>EnvironmentVariables</key><dict><key>PATH</key><string>%s</string></dict>\n", xmlEscape(path))
	switch spec.Trigger {
	case "watch":
		b.WriteString("  <key>WatchPaths</key><array>\n")
		for _, w := range spec.WatchPaths {
			fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(w))
		}
		b.WriteString("  </array>\n")
		if spec.ThrottleS > 0 {
			fmt.Fprintf(&b, "  <key>ThrottleInterval</key><integer>%d</integer>\n", spec.ThrottleS)
		}
	case "interval":
		fmt.Fprintf(&b, "  <key>StartInterval</key><integer>%d</integer>\n", spec.IntervalS)
	}
	if spec.RunAtLoad {
		b.WriteString("  <key>RunAtLoad</key><true/>\n")
	} else {
		b.WriteString("  <key>RunAtLoad</key><false/>\n")
	}
	fmt.Fprintf(&b, "  <key>StandardOutPath</key><string>%s</string>\n", xmlEscape(filepath.Join(logDir, spec.LogBase+".out.log")))
	fmt.Fprintf(&b, "  <key>StandardErrorPath</key><string>%s</string>\n", xmlEscape(filepath.Join(logDir, spec.LogBase+".err.log")))
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

// xmlEscape escapes the five XML predefined entities for plist text.
func xmlEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

// launchAgentPath returns the ~/Library/LaunchAgents plist path for a label.
func launchAgentPath(label string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// RouterDaemonsInstalled reports whether all three router helper LaunchAgents are
// present on disk. macOS only.
func RouterDaemonsInstalled() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	root, err := router.FindRepoRoot()
	if err != nil {
		return false
	}
	routerRoot := filepath.Join(root, ".agents", "idea-router")
	for _, spec := range routerDaemonSpecs(routerRoot) {
		if !fileExists(launchAgentPath(spec.Label)) {
			return false
		}
	}
	return true
}

// InstallRouterDaemons idempotently (re)installs the three router helper
// LaunchAgents for this clone and loads any it wrote or corrected. A daemon whose
// backing script is missing is SKIPPED (never install a LaunchAgent that would
// exit non-zero — the menubar exit-127 class bug). macOS only.
//
// Return: StatusOK when every applicable daemon is installed/correct;
// StatusSkipped on a non-darwin host or when no repo root resolves; StatusFailed
// only if a write/load genuinely fails.
func InstallRouterDaemons() InstallResult {
	if runtime.GOOS != "darwin" {
		return InstallResult{Surface: SurfaceRouterDaemons, Status: StatusSkipped, Message: "router daemons are macOS only"}
	}
	root, err := router.FindRepoRoot()
	if err != nil {
		return InstallResult{Surface: SurfaceRouterDaemons, Status: StatusSkipped, Message: "no Pantheon clone found — nothing to install"}
	}
	return installRouterDaemonsAt(root)
}

// installRouterDaemonsAt is the repo-root-parameterized core of
// InstallRouterDaemons. Split out so tests drive it against a scaffolded clone
// without depending on FindRepoRoot's git-common-dir resolution (which would
// otherwise resolve to the real checkout).
func installRouterDaemonsAt(root string) InstallResult {
	res := InstallResult{Surface: SurfaceRouterDaemons}
	routerRoot := filepath.Join(root, ".agents", "idea-router")
	logDir := filepath.Join(routerRoot, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		res.Status, res.Message = StatusFailed, "create logs dir: "+err.Error()
		return res
	}
	agentDir := filepath.Join(mustHome(), "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		res.Status, res.Message = StatusFailed, "create LaunchAgents dir: "+err.Error()
		return res
	}

	pathEnv := supervisorPath(BinaryPath())
	var installed, skipped, unchanged int
	var problems []string
	for _, spec := range routerDaemonSpecs(routerRoot) {
		scriptPath := filepath.Join(routerRoot, spec.ScriptRel)
		if !fileExists(scriptPath) {
			// Never install a LaunchAgent whose program is missing.
			skipped++
			problems = append(problems, fmt.Sprintf("%s skipped (script missing: %s)", spec.Label, spec.ScriptRel))
			continue
		}
		content := routerDaemonPlist(spec, scriptPath, root, logDir, pathEnv)
		plistPath := launchAgentPath(spec.Label)
		if existing, rerr := os.ReadFile(plistPath); rerr == nil && string(existing) == content {
			unchanged++
			continue
		}
		if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
			problems = append(problems, fmt.Sprintf("%s write failed: %v", spec.Label, err))
			continue
		}
		// Reload so a re-install picks up path/trigger changes; unload error is fine
		// (fails when not yet loaded).
		_ = runLaunchctl("unload", plistPath)
		if err := runLaunchctl("load", plistPath); err != nil {
			problems = append(problems, fmt.Sprintf("%s wrote plist but load failed: %v", spec.Label, err))
			continue
		}
		installed++
	}

	switch {
	case len(problems) > 0:
		if installed+unchanged > 0 {
			res.Status = StatusOK // partial success is still forward progress
		} else {
			res.Status = StatusFailed
		}
		res.Message = fmt.Sprintf("%d installed, %d unchanged, %d skipped — %s",
			installed, unchanged, skipped, strings.Join(problems, "; "))
	case installed == 0 && unchanged > 0:
		res.Status, res.Message = StatusOK, fmt.Sprintf("router daemons already installed (%d, no change)", unchanged)
	default:
		res.Status, res.Message = StatusOK, fmt.Sprintf("router daemons installed (%d new, %d unchanged)", installed, unchanged)
	}
	return res
}

// mustHome returns the user home dir (empty on failure — callers only use it to
// build a path that a later Stat/Write will surface an error for).
func mustHome() string {
	h, _ := os.UserHomeDir()
	return h
}
