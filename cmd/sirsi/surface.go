package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/tui"
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
  sirsi surface use ide      Make Pantheon reachable from your AI IDE (MCP)
  sirsi surface install gui  Install and start the macOS caretaker at login`,
	RunE: func(_ *cobra.Command, _ []string) error {
		output.Header("Surfaces")
		fmt.Println()
		active := setup.ActiveSurface()
		fmt.Printf("  Active: %s\n\n", active.Title())
		caretakerInstalled, caretakerLoaded := setup.MenubarRegistration()
		caretakerStatus := "not installed"
		if caretakerInstalled {
			caretakerStatus = "installed, stopped"
		}
		if caretakerLoaded {
			caretakerStatus = "installed + loaded"
		}

		rows := [][]string{
			{marker(active, setup.SurfaceCLI), "cli", "available", setup.SurfaceCLI.Detail()},
			{marker(active, setup.SurfaceTUI), "tui", "available", setup.SurfaceTUI.Detail()},
			{marker(active, setup.SurfaceGUI), "gui", caretakerStatus, "Native macOS window over the Pantheon dashboard"},
			{marker(active, setup.SurfaceIDE), "ide", "configure in setup", setup.SurfaceIDE.Detail()},
		}
		output.Table([]string{"", "Surface", "Status", "What it is"}, rows)
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
		// The TUI runs in this terminal, so launch it directly rather than
		// returning a message (the engine can't import the cmd-layer program).
		if s == setup.SurfaceTUI {
			_ = setup.SaveActiveSurface(setup.SurfaceTUI)
			return tui.Run()
		}
		msg, err := setup.LaunchSurface(s)
		if err != nil {
			return err
		}
		output.Success("%s", msg)
		return nil
	},
}

var surfaceInstallCmd = &cobra.Command{
	Use:   "install <gui|menubar>",
	Short: "Install one Pantheon surface without running the setup wizard",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		s, err := parseInstallableSurfaceArg(args[0])
		if err != nil {
			return err
		}
		result := setup.Install(s)
		if result.Status != setup.StatusOK {
			return fmt.Errorf("install %s: %s", args[0], result.Message)
		}
		output.Success("%s", result.Message)
		return nil
	},
}

func init() {
	surfaceCmd.AddCommand(surfaceUseCmd)
	surfaceCmd.AddCommand(surfaceInstallCmd)
	rootCmd.AddCommand(surfaceCmd)
}

func parseInstallableSurfaceArg(tok string) (setup.Surface, error) {
	if tok == "gui" || tok == "menubar" {
		return setup.SurfaceMenubar, nil
	}
	return "", fmt.Errorf("surface %q is not separately installable (want: gui or menubar)", tok)
}

// parseSurfaceArg maps a user token to a Surface. "gui" and "menubar" both map
// to the menu bar surface.
func parseSurfaceArg(tok string) (setup.Surface, error) {
	switch tok {
	case "cli":
		return setup.SurfaceCLI, nil
	case "tui":
		return setup.SurfaceTUI, nil
	case "gui":
		return setup.SurfaceGUI, nil
	case "menubar":
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
