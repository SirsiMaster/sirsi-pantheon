// Package main — sirsi-gui
//
// ☥ Sirsi Pantheon — macOS GUI surface (the "Ferrari").
//
// The GUI is a native window over Pantheon's existing Go dashboard server
// (internal/dashboard — Go HTTP + embedded HTML, no Electron/React; Critical
// Design Decision #2). It is one more face over the same engine: if the
// menubar is already serving the dashboard on the fixed port, the GUI reuses
// that server; otherwise it starts its own. No capability lives in the GUI
// that the CLI/TUI/menubar lack — only the presentation differs.
package main

import (
	"fmt"
	"os"
	"runtime"

	webview "github.com/webview/webview_go"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

func main() {
	// WebKit must be driven from the main OS thread on macOS.
	runtime.LockOSThread()

	sneAccessToken, err := dashboard.LoadOrCreateDefaultSNELocalAccessToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sirsi-gui: refusing unauthenticated startup: %v\n", err)
		os.Exit(1)
	}
	sneAccessPath, err := dashboard.DefaultSNELocalAccessTokenPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sirsi-gui: refusing startup without durable capability path: %v\n", err)
		os.Exit(1)
	}

	// Reuse a running dashboard (e.g. the menubar's) if present; else start one.
	srv := dashboard.New(dashboard.Config{
		SirsiBin:                setup.SirsiBinaryPath(),
		SNELocalAccessToken:     sneAccessToken,
		SNELocalAccessTokenPath: sneAccessPath,
	})
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "sirsi-gui: reusing existing dashboard (%v)\n", err)
	} else {
		defer func() { _ = srv.Stop() }()
	}

	// Mark the GUI as the active surface for `sirsi surface`.
	_ = setup.SaveActiveSurface(setup.SurfaceGUI)

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("Sirsi Pantheon")
	w.SetSize(1100, 720, webview.HintNone)
	w.Navigate(srv.URL())
	w.Run()
}
