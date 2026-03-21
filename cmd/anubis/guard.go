package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/guard"
	"github.com/SirsiMaster/sirsi-anubis/internal/jackal"
	"github.com/SirsiMaster/sirsi-anubis/internal/output"
)

var (
	guardSlayTarget string
	guardDryRun     bool
	guardConfirm    bool
)

var guardCmd = &cobra.Command{
	Use:   "guard",
	Short: "🛡️ Manage RAM pressure — audit processes, slay orphans",
	Long: `🛡️ Guard — The RAM Guardian

Audit running processes and identify memory-hungry orphans.
Slay zombie Node processes, stale LSP servers, and runaway builds.

  anubis guard                    Audit RAM usage and show process groups
  anubis guard --slay node        Kill orphaned Node.js processes
  anubis guard --slay lsp         Kill stale language servers
  anubis guard --slay docker      Kill Docker Desktop helpers
  anubis guard --slay electron    Kill Electron helper renderers
  anubis guard --slay build       Kill stale build processes
  anubis guard --slay ai          Kill orphaned AI/ML processes
  anubis guard --slay all         Kill all known orphan types

Safety: --dry-run is the default. Use --confirm to actually kill processes.
        SIGTERM is sent first; SIGKILL only after 5s grace period.
        System processes (kernel_task, WindowServer, launchd) are NEVER killed.`,
	Run: runGuard,
}

func init() {
	guardCmd.Flags().StringVar(&guardSlayTarget, "slay", "", "Target group to kill (node, lsp, docker, electron, build, ai, all)")
	guardCmd.Flags().BoolVar(&guardDryRun, "dry-run", false, "Show what would be killed without actually killing")
	guardCmd.Flags().BoolVar(&guardConfirm, "confirm", false, "Actually kill processes (required for slay)")
}

func runGuard(cmd *cobra.Command, args []string) {
	// Run audit
	result, err := guard.Audit()
	if err != nil {
		output.Error(fmt.Sprintf("Guard audit failed: %v", err))
		os.Exit(1)
	}

	// If --slay is specified, handle that
	if guardSlayTarget != "" {
		if !guard.IsValidTarget(guardSlayTarget) {
			output.Error(fmt.Sprintf("Invalid slay target: %q", guardSlayTarget))
			output.Warn(fmt.Sprintf("Valid targets: %s", strings.Join(slayTargetStrings(), ", ")))
			os.Exit(1)
		}

		if !guardDryRun && !guardConfirm {
			output.Error("Slay requires --dry-run or --confirm flag")
			output.Warn("Try: anubis guard --slay " + guardSlayTarget + " --dry-run")
			os.Exit(1)
		}

		isDryRun := guardDryRun || !guardConfirm
		slayResult, err := guard.Slay(guard.SlayTarget(guardSlayTarget), isDryRun)
		if err != nil {
			output.Error(fmt.Sprintf("Slay failed: %v", err))
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
	output.Info(fmt.Sprintf("Total RAM:  %s", guard.FormatBytes(result.TotalRAM)))
	output.Info(fmt.Sprintf("Used:       %s", guard.FormatBytes(result.UsedRAM)))
	output.Info(fmt.Sprintf("Free:       %s", guard.FormatBytes(result.FreeRAM)))

	usedPercent := float64(result.UsedRAM) / float64(result.TotalRAM) * 100
	if usedPercent > 85 {
		output.Warn(fmt.Sprintf("⚠️  RAM pressure: %.0f%% used — consider slaying orphans", usedPercent))
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
		output.Warn(fmt.Sprintf("🔍 Found %d potential orphan processes using %s",
			result.TotalOrphans, guard.FormatBytes(result.OrphanRSS)))

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
		output.Info("Run: anubis guard --slay <target> --dry-run")
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

	output.Info(fmt.Sprintf("Target:    %s", result.Target))
	output.Info(fmt.Sprintf("Killed:    %d processes", result.Killed))
	if result.Skipped > 0 {
		output.Warn(fmt.Sprintf("Skipped:   %d (protected system processes)", result.Skipped))
	}
	if result.Failed > 0 {
		output.Error(fmt.Sprintf("Failed:    %d", result.Failed))
		for _, err := range result.Errors {
			output.Error(fmt.Sprintf("  → %v", err))
		}
	}
	output.Info(fmt.Sprintf("RAM freed: %s", guard.FormatBytes(result.BytesFreed)))

	if result.DryRun {
		fmt.Println()
		output.Warn("This was a dry run. To actually kill processes:")
		output.Info(fmt.Sprintf("  anubis guard --slay %s --confirm", result.Target))
	}
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
