package main

// conduitarm.go — `sirsi conduit arm|disarm|status` (ADR-039 P4): the
// self-healing trigger mesh that fires the tick continuously.
//
// The daemons that failed before were RESIDENT KeepAlive processes — one
// long-lived process that had to stay alive, and died silently. This is the
// inversion: launchd fires a SHORT-LIVED `sirsi conduit tick` on a StartInterval
// AND whenever the inbox (items/) changes (WatchPaths). KeepAlive is FALSE — each
// tick plans, dispatches, and EXITS, so there is no resident process to leak,
// hang, or exit-78. A tick that dies just misses one firing; the next interval or
// inbox change fires another. That redundancy is the continuity.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

const conduitLabel = "ai.sirsi.conduit.tick"

var (
	conduitArmInterval int
)

var conduitArmCmd = &cobra.Command{
	Use:   "arm",
	Short: "Install the continuous trigger mesh — periodic + inbox-change ticks (macOS)",
	Long: `Arms the continuous work surface: a launchd agent fires 'sirsi conduit tick'
on an interval AND whenever the router inbox changes. Short-lived ticks, no
resident daemon — the design that survives what killed the old wake loops.

Ticks only DISPATCH when autonomous is on (` + "`sirsi autonomous on`" + `); armed but
autonomous-off, the loop ticks continuously and holds everything as proposals.

  sirsi conduit arm                 Arm with the default 120s interval
  sirsi conduit arm --interval 60   Arm with a 60s tick interval`,
	RunE: runConduitArm,
}

var conduitDisarmCmd = &cobra.Command{
	Use:   "disarm",
	Short: "Remove the continuous trigger mesh",
	RunE:  runConduitDisarm,
}

var conduitStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the continuous trigger mesh is armed",
	RunE:  runConduitStatus,
}

func init() {
	conduitArmCmd.Flags().IntVar(&conduitArmInterval, "interval", 120, "Seconds between periodic ticks")
	conduitCmd.AddCommand(conduitArmCmd, conduitDisarmCmd, conduitStatusCmd)
}

func conduitPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", conduitLabel+".plist")
}

func conduitPlist(sirsiBin, workDir, itemsDir string, interval int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>WorkingDirectory</key>
	<string>%[3]s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%[2]s</string>
		<string>conduit</string>
		<string>tick</string>
	</array>
	<key>StartInterval</key>
	<integer>%[5]d</integer>
	<key>WatchPaths</key>
	<array>
		<string>%[4]s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>/tmp/sirsi-conduit-tick.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/sirsi-conduit-tick.log</string>
</dict>
</plist>
`, conduitLabel, sirsiBin, workDir, itemsDir, interval)
}

func runConduitArm(_ *cobra.Command, _ []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("conduit arm is macOS-only for now (launchd); Linux/systemd + Windows Task Scheduler triggers are the next mesh renderers")
	}
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("no idea-router found: %w", err)
	}
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve sirsi binary: %w", err)
	}
	if stable, sErr := router.ResolveStableBinary(repoRoot, bin); sErr == nil {
		bin = stable
	}
	itemsDir := filepath.Join(repoRoot, ".agents", "idea-router", "items")
	plistPath := conduitPlistPath()
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(conduitPlist(bin, repoRoot, itemsDir, conduitArmInterval)), 0o644); err != nil {
		return fmt.Errorf("write conduit plist: %w", err)
	}
	// Reload idempotently: bootout any prior instance, then bootstrap.
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain, plistPath).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w\n%s", err, out)
	}
	fmt.Printf("𓁢 conduit armed — ticks every %ds + on every inbox change (label %s)\n", conduitArmInterval, conduitLabel)
	fmt.Println("   short-lived ticks, no resident daemon. Dispatches only when `sirsi autonomous on`.")
	return nil
}

func runConduitDisarm(_ *cobra.Command, _ []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("conduit disarm is macOS-only")
	}
	plistPath := conduitPlistPath()
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain, plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove conduit plist: %w", err)
	}
	fmt.Println("𓁢 conduit disarmed — the trigger mesh is removed.")
	return nil
}

func runConduitStatus(_ *cobra.Command, _ []string) error {
	armed := runtime.GOOS == "darwin"
	if armed {
		if _, err := os.Stat(conduitPlistPath()); err != nil {
			armed = false
		}
	}
	if armed {
		fmt.Printf("𓁢 conduit: ARMED (%s)\n", conduitLabel)
	} else {
		fmt.Println("𓁢 conduit: not armed — run `sirsi conduit arm`")
	}
	return nil
}
