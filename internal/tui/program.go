package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// Run builds and runs the five-screen operator console over live workstation
// data, read through the injectable command runner. It blocks until the user
// quits and returns any terminal/runtime error.
//
// Capabilities are resolved from the environment; the console selects the
// alternate screen for the fullscreen path and mouse cell-motion for optional
// click-to-focus, but is 100% keyboard-complete with the mouse disabled (§3.3).
func Run() error {
	caps := DetectCapabilities(os.Getenv)
	app, err := NewApp(caps)
	if err != nil {
		return err
	}
	// Alt-screen is carried on the View (App.View), so no program option is
	// needed here; the renderer enters/exits it based on that flag.
	_, err = tea.NewProgram(app).Run()
	return err
}
