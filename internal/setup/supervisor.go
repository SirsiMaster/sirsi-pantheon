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

// ── Horus agent-router supervisor ────────────────────────────────────────────
//
// The supervisor is a resident LaunchAgent that runs `sirsi horus supervise`
// (the agent-router resident loop, built separately). It is installed alongside
// the menubar on macOS: a background KeepAlive agent that starts at login and
// restarts on crash. The command runs from the repo root as an explicit
// WorkingDirectory — never the launchd default cwd — so router state under
// .agents/idea-router/ resolves correctly.

// supervisorPlistLabel is the LaunchAgent label for the Horus agent-router.
const supervisorPlistLabel = "ai.sirsi.horus.agent-router"

// supervisorPlistContent renders the LaunchAgent that runs `sirsi horus
// supervise` from an explicit WorkingDirectory.
//
// It pins WorkingDirectory to the repo root so router state resolves regardless
// of the launchd default cwd, and sets an explicit PATH via EnvironmentVariables
// so the supervisor's wakeability probe (exec.LookPath for `claude`, `codex`,
// etc.) finds the agent CLIs. A bare `/bin/zsh -l` is NOT enough: a
// non-interactive login shell sources .zprofile/.zshenv but not .zshrc, where
// ~/.local/bin is commonly added — so launchd would hand the supervisor a
// minimal PATH and every agent would be misreported as "not found in PATH".
// The PATH leads with the sirsi binary's own directory (agent CLIs are usually
// installed beside it). ProcessType is Background — a headless resident loop.
func supervisorPlistContent(sirsiBinPath, workDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>ai.sirsi.horus.agent-router</string>
	<key>WorkingDirectory</key>
	<string>%[2]s</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%[3]s</string>
	</dict>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/zsh</string>
		<string>-l</string>
		<string>-c</string>
		<string>exec %[1]q horus supervise</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/tmp/sirsi-horus-supervisor.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/sirsi-horus-supervisor.err</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, sirsiBinPath, workDir, supervisorPath(sirsiBinPath))
}

// supervisorPath builds the PATH the LaunchAgent exports so the supervisor's
// wakeability probe (exec.LookPath) can resolve agent CLIs under launchd's
// minimal default environment. It leads with the sirsi binary's own directory
// (agent CLIs are typically installed alongside it), then ~/.local/bin, common
// package-manager prefixes, and the system dirs, de-duplicated in order.
func supervisorPath(sirsiBinPath string) string {
	dirs := []string{filepath.Dir(sirsiBinPath)}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	dirs = append(dirs,
		"/opt/homebrew/bin", "/usr/local/bin",
		"/usr/bin", "/bin", "/usr/sbin", "/sbin",
	)
	seen := make(map[string]bool, len(dirs))
	uniq := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" || d == "." || seen[d] {
			continue
		}
		seen[d] = true
		uniq = append(uniq, d)
	}
	return strings.Join(uniq, ":")
}

// supervisorPlistPath is where the LaunchAgent is written.
func supervisorPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", supervisorPlistLabel+".plist")
}

// SupervisorInstalled reports whether the Horus agent-router LaunchAgent is in
// place. macOS only.
func SupervisorInstalled() bool {
	return runtime.GOOS == "darwin" && fileExists(supervisorPlistPath())
}

// InstallSupervisor writes the Horus agent-router LaunchAgent and loads it so
// `sirsi horus supervise` starts now and at every login, running from the repo
// root. macOS only; idempotent (a re-install reloads).
func InstallSupervisor() InstallResult {
	res := InstallResult{Surface: SurfaceSupervisor}
	if runtime.GOOS != "darwin" {
		res.Status, res.Message = StatusSkipped, "agent-router supervisor is macOS only"
		return res
	}
	bin := BinaryPath()
	if bin == "" {
		res.Status = StatusFailed
		res.Message = "sirsi binary path could not be resolved"
		return res
	}
	// Guard: only install the LaunchAgent if `sirsi horus supervise` actually
	// exists in this build. Installing it for a missing command would create a
	// failing LaunchAgent (the menubar exit-127 class bug). The command ships
	// from the router/Horus lane; until then this skips cleanly so setup never
	// installs a broken resident agent.
	if exec.Command(bin, "horus", "supervise", "--help").Run() != nil {
		res.Status = StatusSkipped
		res.Message = "skipped — `sirsi horus supervise` not available in this build yet"
		return res
	}
	workDir, err := router.FindRepoRoot()
	if err != nil {
		res.Status = StatusFailed
		res.Message = "could not resolve repo root for WorkingDirectory: " + err.Error()
		return res
	}
	// Root-ful invocation: persist the clone root so app-context callers (the
	// menubar's launchd-spawned levers, cwd=/) can resolve it later (B2).
	router.RememberRepoRoot(workDir)
	path := supervisorPlistPath()
	content := supervisorPlistContent(bin, workDir)
	// Idempotence: an already-correct plist means the resident supervisor is
	// already the one we would install — leave it alone rather than bouncing a
	// healthy KeepAlive loop on every setup/install-daemons run.
	if existing, rerr := os.ReadFile(path); rerr == nil && string(existing) == content {
		res.Status, res.Message = StatusOK, "agent-router supervisor already installed (no change)"
		return res
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		res.Status, res.Message = StatusFailed, err.Error()
		return res
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		res.Status, res.Message = StatusFailed, err.Error()
		return res
	}
	// Reload: unload first so a re-install picks up changes; ignore unload error
	// (it fails when not yet loaded, which is fine).
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		res.Status = StatusFailed
		res.Message = "wrote LaunchAgent but launchctl load failed: " + err.Error()
		return res
	}
	res.Status, res.Message = StatusOK, "agent-router supervisor installed and started"
	return res
}
