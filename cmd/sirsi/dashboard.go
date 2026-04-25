package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var dashboardPort int
var dashboardNoBrowser bool

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "𓂀 Launch the Horus workstation monitor in your browser",
	Long: `𓂀 Horus — Local Workstation Monitor

Starts a local HTTP server and opens the dashboard in your default browser.
All data stays on your machine — zero telemetry (Rule A11).

  sirsi dashboard              Open dashboard at localhost:9119
  sirsi dashboard --port 8080  Use custom port
  sirsi dashboard --no-open    Start server without opening browser`,
	Run: runDashboard,
}

func init() {
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", dashboard.DashboardPort, "Dashboard server port")
	dashboardCmd.Flags().BoolVar(&dashboardNoBrowser, "no-open", false, "Don't open browser automatically")
}

func runDashboard(cmd *cobra.Command, args []string) {
	output.Header("𓂀 Horus — Local Workstation Monitor")

	nStore, err := notify.Open(notify.DefaultPath())
	if err != nil {
		output.Warn("Notification store unavailable: %v", err)
	}

	// Find our own binary path for the command runner.
	selfBin, _ := os.Executable()

	srv := dashboard.New(dashboard.Config{
		Port:     dashboardPort,
		NotifyDB: nStore,
		Events:   dashboard.NewEventBuffer(256),
		SirsiBin: selfBin,
		StatsFn: func() ([]byte, error) {
			snap := collectDashboardStats()
			return json.Marshal(snap)
		},
	})

	if err := srv.Start(); err != nil {
		output.Error("Failed to start dashboard: %v", err)
		os.Exit(1)
	}

	output.Success("Dashboard running at %s", srv.URL())
	output.Info("Press Ctrl+C to stop")

	if !dashboardNoBrowser {
		if err := srv.OpenPage("/"); err != nil {
			output.Warn("Could not open browser: %v", err)
			output.Info("Open manually: %s", srv.URL())
		}
	}

	// Block until signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	output.Info("Shutting down dashboard...")
	_ = srv.Stop()
	if nStore != nil {
		nStore.Close()
	}
}

// collectDashboardStats gathers system metrics for the dashboard.
// This is a lightweight version of the menubar's CollectStats — reuses
// the same system calls without importing the menubar package.
func collectDashboardStats() map[string]interface{} {
	stats := map[string]interface{}{
		"ram_percent":         0.0,
		"ram_pressure":        "unknown",
		"ram_icon":            "⚪",
		"uncommitted_files":   0,
		"git_branch":          "—",
		"time_since_commit":   "",
		"osiris_risk":         "unknown",
		"osiris_icon":         "⚪",
		"primary_accelerator": "Unknown",
		"accel_icon":          "💻",
		"active_deities":      []string{},
		"deity_count":         0,
		"ra_deployed":         false,
		"ra_scopes":           []interface{}{},
		"ra_icon":             "⚫",
	}

	collectDashRAM(stats)
	collectDashGit(stats)
	collectDashAccelerator(stats)
	collectDashDeities(stats)

	return stats
}
