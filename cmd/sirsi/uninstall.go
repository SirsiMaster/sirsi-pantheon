package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

var uninstallConfirm bool

// uninstallCmd is Pantheon ghost-hunting itself: a clean, dry-run-first removal
// of its own runtime footprint — LaunchAgents, dev binaries, config, the menubar
// .app (to Trash), and its Full Disk Access grant — so it never leaves the kind
// of clutter Anubis/Ka exist to clean (Rule A1 dry-run; A19-safe via Trash).
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "𓁢 Remove Pantheon cleanly — files, LaunchAgents, and its Full Disk Access grant",
	Long: `Removes Pantheon's runtime footprint and resets its Full Disk Access grant.

Preview by default; pass --confirm to apply. Apps are moved to the Trash
(recoverable), not hard-deleted. Two things macOS forces to be manual and are
surfaced as guidance: Homebrew installs (use 'brew uninstall --cask') and any
/Applications/*.app bundle (Rule A19 forbids automated removal).`,
	RunE: func(_ *cobra.Command, _ []string) error {
		if !JsonOutput {
			output.Banner()
			output.Header("Uninstall Pantheon")
		}

		plan := setup.PlanUninstall()
		var present []setup.UninstallTarget
		for _, t := range plan {
			if t.Exists || t.Kind == "tcc" {
				present = append(present, t)
			}
		}

		if len(present) == 0 {
			output.Success("Nothing to remove — Pantheon's runtime footprint isn't present here.")
			return nil
		}

		for _, t := range present {
			output.Info("  %-12s %s", t.Kind, t.Path)
		}

		if !uninstallConfirm {
			cr := &output.CommandResult{
				Command: "sirsi uninstall",
				Summary: fmt.Sprintf("Dry run: %d items would be removed", len(present)),
			}
			cr.AddNextAction("sirsi uninstall --confirm", "Apply (apps → Trash; FDA grant reset)")
			cr.AddEvidence("Homebrew install", "also run: brew uninstall --cask sirsimaster/tools/sirsi-pantheon")
			cr.AddEvidence("/Applications/*.app", "remove manually — Rule A19 forbids automated removal")
			cr.Render()
			return nil
		}

		fmt.Fprint(os.Stderr, "\n  Remove all of the above? Apps go to Trash, FDA grant is reset. [y/N] ")
		resp, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if s := strings.TrimSpace(strings.ToLower(resp)); s != "y" && s != "yes" {
			output.Info("Canceled.")
			return nil
		}

		acted, errs := setup.Uninstall(false)
		for _, t := range acted {
			if t.Action == "tccutil-reset" {
				output.Success("  reset Full Disk Access grant for %s", t.Path)
			} else {
				output.Success("  %s %s", strings.TrimSuffix(t.Action, "e")+"ed", t.Path)
			}
		}
		for _, e := range errs {
			output.Warn("  %s", e)
		}

		output.Header("Manual steps macOS requires")
		output.Dim("  • Homebrew install?  brew uninstall --cask sirsimaster/tools/sirsi-pantheon")
		output.Dim("  • /Applications/Pantheon.app (if present) — drag to Trash (A19)")
		output.Dim("  • System Settings → Full Disk Access — remove any leftover greyed Sirsi rows")
		return nil
	},
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallConfirm, "confirm", false, "Apply the uninstall (default is a dry-run preview)")
	rootCmd.AddCommand(uninstallCmd)
}
