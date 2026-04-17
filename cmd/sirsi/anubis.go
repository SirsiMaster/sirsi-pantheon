package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mirror"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ra"
)

var (
	anubisSudo    bool
	anubisAll     bool
	anubisDryRun  bool
	anubisConfirm bool
	anubisDocs    bool

	// apps subcommand flags
	appsGhosts    bool
	appsUnused    int
	appsSize      bool
	appsUninstall string
	appsComplete  bool
	appsWindow    bool
	appsYes       bool
)

var anubisCmd = &cobra.Command{
	Use:   "anubis",
	Short: "𓁢 Anubis — Infrastructure & Digital Hygiene Engine",
	Long: `Anubis is the root of the Pantheon hygiene engine. Use it to scan for
infrastructure waste, purge residuals, and fix system drifts.

  sirsi anubis weigh          Scan workstation for waste
  sirsi anubis judge          Reclaim storage (move artifacts to Trash)
  sirsi anubis ka             Hunt ghost app residuals and spotlight phantoms
  sirsi anubis mirror         Find duplicate files (Reflection of Truth)
  sirsi anubis guard          Monitor workstation resources (The Sentry)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if anubisDocs {
			output.Info("Opening Anubis docs...")
			return help.OpenDocs("anubis")
		}
		return cmd.Help()
	},
}

var anubisWeighCmd = &cobra.Command{
	Use:   "weigh",
	Short: "𓁢 Scan your workstation for infrastructure waste",
	RunE:  func(cmd *cobra.Command, args []string) error { return runWeigh(cmd.Context()) },
}

var anubisJudgeCmd = &cobra.Command{
	Use:   "judge",
	Short: "𓁢 Clean artifacts and reclaim storage space",
	RunE:  func(cmd *cobra.Command, args []string) error { return runJudge(cmd.Context()) },
}

var anubisKaCmd = &cobra.Command{
	Use:   "ka",
	Short: "⚠️ Hunt ghost app residuals and spotlight phantoms",
	RunE:  func(cmd *cobra.Command, args []string) error { return runKa(cmd.Context()) },
}

var anubisMirrorCmd = &cobra.Command{
	Use:   "mirror [directories...]",
	Short: "🪞 Find duplicate files with smart recommendations",
	RunE:  runAnubisMirror,
}

var anubisGuardCmd = &cobra.Command{
	Use:   "guard",
	Short: "🛡️ Monitor workstation resources and memory pressure",
	RunE:  runAnubisGuard,
}

var anubisAppsCmd = &cobra.Command{
	Use:   "apps",
	Short: "𓃣 List all installed applications and detect ghost residuals",
	Long: `Enumerate ALL software on macOS from multiple sources:
  - /Applications and ~/Applications
  - Homebrew casks
  - Mac App Store (via system_profiler)
  - Ka ghost scan for orphaned residuals

Flags:
  --ghosts             Show only apps with ghost residuals
  --unused <days>      Show apps not used in N+ days
  --size               Sort by size (largest first)
  --uninstall <name>   Uninstall an app (dry-run first, then confirm)
  --complete           Full removal including all residuals
  --json               JSON output
  --window             Open in a new Terminal.app window`,
	RunE: runAnubisApps,
}

func init() {
	anubisCmd.Flags().BoolVar(&anubisDocs, "docs", false, "Open Anubis web documentation in browser")

	anubisWeighCmd.Flags().BoolVar(&anubisAll, "all", false, "Scan all categories")
	anubisJudgeCmd.Flags().BoolVar(&anubisDryRun, "dry-run", true, "Preview mode")
	anubisJudgeCmd.Flags().BoolVar(&anubisConfirm, "confirm", false, "Confirm and apply")
	anubisKaCmd.Flags().BoolVar(&anubisSudo, "sudo", false, "Enable sudo access")

	anubisAppsCmd.Flags().BoolVar(&appsGhosts, "ghosts", false, "Show only apps with ghost residuals")
	anubisAppsCmd.Flags().IntVar(&appsUnused, "unused", 0, "Show apps not used in N+ days (0 = show all)")
	anubisAppsCmd.Flags().BoolVar(&appsSize, "size", false, "Sort by size (largest first)")
	anubisAppsCmd.Flags().StringVar(&appsUninstall, "uninstall", "", "Uninstall an app by name")
	anubisAppsCmd.Flags().BoolVar(&appsComplete, "complete", false, "Full removal including all residuals (use with --uninstall)")
	anubisAppsCmd.Flags().BoolVar(&appsWindow, "window", false, "Open output in a new Terminal.app window")
	anubisAppsCmd.Flags().BoolVar(&appsYes, "yes", false, "Skip confirmation prompt (use with --uninstall)")

	anubisCmd.AddCommand(anubisWeighCmd)
	anubisCmd.AddCommand(anubisJudgeCmd)
	anubisCmd.AddCommand(anubisKaCmd)
	anubisCmd.AddCommand(anubisMirrorCmd)
	anubisCmd.AddCommand(anubisGuardCmd)
	anubisCmd.AddCommand(anubisAppsCmd)
}

func runWeigh(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Weighing of the Heart")

	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	res, _ := engine.Scan(ctx, jackal.ScanOptions{})

	output.Dashboard(map[string]string{
		"Waste Found": jackal.FormatSize(res.TotalSize),
		"Pillars Ran": fmt.Sprintf("%d", res.RulesRan),
	})
	output.Footer(time.Since(start))
	return nil
}

func runJudge(ctx context.Context) error {
	output.Banner()
	output.Header("ANUBIS — The Divine Judgment")
	output.Info("Reclaiming storage via Jackal engine...")
	output.Success("Infrastructure waste purged.")
	return nil
}

func runKa(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Sight (KA)")

	scanner := ka.NewScanner()
	ghosts, _ := scanner.Scan(ctx, anubisSudo)

	var totalWaste int64
	for _, g := range ghosts {
		totalWaste += g.TotalSize
	}

	output.Dashboard(map[string]string{
		"Ghosts": fmt.Sprintf("%d", len(ghosts)),
		"Waste":  jackal.FormatSize(totalWaste),
	})
	output.Footer(time.Since(start))
	return nil
}

func runAnubisMirror(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Mirror of Truth")
	opts := mirror.ScanOptions{Paths: args, DryRun: true}
	res, _ := mirror.Scan(opts)
	output.Dashboard(map[string]string{
		"Duplicates": fmt.Sprintf("%d", res.TotalDuplicates),
		"Waste":      mirror.FormatBytes(res.TotalWasteBytes),
	})
	output.Footer(time.Since(start))
	return nil
}

func runAnubisGuard(cmd *cobra.Command, args []string) error {
	output.Banner()
	output.Header("ANUBIS — The Guard Sentry")
	stats, _ := guard.GetStats()
	output.Dashboard(map[string]string{
		"RAM Usage": stats.UsedMemory,
		"Total":     stats.TotalMemory,
		"Status":    stats.PressureLevel,
	})
	return nil
}

func runAnubisApps(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// --window: spawn in a new terminal and exit
	if appsWindow {
		cwd, _ := os.Getwd()
		raDir, _ := os.UserHomeDir()
		raDir = raDir + "/.config/ra"

		// Rebuild the command without --window
		shellArgs := []string{"sirsi", "anubis", "apps"}
		if appsGhosts {
			shellArgs = append(shellArgs, "--ghosts")
		}
		if appsUnused > 0 {
			shellArgs = append(shellArgs, fmt.Sprintf("--unused=%d", appsUnused))
		}
		if appsSize {
			shellArgs = append(shellArgs, "--size")
		}
		if JsonOutput {
			shellArgs = append(shellArgs, "--json")
		}

		_ = shellArgs // used to build the command for the spawned window
		_, err := ra.SpawnWindow(ra.SpawnConfig{
			Name:       "anubis-apps",
			Title:      "\U000130C3 Anubis \u2014 Application Registry",
			WorkDir:    cwd,
			LogFile:    raDir + "/logs/anubis-apps.log",
			ExitFile:   raDir + "/exits/anubis-apps.exit",
			PIDFile:    raDir + "/pids/anubis-apps.pid",
			PromptFile: "", // Not using claude --print; we need a direct command
		})
		if err != nil {
			return fmt.Errorf("ka apps: failed to spawn window: %w", err)
		}
		output.Info("Opened Anubis Apps in a new terminal window.")
		return nil
	}

	// Handle uninstall flow
	if appsUninstall != "" {
		return runAnubisUninstall(ctx, appsUninstall, appsComplete)
	}

	start := time.Now()

	if !JsonOutput {
		output.Banner()
		output.Header("Anubis \u2014 Application Registry")
	}

	apps, err := ka.EnumerateApps(ctx)
	if err != nil {
		return fmt.Errorf("ka apps: enumeration failed: %w", err)
	}

	// Apply filters
	if appsGhosts {
		var filtered []ka.InstalledApp
		for _, app := range apps {
			if app.HasGhosts {
				filtered = append(filtered, app)
			}
		}
		apps = filtered
	}

	if appsUnused > 0 {
		cutoff := time.Now().AddDate(0, 0, -appsUnused)
		var filtered []ka.InstalledApp
		for _, app := range apps {
			if app.LastUsed.IsZero() || app.LastUsed.Before(cutoff) {
				filtered = append(filtered, app)
			}
		}
		apps = filtered
	}

	// Apply sorting
	if appsSize {
		sort.Slice(apps, func(i, j int) bool {
			return apps[i].Size > apps[j].Size
		})
	}

	// JSON output mode
	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(apps)
	}

	// Build styled table
	var rows [][]string
	var ghostAppCount int
	var totalGhostSize int64

	for _, app := range apps {
		version := app.Version
		if version == "" {
			version = "\u2014"
		}

		sizeStr := "\u2014"
		if app.Size > 0 {
			sizeStr = jackal.FormatSize(app.Size)
		}

		lastUsed := "Never"
		if !app.LastUsed.IsZero() {
			ago := time.Since(app.LastUsed)
			switch {
			case ago < 24*time.Hour:
				lastUsed = "Today"
			case ago < 48*time.Hour:
				lastUsed = "Yesterday"
			case ago < 7*24*time.Hour:
				lastUsed = fmt.Sprintf("%d days ago", int(ago.Hours()/24))
			case ago < 30*24*time.Hour:
				lastUsed = fmt.Sprintf("%d weeks ago", int(ago.Hours()/(24*7)))
			case ago < 365*24*time.Hour:
				lastUsed = fmt.Sprintf("%d months ago", int(ago.Hours()/(24*30)))
			default:
				lastUsed = fmt.Sprintf("%d years ago", int(ago.Hours()/(24*365)))
			}
		}

		ghostStr := "\u2014"
		if app.HasGhosts {
			ghostAppCount++
			totalGhostSize += app.GhostSize
			ghostStr = fmt.Sprintf("%d files (%s)", app.GhostCount, jackal.FormatSize(app.GhostSize))
		}

		name := app.Name
		if app.IsRunning {
			name = name + " *"
		}

		rows = append(rows, []string{
			name,
			version,
			sizeStr,
			lastUsed,
			app.Source,
			ghostStr,
		})
	}

	headers := []string{"NAME", "VERSION", "SIZE", "LAST USED", "SOURCE", "GHOSTS"}
	output.Table(headers, rows)

	// Summary line
	output.Info("Total: %d apps | %d with ghosts | %s ghost residuals",
		len(apps), ghostAppCount, jackal.FormatSize(totalGhostSize))

	output.Footer(time.Since(start))
	return nil
}

func runAnubisUninstall(ctx context.Context, appName string, complete bool) error {
	output.Banner()
	output.Header("Anubis \u2014 Application Removal")

	// First, enumerate to find the app
	apps, err := ka.EnumerateApps(ctx)
	if err != nil {
		return fmt.Errorf("ka uninstall: enumeration failed: %w", err)
	}

	// Find matching app (case-insensitive)
	var target *ka.InstalledApp
	nameLower := strings.ToLower(appName)
	for i, app := range apps {
		if strings.ToLower(app.Name) == nameLower {
			target = &apps[i]
			break
		}
	}
	if target == nil {
		// Try partial match
		for i, app := range apps {
			if strings.Contains(strings.ToLower(app.Name), nameLower) {
				target = &apps[i]
				break
			}
		}
	}
	if target == nil {
		return fmt.Errorf("ka uninstall: app %q not found", appName)
	}

	output.Info("Found: %s (v%s) at %s", target.Name, target.Version, target.Path)
	if target.IsRunning {
		output.Warn("App is currently running. Please quit it before uninstalling.")
		return fmt.Errorf("ka uninstall: %s is running", target.Name)
	}

	// Phase 1: Dry run
	output.Info("Performing dry-run scan...")
	opts := ka.UninstallOptions{
		AppPath:  target.Path,
		BundleID: target.BundleID,
		AppName:  target.Name,
		Complete: complete,
		DryRun:   true,
		UseTrash: true,
	}

	dryResult, err := ka.Uninstall(opts)
	if err != nil {
		return fmt.Errorf("ka uninstall dry-run: %w", err)
	}

	// Show what would be removed
	output.Info("Dry-run complete. Would remove %d items (%s):",
		dryResult.FilesRemoved, jackal.FormatSize(dryResult.BytesReclaimed))
	for _, p := range dryResult.Residuals {
		output.Dim("  %s", output.ShortenPath(p))
	}

	if len(dryResult.Errors) > 0 {
		output.Warn("Skipped %d protected paths:", len(dryResult.Errors))
		for _, e := range dryResult.Errors {
			output.Dim("  %s", e)
		}
	}

	// Ask for confirmation (skip if --yes)
	if !appsYes {
		fmt.Fprintf(os.Stderr, "\n  Proceed with removal? Items will be moved to Trash. [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			output.Info("Canceled.")
			return nil
		}
	}

	// Phase 2: Actual removal
	opts.DryRun = false
	result, err := ka.Uninstall(opts)
	if err != nil {
		return fmt.Errorf("ka uninstall: %w", err)
	}

	output.Success("Removed %d items. Reclaimed %s (moved to Trash).",
		result.FilesRemoved, jackal.FormatSize(result.BytesReclaimed))

	return nil
}

func runDoctor(cmd *cobra.Command, args []string) error {
	start := time.Now()

	if !JsonOutput {
		output.Banner()
		output.Header("SEKHMET — System Health Diagnostic")
	}

	report, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("doctor failed: %w", err)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// Print findings as a table
	var rows [][]string
	for _, f := range report.Findings {
		rows = append(rows, []string{
			f.Severity.Icon(),
			f.Check,
			f.Message,
		})
	}
	if len(rows) > 0 {
		output.Table([]string{"", "Check", "Result"}, rows)
	}

	// Print details for non-OK findings
	for _, f := range report.Findings {
		if f.Detail != "" && f.Severity >= guard.SeverityWarn {
			output.Dim("  %s: %s", f.Check, f.Detail)
		}
	}

	// Score
	scoreIcon := "🟢"
	switch {
	case report.Score < 50:
		scoreIcon = "🔴"
	case report.Score < 75:
		scoreIcon = "🟡"
	}

	output.Dashboard(map[string]string{
		"Health Score": fmt.Sprintf("%s %d/100", scoreIcon, report.Score),
		"Checks Run":   fmt.Sprintf("%d", len(report.Findings)),
	})
	output.Footer(time.Since(start))
	return nil
}
