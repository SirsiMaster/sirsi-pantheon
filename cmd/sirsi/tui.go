package main

import (
	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the full-screen terminal app (the operator console)",
	Long: `Open Pantheon's five-screen operator console (ADR-020):

  1 Pulse     memory-first — RAM gauge, pressure, top hogs, one-key relief
  2 Waste     scan → review → clean, with a freed-space proof
  3 Ghosts    residuals of uninstalled apps → clean
  4 Health    diagnose findings, each with its honest one-key fix
  5 Activity  provenance ledger — what sirsi actually changed

  tab next · 1-5 jump · ↑↓ move · enter inspect · r refresh · ? help · q quit

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
