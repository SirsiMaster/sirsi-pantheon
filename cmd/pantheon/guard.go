package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	guardSlayTarget   string
	guardReniceTarget string
	guardDryRun       bool
	guardConfirm      bool
	guardWatch        bool
	guardThreshold    float64
)

var guardCmd = &cobra.Command{
	Use:   "guard",
	Short: "🛡️ Manage RAM pressure — audit processes, slay orphans",
	Long: `🛡️ Guard — The RAM Guardian

Audit running processes and identify memory-hungry orphans.
Slay zombie Node processes, stale LSP servers, and runaway builds.

  pantheon guard                    Audit RAM usage and show process groups
  pantheon guard --slay node        Kill orphaned Node.js processes
  pantheon guard --slay lsp         Kill stale language servers
  pantheon guard --slay all         Kill all known orphan types
  pantheon guard --renice lsp       Deprioritize LSPs for click latency fix

Safety: --dry-run is the default. Use --confirm to actually kill processes.
        SIGTERM is sent first; SIGKILL only after 5s grace period.
        System processes (kernel_task, WindowServer, launchd) are NEVER killed.
        --renice does NOT kill processes — it lowers their scheduler priority.`,
	Run: runGuard,
}

func init() {
	guardCmd.Flags().StringVar(&guardSlayTarget, "slay", "", "Target group to kill (node, lsp, docker, electron, build, ai, all)")
	guardCmd.Flags().StringVar(&guardReniceTarget, "renice", "", "Deprioritize background processes (lsp, all)")
	guardCmd.Flags().BoolVar(&guardDryRun, "dry-run", false, "Show what would be killed without actually killing")
	guardCmd.Flags().BoolVar(&guardConfirm, "confirm", false, "Actually kill processes (required for slay)")
	guardCmd.Flags().BoolVar(&guardWatch, "watch", false, "Sekhmet watchdog mode — monitor CPU pressure continuously")
	guardCmd.Flags().Float64Var(&guardThreshold, "threshold", 80.0, "CPU threshold for watchdog alerts (default: 80%%)")
}

