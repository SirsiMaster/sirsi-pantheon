package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mercury"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	buildversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
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
	fabricContext, cancelFabric := context.WithCancel(cmd.Context())
	defer cancelFabric()
	fabricSource := ledger.NewFabricGenerationProducerContext(fabricContext, collectDashboardFabricReadContext, buildversion.Current("sirsi").Version)

	srv := dashboard.New(dashboard.Config{
		MercuryFn: mercury.NewConfigured().Read,
		Port:      dashboardPort,
		NotifyDB:  nStore,
		Events:    dashboard.NewEventBuffer(256),
		SirsiBin:  selfBin,
		StatsFn: func() ([]byte, error) {
			snap := collectDashboardStats()
			return json.Marshal(snap)
		},
		VitalsFn:     collectDashboardVitals,
		NodeStatusFn: collectDashboardNodeStatus,
		LedgerFn:     collectDashboardLedger,
		FleetFn:      collectDashboardFleet,
		RouterFn:     collectDashboardRouter,
		RoutabilityFn: func() (map[string]bool, error) {
			repoRoot, err := router.FindRepoRoot()
			if err != nil {
				return nil, err
			}
			return unroutableAgents(repoRoot)
		},
		FabricReadFn:        collectDashboardFabricRead,
		FabricSource:        fabricSource,
		FabricBuild:         buildversion.Current("sirsi").Version,
		MaatDecisionsFn:     collectDashboardMaatDecisions,
		MaatCasebookFn:      collectDashboardMaatCasebook,
		MaatKnowledgeFn:     collectDashboardMaatKnowledge,
		MaatHistoryFn:       collectDashboardMaatHistory,
		MaatKnowledgeReadFn: collectDashboardMaatKnowledgeRead,
		ApolloTelemetryFn: func() (apollo.TelemetryRead, error) {
			home, err := os.UserHomeDir()
			if err != nil {
				return apollo.TelemetryRead{}, err
			}
			return apollo.ReadTelemetry(home)
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
	cancelFabric()
	_ = srv.Stop()
	if nStore != nil {
		nStore.Close()
	}
}

// Explicit browser diagnostics use the exact ranked CLI producer. Periodic
// dashboard stats do not call this; only GET /api/vitals does.
func collectDashboardVitals() ([]byte, error) {
	report, err := sampleVitalsReport(false, 5)
	if err != nil {
		return nil, err
	}
	return json.Marshal(report)
}

func collectDashboardMaatDecisions(limit int) ([]maat.Decision, error) {
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		return nil, err
	}
	return journal.Recent(limit)
}

func collectDashboardMaatCasebook(query casebook.Query) (casebook.View, error) {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		return casebook.View{}, err
	}
	result, err := casebook.SearchHistory(journal, casebook.HistoryQuery{Query: query, Context: context.Background()})
	return result.View, err
}

func collectDashboardMaatHistory(ctx context.Context, query casebook.HistoryQuery) (casebook.HistorySearchResult, error) {
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		return casebook.HistorySearchResult{}, err
	}
	query.Context = ctx
	return casebook.SearchHistory(journal, query)
}

func collectDashboardMaatKnowledgeRead(ctx context.Context, request dashboard.MaatKnowledgeReadRequest) (any, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("Ma'at knowledge home unavailable")
	}
	if request.Mode == "search" {
		return knowledge.LoadPageContext(ctx, home, request.Query)
	}
	archive, err := knowledge.Open(home)
	if err != nil {
		return nil, err
	}
	defer func() { _ = archive.Close() }()
	if request.Mode == "detail" {
		return archive.Detail(ctx, request.ID, request.Revision)
	}
	return archive.HistoryPage(ctx, request.ID, request.Query.Limit, request.Query.Cursor)
}

func collectDashboardMaatKnowledge(query string) (knowledge.View, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return knowledge.View{}, fmt.Errorf("locate Ma'at knowledge home: %w", err)
	}
	return knowledge.Load(home, query)
}

func collectDashboardFabricRead() (ledger.FabricRead, error) {
	return collectDashboardFabricReadContext(context.Background())
}
func collectDashboardFabricReadContext(ctx context.Context) (ledger.FabricRead, error) {
	if err := ctx.Err(); err != nil {
		return ledger.FabricRead{}, err
	}
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return ledger.FabricRead{}, fmt.Errorf("locate repo root: %w", err)
	}
	return collectDashboardFabricReadAtRootContext(ctx, repoRoot)
}

func collectDashboardFabricReadAtRoot(repoRoot string) (ledger.FabricRead, error) {
	return collectDashboardFabricReadAtRootContext(context.Background(), repoRoot)
}
func collectDashboardFabricReadAtRootContext(ctx context.Context, repoRoot string) (ledger.FabricRead, error) {
	now := time.Now().UTC()
	read, err := ledger.ReadFabricContext(ctx, repoRoot, now)
	if err != nil {
		return ledger.FabricRead{}, err
	}
	health := collectFabricHostHealthContext(ctx, repoRoot, now)
	if err := ctx.Err(); err != nil {
		return ledger.FabricRead{}, err
	}
	read.Evidence.Health = &health
	return read, nil
}
func collectDashboardFabric() (ledger.FabricBoard, error) {
	producer := ledger.NewFabricGenerationProducer(collectDashboardFabricRead, buildversion.Current("sirsi").Version)
	generation, err := producer.Read(context.Background())
	if err != nil {
		return ledger.FabricBoard{}, err
	}
	view, err := generation.View()
	return view.Board, err
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
	snap, err := ledger.Build(repoRoot, "", time.Now().UTC(), ledger.DefaultStaleAfter)
	if err != nil {
		return snap, err
	}
	// Delivery aliases redirect new mail, not historical task ownership. Keep
	// every ledger row, including alias-owned obligations, exactly as the CLI
	// does. Hiding or folding rows loses counts and can collide task identities.
	return snap, nil
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
	// Web observation must not launch agent auth/model commands. Explicit CLI
	// diagnostics retain the collector's default active auth probe.
	return router.CollectNodeStatus(repoRoot, nil, func(string, string) (bool, bool, string) {
		return false, false, "not probed during passive dashboard read; use explicit auth diagnostics"
	})
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
