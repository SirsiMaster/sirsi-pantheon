package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
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
	output.Header("Workstation Monitor")

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
		NodeStatusFn: collectDashboardNodeStatus,
		LedgerFn:     collectDashboardLedger,
		FleetFn:      collectDashboardFleet,
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

// collectDashboardLedger wires GET /api/ledger (A26 Nexus seam) to the
// same ledger.Build + Summarize pipeline that `sirsi router ledger` uses.
// Repo root is resolved per request so the endpoint stays correct across
// repo appears/disappears events.
func collectDashboardLedger() (ledger.BoardSummary, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return ledger.BoardSummary{}, fmt.Errorf("locate repo root: %w", err)
	}
	snap, err := ledger.Build(repoRoot, "", time.Now().UTC(), ledger.DefaultStaleAfter)
	if err != nil {
		return ledger.BoardSummary{}, fmt.Errorf("build ledger: %w", err)
	}
	return ledger.Summarize(snap), nil
}

// collectDashboardFleet wires GET /api/fleet (A32 owner-reporting board) to
// the SAME ledger.Build the CLI uses — one read model. The retired Python
// board learned this the hard way: it iterated agents.json and made a task
// call per agent, so any agent present in the store but absent from that file
// was invisible. It read 196 tasks while the router read 205. The snapshot
// already embeds every agent's tasks; use it and nothing else.
//
// Returns the RAW snapshot, not a summary: the fleet board diffs consecutive
// snapshots to derive the activity feed, so it needs per-task status.
func collectDashboardFleet() (ledger.Snapshot, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return ledger.Snapshot{}, fmt.Errorf("locate repo root: %w", err)
	}
	return ledger.Build(repoRoot, "", time.Now().UTC(), ledger.DefaultStaleAfter)
}

// collectDashboardNodeStatus wires GET /api/node-status (ADR-026) to the
// SAME collector `sirsi router node-status` uses — one read-model, no
// re-aggregation. Repo root is resolved per request so a dashboard started
// before/after a repo appears stays correct; if no router repo is reachable
// the error propagates and the endpoint degrades to an honest 5xx instead of
// serving a fabricated NodeStatus.
func collectDashboardNodeStatus() (*router.NodeStatus, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("locate repo root: %w", err)
	}
	return router.CollectNodeStatus(repoRoot, nil)
}

// collectDashboardStats gathers system metrics for the dashboard.
// This is a lightweight version of the menubar's CollectStats — reuses
// the same system calls without importing the menubar package.
func collectDashboardStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_ram":           int64(0),
		"used_ram":            int64(0),
		"free_ram":            int64(0),
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
