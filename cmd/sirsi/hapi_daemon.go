package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

// The Hapi daemon makes the memory governor ALWAYS-ON. `sirsi hapi watch` only
// governs while a terminal holds it open; the daemon installs a macOS LaunchAgent
// that runs the watcher at login and restarts it on crash, so the "protect" pillar
// is live without anyone babysitting a window. It mirrors the Horus supervisor
// LaunchAgent pattern (internal/setup/supervisor.go).
//
// Safety posture (A1 + A5): the daemon defaults to GOVERN mode, which is safe by
// construction — teeth (suspend→kill) only ever touch processes that registered
// themselves as governed (the broker self-registers; nothing else opts in), and
// SIGSTOP (reversible) is always tried before SIGTERM. That honors A5's
// "conservative with GPU/compute processes": a healthy inference job is never
// arbitrarily killed; only a consented runaway, only when the host itself is about
// to Jetsam, and reversibly first. `--warn-only` installs an observe-only daemon
// (no teeth at all) for those who want surfacing without intervention.

const hapiDaemonLabel = "ai.sirsi.hapi"

var (
	hapiInstallWarnOnly bool
	hapiInstallInterval int
)

var hapiInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the Hapi memory governor as a background LaunchAgent (always-on, starts at login)",
	Long: `Install runs the memory governor as a macOS LaunchAgent so it protects the
host continuously — at login and across crashes — without a foreground terminal.

  sirsi hapi install              # govern mode (default): suspend→kill a GOVERNED
                                  # runaway before Jetsam; safe — teeth only touch
                                  # processes that registered as governed
  sirsi hapi install --warn-only  # observe-only: surface pressure, never intervene
  sirsi hapi install --interval 5 # sample every 5s (default 3s)
  sirsi hapi uninstall            # remove it

Reversible: ` + "`sirsi hapi uninstall`" + ` unloads and deletes the LaunchAgent.`,
	RunE: runHapiInstall,
}

var hapiUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the Hapi memory-governor LaunchAgent",
	RunE:  runHapiUninstall,
}

func init() {
	hapiInstallCmd.Flags().BoolVar(&hapiInstallWarnOnly, "warn-only", false, "Observe-only daemon: surface pressure but never suspend/kill")
	hapiInstallCmd.Flags().IntVar(&hapiInstallInterval, "interval", 3, "Sampling interval in seconds")
	hapiCmd.AddCommand(hapiInstallCmd, hapiUninstallCmd)
}

func hapiDaemonPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", hapiDaemonLabel+".plist")
}

// hapiDaemonInstalled reports whether the LaunchAgent plist is in place (macOS).
func hapiDaemonInstalled() bool {
	return runtime.GOOS == "darwin" && fileExistsHapi(hapiDaemonPlistPath())
}

func fileExistsHapi(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// hapiDaemonPlist renders the LaunchAgent that runs `sirsi hapi watch`. Pure (no
// side effects) so it is unit-testable. govern=false adds nothing (warn-only is
// the bare watch); govern=true passes --govern. Logs to /tmp so a user can tail
// what Hapi is doing. ProcessType Background — a headless resident loop.
func hapiDaemonPlist(sirsiBin string, govern bool, interval int) string {
	args := fmt.Sprintf("exec %q hapi watch --interval %d", sirsiBin, interval)
	if govern {
		args += " --govern"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/zsh</string>
		<string>-l</string>
		<string>-c</string>
		<string>%[2]s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>/tmp/sirsi-hapi.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/sirsi-hapi.err</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, hapiDaemonLabel, args)
}

func runHapiInstall(cmd *cobra.Command, args []string) error {
	res := &output.CommandResult{Command: "sirsi hapi install"}
	if runtime.GOOS != "darwin" {
		res.Status = "skipped"
		res.Summary = "The Hapi daemon is a macOS LaunchAgent — not available on this OS (Mac-first, ADR-032)."
		res.Render()
		return nil
	}
	bin, err := os.Executable()
	if err != nil || bin == "" {
		res.Status = "error"
		res.Errors = []string{"could not resolve the sirsi binary path"}
		res.Render()
		return nil
	}
	if rp, e := filepath.EvalSymlinks(bin); e == nil {
		bin = rp
	}
	// Guard against the exit-127 class bug: never install a LaunchAgent for a
	// command this build can't run.
	if exec.Command(bin, "hapi", "watch", "--help").Run() != nil {
		res.Status = "error"
		res.Errors = []string{"`sirsi hapi watch` is not runnable in this build — refusing to install a broken LaunchAgent"}
		res.Render()
		return nil
	}

	govern := !hapiInstallWarnOnly
	path := hapiDaemonPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		res.Status, res.Errors = "error", []string{err.Error()}
		res.Render()
		return nil
	}
	if err := os.WriteFile(path, []byte(hapiDaemonPlist(bin, govern, hapiInstallInterval)), 0o644); err != nil {
		res.Status, res.Errors = "error", []string{err.Error()}
		res.Render()
		return nil
	}
	// Reload so a re-install picks up flag changes; unload error is fine (not loaded yet).
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		res.Status = "error"
		res.Errors = []string{"wrote the LaunchAgent but `launchctl load` failed: " + err.Error()}
		res.Render()
		return nil
	}

	mode := "GOVERN (suspend→kill a governed runaway before Jetsam; teeth only on consented compute)"
	if !govern {
		mode = "warn-only (surface pressure; never intervene)"
	}
	res.Status = "ok"
	res.Evidence = []output.Evidence{
		{Label: "Mode", Value: mode},
		{Label: "Interval", Value: fmt.Sprintf("%ds", hapiInstallInterval)},
		{Label: "LaunchAgent", Value: path},
		{Label: "Logs", Value: "/tmp/sirsi-hapi.log"},
	}
	res.Summary = "Hapi memory governor is now resident — it starts at login and restarts on crash, protecting the host continuously. Remove it with `sirsi hapi uninstall`."
	res.NextActions = []output.NextAction{{
		Label:       "Watch what it's doing",
		Command:     "hapi status",
		Description: "current pressure tier + whether the daemon is installed; `tail -f /tmp/sirsi-hapi.log` for the live loop",
	}}
	res.Render()
	return nil
}

func runHapiUninstall(cmd *cobra.Command, args []string) error {
	res := &output.CommandResult{Command: "sirsi hapi uninstall"}
	path := hapiDaemonPlistPath()
	if !fileExistsHapi(path) {
		res.Status = "ok"
		res.Summary = "No Hapi daemon installed — nothing to remove."
		res.Render()
		return nil
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil {
		res.Status, res.Errors = "error", []string{err.Error()}
		res.Render()
		return nil
	}
	res.Status = "ok"
	res.Summary = "Hapi daemon removed (LaunchAgent unloaded and deleted). The governor no longer runs in the background; `sirsi hapi watch` still works on demand."
	res.Render()
	return nil
}

// hapiDaemonStatusLine returns a short human label for the daemon's state, for the
// `sirsi hapi` status surface.
func hapiDaemonStatusLine() string {
	if runtime.GOOS != "darwin" {
		return "n/a (macOS only)"
	}
	if !hapiDaemonInstalled() {
		return "not installed (run `sirsi hapi install` for always-on protection)"
	}
	out, _ := exec.Command("launchctl", "list", hapiDaemonLabel).Output()
	if len(out) > 0 {
		return "installed + running (LaunchAgent " + hapiDaemonLabel + ")"
	}
	return "installed (not currently loaded — `sirsi hapi install` to reload)"
}
