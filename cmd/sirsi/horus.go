package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/horus"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var (
	horusKind              string
	horusFilter            string
	horusSuperviseOnce     bool
	horusSuperviseInterval time.Duration
	horusSuperviseAgentID  string
	horusSuperviseThreadID string
)

var horusCmd = &cobra.Command{
	Use:   "horus",
	Short: "Structural code graph — symbols, outlines, context (subsumes Code Review Graph)",
	Long: `𓂀 Horus — Structural Code Graph

Extracts symbols (types, functions, methods, interfaces) from Go source
and serves compact outlines and signatures instead of full files.
8-49x smaller than reading full source.

  sirsi horus scan .                   Build symbol graph
  sirsi horus outline internal/mcp/tools.go   File outline (no bodies)
  sirsi horus symbols --kind=func      List all functions
  sirsi horus context NewServer        Show symbol context
  sirsi horus supervise --once         Check local router wake surfaces`,
}

var horusScanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Build symbol graph for a project",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHorusScan,
}

var horusOutlineCmd = &cobra.Command{
	Use:   "outline <file>",
	Short: "Print compact file outline (declarations only, no bodies)",
	Args:  cobra.ExactArgs(1),
	RunE:  runHorusOutline,
}

var horusSymbolsCmd = &cobra.Command{
	Use:   "symbols",
	Short: "List symbols matching filters",
	RunE:  runHorusSymbols,
}

var horusContextCmd = &cobra.Command{
	Use:   "context <symbol>",
	Short: "Show minimal context for a symbol",
	Args:  cobra.ExactArgs(1),
	RunE:  runHorusContext,
}

var horusStatsCmd = &cobra.Command{
	Use:   "stats [path]",
	Short: "Print graph statistics",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHorusStats,
}

var horusSuperviseCmd = &cobra.Command{
	Use:   "supervise",
	Short: "Run the Horus resident router supervisor loop",
	Long: `Runs the local Ra/Horus router supervisor in the foreground.

The supervisor inventories agents.json, refreshes its resident CTR thread,
heartbeats every interval, pulls local agent inboxes, and reports each surface
as wakeable, stale, blocked, or manual. It does not dispatch or merge work.

  sirsi horus supervise --once
  sirsi horus supervise --interval 60s`,
	RunE: runHorusSupervise,
}

func init() {
	horusSymbolsCmd.Flags().StringVar(&horusKind, "kind", "", "Filter by kind: type, func, method, interface, struct, const, var")
	horusSymbolsCmd.Flags().StringVar(&horusFilter, "filter", "", "Filter by name pattern (glob with *)")
	horusSuperviseCmd.Flags().BoolVar(&horusSuperviseOnce, "once", false, "Run one supervisor pass and exit")
	horusSuperviseCmd.Flags().DurationVar(&horusSuperviseInterval, "interval", 60*time.Second, "Heartbeat/pull interval for resident mode")
	horusSuperviseCmd.Flags().StringVar(&horusSuperviseAgentID, "agent-id", router.SupervisorAgentID, "CTR agent id for the supervisor thread")
	horusSuperviseCmd.Flags().StringVar(&horusSuperviseThreadID, "thread", "", "Reuse a specific supervisor thread id")

	horusCmd.AddCommand(horusScanCmd, horusOutlineCmd, horusSymbolsCmd, horusContextCmd, horusStatsCmd, horusSuperviseCmd)
}

func horusParseDir(path string) (*horus.SymbolGraph, error) {
	if path == "" || path == "." {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("determine working directory: %w", err)
		}
	}

	p := horus.NewGoParser()
	return p.ParseDir(path)
}

func runHorusScan(_ *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	output.Banner()
	fmt.Printf("𓂀 Horus scanning %s...\n\n", path)

	graph, err := horusParseDir(path)
	if err != nil {
		return err
	}

	fmt.Printf("Symbol Graph Built\n")
	fmt.Printf("  Files:      %d\n", graph.Stats.Files)
	fmt.Printf("  Packages:   %d\n", graph.Stats.Packages)
	fmt.Printf("  Types:      %d\n", graph.Stats.Types)
	fmt.Printf("  Interfaces: %d\n", graph.Stats.Interfaces)
	fmt.Printf("  Functions:  %d\n", graph.Stats.Functions)
	fmt.Printf("  Methods:    %d\n", graph.Stats.Methods)
	fmt.Printf("  Lines:      %d\n", graph.Stats.TotalLines)
	fmt.Printf("  Symbols:    %d total\n", len(graph.Symbols))
	return nil
}

func runHorusOutline(_ *cobra.Command, args []string) error {
	path := args[0]
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	p := horus.NewGoParser()
	symbols, err := p.ParseFile(path, src)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	graph := &horus.SymbolGraph{Root: ".", Symbols: symbols}
	q := horus.NewQuery(graph)
	fmt.Println(q.FileOutline(path))

	ratio := float64(len(q.FileOutline(path))) / float64(len(src)) * 100
	fmt.Printf("\n── %.0f%% of original (%d → %d bytes) ──\n", ratio, len(src), len(q.FileOutline(path)))
	return nil
}

