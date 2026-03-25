// Package main — pantheon-menubar
//
// 𓂀 Pantheon Menu Bar Application (ADR-010)
//
// A native macOS menu bar application that gives Pantheon a persistent visual
// presence. Appears as an ankh icon (☥) in the macOS menu bar with:
//
//   - Live stats panel (RAM, Git status, accelerator, active deities)
//   - Command shortcuts (Scan, Judge, Guard, Ka, Mirror)
//   - Quick actions (Start Watchdog, Open Build Log)
//   - Osiris checkpoint warnings
//
// Architecture:
//
//	┌─────────────┐     ┌──────────┐     ┌───────────┐
//	│ systray.Run  │────▶│  Stats    │────▶│  Menu     │
//	│ (event loop) │     │ Collector │     │  Builder  │
//	└─────────────┘     └──────────┘     └───────────┘
//	       │                                     │
//	       └───── onReady() ─── click ──── Handlers
//
// Build: go build -o bin/pantheon-menubar ./cmd/pantheon-menubar/
// Bundle: make bundle (creates Pantheon.app)
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "v0.4.0-alpha"

func main() {
	fmt.Println("𓂀 Pantheon Menu Bar v" + version)
	fmt.Println("  Starting menu bar application...")
	fmt.Println()

	// Check if we should run in headless mode (for testing / CI)
	if os.Getenv("PANTHEON_HEADLESS") == "1" {
		fmt.Println("  Running in headless mode (no systray)")
		runHeadless()
		return
	}

	// The systray integration requires CGo and platform-specific code.
	// For the initial build, we use a polling-based approach that works
	// without systray as a foundation, then layer systray on top.
	//
	// To enable systray:
	//   1. Add `fyne.io/systray` to go.mod
	//   2. Uncomment the systray.Run block below
	//   3. Build with CGo enabled
	//
	// For now, we print stats to stdout and accept signal-based commands.

	// systray.Run(onReady, onExit)  // Uncomment when systray dep added

	// Headless fallback: poll and print stats
	runHeadless()
}

// runHeadless runs the stats collector in a terminal-friendly mode.
// This is useful for testing and for systems without systray support.
func runHeadless() {
	cfg := DefaultStatsConfig()

	// Find repo root
	root, err := findRepoRoot()
	if err == nil {
		cfg.RepoDir = root
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Initial collection
	printStats(cfg)

	for {
		select {
		case <-ticker.C:
			printStats(cfg)
		case sig := <-sigCh:
			fmt.Printf("\n𓂀 Pantheon shutting down (signal: %s)\n", sig)
			return
		}
	}
}

func printStats(cfg StatsConfig) {
	snap := CollectStats(cfg)

	fmt.Println("─── 𓂀 Pantheon Status ───────────────────")
	for _, item := range snap.FormatMenuItems() {
		fmt.Printf("  %s\n", item)
	}
	fmt.Printf("  %s\n", snap.StatusLine())
	fmt.Println("──────────────────────────────────────────")
	fmt.Println()
}

// ── systray callbacks (uncomment when dep added) ────────────────────────

/*
func onReady() {
	systray.SetIcon(getIcon())
	systray.SetTitle("")
	systray.SetTooltip("𓂀 Pantheon — Active")

	// Stats section (non-clickable info items)
	cfg := DefaultStatsConfig()
	root, _ := findRepoRoot()
	if root != "" {
		cfg.RepoDir = root
	}

	snap := CollectStats(cfg)
	menuItems := snap.FormatMenuItems()
	for _, item := range menuItems {
		systray.AddMenuItem(item, "").Disable()
	}

	systray.AddSeparator()

	// Command section
	for _, h := range PantheonHandlers() {
		handler := h // capture
		item := systray.AddMenuItem(handler.Name, "Run "+handler.Name)
		go func() {
			for range item.ClickedCh {
				_ = handler.Execute()
			}
		}()
	}

	systray.AddSeparator()

	// Quick actions
	mBuildLog := systray.AddMenuItem("📄 Open Build Log", "")
	mCaseStudies := systray.AddMenuItem("📚 Open Case Studies", "")
	systray.AddSeparator()

	// Status line
	mStatus := systray.AddMenuItem(snap.StatusLine(), "")
	mStatus.Disable()

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Pantheon", "Quit")

	// Background stats refresh
	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		for range ticker.C {
			snap := CollectStats(cfg)
			mStatus.SetTitle(snap.StatusLine())
			// Note: systray doesn't support updating existing items easily
			// Future: rebuild menu on each tick
		}
	}()

	// Handle clicks
	go func() {
		for {
			select {
			case <-mBuildLog.ClickedCh:
				_ = OpenBuildLog()
			case <-mCaseStudies.ClickedCh:
				_ = OpenCaseStudies()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	fmt.Println("𓂀 Pantheon menu bar exited")
}
*/
