package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
)

// `sirsi menubar quarantine` / `unquarantine` — the sanctioned way to set or
// clear liveness.MenubarQuarantineMarkerPath. Mirrors `sirsi gemma
// quarantine`: plist presence alone cannot tell liveness-watch "the owner
// took this offline on purpose" (a `launchctl bootout` leaves the exact
// plist in place), so an explicit marker is the only durable signal.
var menubarCmd = &cobra.Command{
	Use:   "menubar",
	Short: "Control the SwiftUI menubar surface",
}

var menubarQuarantineCmd = &cobra.Command{
	Use:   "quarantine",
	Short: "Stop the menubar and tell liveness-watch to stand down",
	Long: `Stops the SwiftUI menubar (if running) and writes the quarantine marker
liveness-watch checks BEFORE alerting on a down menubar. Without this, "the
menubar isn't running" has always meant "relaunch it" — there was no way to
say "I stopped this on purpose."

  sirsi menubar quarantine
  sirsi menubar unquarantine`,
	RunE: runMenubarQuarantine,
}

var menubarUnquarantineCmd = &cobra.Command{
	Use:   "unquarantine",
	Short: "Clear the quarantine marker so liveness-watch resumes alerting on a down menubar",
	RunE:  runMenubarUnquarantine,
}

func init() {
	menubarCmd.AddCommand(menubarQuarantineCmd, menubarUnquarantineCmd)
	rootCmd.AddCommand(menubarCmd)
}

func runMenubarQuarantine(cmd *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("menubar quarantine: %w", err)
	}
	// Best-effort graceful stop — quarantine is meaningful even if nothing is
	// currently running, so a failed kill here is not fatal to writing the marker.
	_ = exec.Command("pkill", "-f", "Sirsi Menubar.app/Contents/MacOS/SirsiMenubar").Run()

	path := liveness.MenubarQuarantineMarkerPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("menubar quarantine: %w", err)
	}
	if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600); err != nil {
		return fmt.Errorf("menubar quarantine: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Menubar quarantined. liveness-watch will not alert on it until `sirsi menubar unquarantine`.\n")
	return nil
}

func runMenubarUnquarantine(cmd *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("menubar unquarantine: %w", err)
	}
	path := liveness.MenubarQuarantineMarkerPath(home)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("menubar unquarantine: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Quarantine cleared. liveness-watch will alert on its next tick if the menubar is down.\n")
	return nil
}
