package main

import (
	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the full-screen terminal app (the TUI surface)",
	Long: `Open Pantheon's full-screen terminal console: Scan, Ra Fleet, and Router
Inbox views, fully keyboard-driven.

  ↑/↓ move · enter inspect · tab next view · / filter · r refresh · q quit

Every surface is a face over the same engine — switch anytime with
'sirsi surface use <cli|tui|gui|ide>'.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Launching the TUI makes it the active surface.
		_ = setup.SaveActiveSurface(setup.SurfaceTUI)
		return tui.Run()
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
