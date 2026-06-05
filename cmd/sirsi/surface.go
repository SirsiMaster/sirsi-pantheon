package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

var surfaceCmd = &cobra.Command{
	Use:   "surface",
	Short: "Show or switch the Pantheon surface (CLI / TUI / GUI / IDE)",
	Long: `Every surface is a face over the same engine — switching never changes
what Pantheon can do, only how you drive it.

  sirsi surface              Show the active surface and what's installed
  sirsi surface use cli      Switch to direct terminal commands
  sirsi surface use tui      Switch to the full-screen terminal app
  sirsi surface use gui      Bring the macOS menu bar app forward
  sirsi surface use ide      Make Pantheon reachable from your AI IDE (MCP)`,
	RunE: func(_ *cobra.Command, _ []string) error {
		output.Header("Surfaces")
		fmt.Println()
		active := setup.ActiveSurface()
		fmt.Printf("  Active: %s\n\n", active.Title())

		rows := [][]string{
			{marker(active, setup.SurfaceCLI), "cli", setup.SurfaceCLI.Detail()},
			{marker(active, setup.SurfaceTUI), "tui", setup.SurfaceTUI.Detail()},
			{marker(active, setup.SurfaceGUI), "gui", "macOS menu bar / native app"},
			{marker(active, setup.SurfaceIDE), "ide", setup.SurfaceIDE.Detail()},
		}
		output.Table([]string{"", "Surface", "What it is"}, rows)
		fmt.Println()
		fmt.Println("  Switch with:  sirsi surface use <cli|tui|gui|ide>")
		return nil
	},
}

var surfaceUseCmd = &cobra.Command{
	Use:   "use <cli|tui|gui|ide>",
	Short: "Switch the active surface",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		s, err := parseSurfaceArg(args[0])
		if err != nil {
			return err
		}
		msg, err := setup.LaunchSurface(s)
		if err != nil {
			return err
		}
		output.Success("%s", msg)
		return nil
	},
}

func init() {
	surfaceCmd.AddCommand(surfaceUseCmd)
	rootCmd.AddCommand(surfaceCmd)
}

// parseSurfaceArg maps a user token to a Surface. "gui" and "menubar" both map
// to the menu bar surface.
func parseSurfaceArg(tok string) (setup.Surface, error) {
	switch tok {
	case "cli":
		return setup.SurfaceCLI, nil
	case "tui":
		return setup.SurfaceTUI, nil
	case "gui", "menubar":
		return setup.SurfaceMenubar, nil
	case "ide", "mcp":
		return setup.SurfaceIDE, nil
	}
	return "", fmt.Errorf("unknown surface %q (want: cli, tui, gui, ide)", tok)
}

// marker returns a pointer glyph for the active surface, blank otherwise.
func marker(active, s setup.Surface) string {
	if active == s || (s == setup.SurfaceGUI && active == setup.SurfaceMenubar) {
		return "▸"
	}
	return ""
}