func runGuard(cmd *cobra.Command, args []string) {
	// Renice mode
	if guardReniceTarget != "" {
		runRenice()
		return
	}

	// Watch mode (Sekhmet watchdog)
	if guardWatch {
		runWatchdog()
		return
	}

	// Run audit
	result, err := guard.Audit()
	if err != nil {
		output.Error("Guard audit failed: %v", err)
		os.Exit(1)
	}

	// If --slay is specified, handle that
	if guardSlayTarget != "" {
		if !guard.IsValidTarget(guardSlayTarget) {
			output.Error("Invalid slay target: %q", guardSlayTarget)
			output.Warn("Valid targets: %s", strings.Join(slayTargetStrings(), ", "))
			os.Exit(1)
		}

		if !guardDryRun && !guardConfirm {
			output.Error("Slay requires --dry-run or --confirm flag")
			output.Warn("Try: pantheon guard --slay " + guardSlayTarget + " --dry-run")
			os.Exit(1)
		}

		isDryRun := guardDryRun || !guardConfirm
		slayResult, err := guard.Slay(guard.SlayTarget(guardSlayTarget), isDryRun)
		if err != nil {
			output.Error("Slay failed: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(slayResult)
			return
		}

		renderSlayResult(slayResult)
		return
	}

	// Default: show audit results
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	renderAuditResult(result)
}

func renderAuditResult(result *guard.AuditResult) {
	output.Header("🛡️ Guard — RAM Audit")
	fmt.Println()

	// Memory overview
	output.Info("Total RAM:  %s", guard.FormatBytes(result.TotalRAM))
	output.Info("Used:       %s", guard.FormatBytes(result.UsedRAM))
	output.Info("Free:       %s", guard.FormatBytes(result.FreeRAM))

	usedPercent := float64(result.UsedRAM) / float64(result.TotalRAM) * 100
	if usedPercent > 85 {
		output.Warn("⚠️  RAM pressure: %.0f%% used — consider slaying orphans", usedPercent)
	}
	fmt.Println()

	// Process groups
	output.Header("Process Groups (by RAM usage)")
	fmt.Println()

	for _, g := range result.Groups {
		if g.TotalRSS < 10*1024*1024 { // Skip groups < 10 MB
			continue
		}
		label := fmt.Sprintf("  %-14s  %3d processes  %s", g.Name, g.TotalCount, guard.FormatBytes(g.TotalRSS))
		if g.TotalRSS > 1024*1024*1024 { // > 1 GB
			output.Warn(label)
		} else {
			output.Info(label)
		}
	}
	fmt.Println()

	// Orphans summary
	if result.TotalOrphans > 0 {
		output.Warn("🔍 Found %d potential orphan processes using %s",
			result.TotalOrphans, guard.FormatBytes(result.OrphanRSS))

		// Show top 10 orphans
		limit := 10
		if len(result.Orphans) < limit {
			limit = len(result.Orphans)
		}
		fmt.Println()
		for _, o := range result.Orphans[:limit] {
			shortName := o.Name
			if len(shortName) > 30 {
				shortName = shortName[:27] + "..."
			}
			fmt.Printf("    PID %-6d  %-30s  %s  [%s]\n",
				o.PID, shortName, jackal.FormatSize(o.RSS), o.Group)
		}
		if result.TotalOrphans > limit {
			fmt.Printf("    ... and %d more\n", result.TotalOrphans-limit)
		}
		fmt.Println()
		output.Info("Run: pantheon guard --slay <target> --dry-run")
	} else {
		output.Info("✅ No significant orphan processes detected")
	}

	fmt.Println()
}

func renderSlayResult(result *guard.SlayResult) {
	if result.DryRun {
		output.Header("🛡️ Guard — Slay [DRY RUN]")
	} else {
		output.Header("🛡️ Guard — Slay")
	}
	fmt.Println()

	output.Info("Target:    %s", result.Target)
	output.Info("Killed:    %d processes", result.Killed)
	if result.Skipped > 0 {
		output.Warn("Skipped:   %d (protected system processes)", result.Skipped)
	}
	if result.Failed > 0 {
		output.Error("Failed:    %d", result.Failed)
		for _, err := range result.Errors {
			output.Error("  → %v", err)
		}
	}
	output.Info("RAM freed: %s", guard.FormatBytes(result.BytesFreed))

	if result.DryRun {
		fmt.Println()
		output.Warn("This was a dry run. To actually kill processes:")
		output.Info("  pantheon guard --slay %s --confirm", result.Target)
	}
	fmt.Println()
}

func runRenice() {
	target := guard.ReniceTarget(guardReniceTarget)
	if target != guard.ReniceTargetLSP && target != guard.ReniceTargetAll {
		output.Error("Invalid renice target: %q", guardReniceTarget)
		output.Warn("Valid targets: lsp, all")
		os.Exit(1)
	}

	result, err := guard.Renice(target)
	if err != nil {
		output.Error("Renice failed: %v", err)
		os.Exit(1)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	output.Header("𓁵 Sekhmet — Renice")
	fmt.Println()
	fmt.Print(guard.FormatReniceReport(result))
	fmt.Println()
}

func slayTargetStrings() []string {
	targets := guard.ValidSlayTargets()
	strs := make([]string, len(targets))
	for i, t := range targets {
		strs[i] = string(t)
	}
	return strs
}

// runWatchdog starts the Sekhmet watchdog mode with the Antigravity IPC bridge.
// The bridge connects the watchdog to MCP consumers via a thread-safe AlertRing,
// so any running MCP server can query live alerts via anubis://watchdog-alerts.
func runWatchdog() {
	numCPU := runtime.NumCPU()

	output.Header("𓁵 Sekhmet — Watchdog Mode (Antigravity Bridge)")
	fmt.Println()
	output.Info("CPU threshold:  %.0f%%", guardThreshold)
	output.Info("Cores detected: %d", numCPU)
	output.Info("Polling:        every 800ms, sustain-count=1")
	output.Info("Architecture:   Antigravity IPC bridge + AlertRing buffer")
	output.Info("MCP resource:   anubis://watchdog-alerts (live)")
	output.Info("Press Ctrl+C to stop.")
	fmt.Println()

	// Configure the bridge (not just the raw watchdog)
	cfg := guard.DefaultBridgeConfig()
	cfg.WatchConfig.CPUThreshold = guardThreshold
	cfg.OnAlert = func(entry guard.AlertEntry) {
		// Print alerts to stderr in real-time
		severity := "⚠️"
		if entry.Severity == "critical" {
			severity = "🔴"
		}
		fmt.Fprintf(os.Stderr, "  %s  [%s] PID %-6d %-20s  CPU: %.0f%%  RAM: %s  (%s)\n",
			severity, entry.Severity, entry.PID, entry.ProcessName,
			entry.CPUPercent, entry.RSSHuman, entry.Duration)

		// Give actionable advice
		group := classifyForAdvice(entry.ProcessName)
		if group != "" {
			output.Warn("  → Fix: pantheon guard --slay %s --dry-run", group)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the full bridge (watchdog + ring buffer + MCP integration)
	bridge := guard.StartBridge(ctx, cfg)

	// Register with MCP so the watchdog-alerts resource serves live data
	mcp.SetWatchdogBridge(bridge)
	output.Success("Antigravity bridge active — MCP consumers can query alerts")
	fmt.Println()

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	fmt.Println()
	output.Info("𓁵 Sekhmet standing down.")

	// Report final stats from the bridge
	status := bridge.Status()
	fmt.Println()
	output.Info("Buffered alerts:  %d", status.BufferedCount)
	output.Info("Lifetime alerts:  %d", status.LifetimeAlerts)
	output.Info("Watchdog polls:   %d", status.WatchdogPolls)
	output.Info("Backoffs:         %d", status.WatchdogBackoffs)

	// Clean shutdown
	bridge.Stop()
	mcp.SetWatchdogBridge(nil)
}

// classifyForAdvice maps process names to slay targets for actionable suggestions.
func classifyForAdvice(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "node") || strings.Contains(name, "npm"):
		return "node"
	case strings.Contains(name, "gopls") || strings.Contains(name, "language"):
		return "lsp"
	case strings.Contains(name, "docker"):
		return "docker"
	case strings.Contains(name, "electron") || strings.Contains(name, "plugin host") || strings.Contains(name, "helper"):
		return "electron"
	case strings.Contains(name, "cargo") || strings.Contains(name, "gradle") || strings.Contains(name, "webpack"):
		return "build"
	case strings.Contains(name, "ollama") || strings.Contains(name, "mlx"):
		return "ai"
	default:
		return ""
	}
}