func runHorusSymbols(_ *cobra.Command, _ []string) error {
	graph, err := horusParseDir(".")
	if err != nil {
		return err
	}

	q := horus.NewQuery(graph)
	symbols := graph.Symbols

	if horusKind != "" {
		symbols = q.ByKind(horus.SymbolKind(horusKind))
	}
	if horusFilter != "" {
		symbols = q.MatchSymbols(horusFilter)
	}

	for _, s := range symbols {
		if s.Kind == horus.KindPackage {
			continue
		}
		prefix := " "
		if s.Exported {
			prefix = "▸"
		}
		sig := s.Signature
		if sig == "" {
			sig = fmt.Sprintf("%s %s", s.Kind, s.Name)
		}
		fmt.Printf("%s %s:%d  %s\n", prefix, s.File, s.Line, sig)
	}
	return nil
}

func runHorusContext(_ *cobra.Command, args []string) error {
	graph, err := horusParseDir(".")
	if err != nil {
		return err
	}

	q := horus.NewQuery(graph)
	fmt.Println(q.ContextFor(args[0]))
	return nil
}

func runHorusStats(_ *cobra.Command, args []string) error {
	start := time.Now()
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	graph, err := horusParseDir(path)
	if err != nil {
		return err
	}

	if JsonOutput {
		result := map[string]interface{}{
			"files":      graph.Stats.Files,
			"packages":   graph.Stats.Packages,
			"types":      graph.Stats.Types,
			"interfaces": graph.Stats.Interfaces,
			"functions":  graph.Stats.Functions,
			"methods":    graph.Stats.Methods,
			"lines":      graph.Stats.TotalLines,
			"symbols":    len(graph.Symbols),
			"elapsed_ms": time.Since(start).Milliseconds(),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	output.Banner()
	output.Header("Code Graph Statistics")
	output.Dashboard(map[string]string{
		"Files":     fmt.Sprintf("%d", graph.Stats.Files),
		"Functions": fmt.Sprintf("%d", graph.Stats.Functions),
		"Lines":     fmt.Sprintf("%d", graph.Stats.TotalLines),
	})
	output.Info("  Packages:   %d (%v)", graph.Stats.Packages, graph.Packages)
	output.Info("  Types:      %d", graph.Stats.Types)
	output.Info("  Interfaces: %d", graph.Stats.Interfaces)
	output.Info("  Methods:    %d", graph.Stats.Methods)
	output.Footer(time.Since(start))
	output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "horus", Subcommand: "stats"}))
	return nil
}

func runHorusSupervise(cmd *cobra.Command, _ []string) error {
	if horusSuperviseInterval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("no idea-router found: %w", err)
	}
	opts := router.SuperviseOptions{
		RepoRoot: repoRoot,
		AgentID:  horusSuperviseAgentID,
		ThreadID: horusSuperviseThreadID,
	}
	if horusSuperviseOnce {
		report, err := router.SuperviseOnce(opts)
		if err != nil {
			return err
		}
		return printSuperviseReport(report)
	}

	fmt.Printf("𓂀 Horus supervisor running (interval=%s, repo=%s)\n", horusSuperviseInterval, repoRoot)
	fmt.Println("  Press Ctrl-C to stop. LaunchAgent label: ai.sirsi.horus.agent-router")
	fmt.Println()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	ticker := time.NewTicker(horusSuperviseInterval)
	defer ticker.Stop()

	for {
		report, err := router.SuperviseOnce(opts)
		if err != nil {
			if JsonOutput {
				return err
			}
			fmt.Fprintf(os.Stderr, "supervise: %v\n", err)
		} else {
			opts.ThreadID = report.ThreadID
			if err := printSuperviseReport(report); err != nil {
				return err
			}
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-sig:
			fmt.Println("Horus supervisor stopped.")
			return nil
		case <-ticker.C:
		}
	}
}

func printSuperviseReport(report *router.SuperviseReport) error {
	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Printf("[%s] supervisor=%s status=%s pending=%d live=%d stale=%d\n",
		time.Now().Format(time.RFC3339),
		report.ThreadID,
		report.Status,
		report.PendingTotal,
		report.LiveThreadCount,
		report.StaleThreadCount,
	)
	for _, agent := range report.Agents {
		if agent.PendingCount == 0 && agent.Status == router.SupervisorStatusWakeable {
			continue
		}
		fmt.Printf("  • %-24s %-8s pending=%d", agent.AgentID, agent.Status, agent.PendingCount)
		if len(agent.StaleThreads) > 0 {
			fmt.Printf(" stale=%v", agent.StaleThreads)
		}
		if agent.Detail != "" && agent.Status != router.SupervisorStatusWakeable {
			fmt.Printf(" — %s", agent.Detail)
		}
		fmt.Println()
		for _, id := range agent.PendingItems {
			fmt.Printf("      inbox: %s\n", id)
		}
	}
	return nil
}
