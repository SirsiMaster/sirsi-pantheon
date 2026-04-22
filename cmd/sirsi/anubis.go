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
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
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

	if !JsonOutput {
		output.Banner()
		output.Header("ANUBIS — The Weighing of the Heart")
	}

	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	res, _ := engine.Scan(ctx, jackal.ScanOptions{})

	// Ghost scan is part of a full scan — dead app remnants are waste too.
	if !JsonOutput {
		output.Info("Scanning for ghost app remnants...")
	}
	ghostScanner := ka.NewScanner()
	ghosts, _ := ghostScanner.Scan(ctx, false)
	var ghostWaste int64
	for _, g := range ghosts {
		ghostWaste += g.TotalSize
		// Add ghost findings to the scan result so they appear in persisted output.
		for _, r := range g.Residuals {
			// Caches and logs are safe to delete. Preferences and app data need review.
			sev := jackal.SeveritySafe
			rType := string(r.Type)
			if rType == "preferences" || rType == "application_support" || rType == "containers" || rType == "group_containers" {
				sev = jackal.SeverityCaution
			}
			res.Findings = append(res.Findings, jackal.Finding{
				RuleName:    "ka_ghost",
				Category:    jackal.CategoryGeneral,
				Description: fmt.Sprintf("Ghost: %s (%s)", g.AppName, rType),
				Path:        r.Path,
				SizeBytes:   r.SizeBytes,
				FileCount:   r.FileCount,
				Severity:    sev,
				IsDir:       true,
			})
		}
		res.TotalSize += g.TotalSize
	}
	if len(ghosts) > 0 {
		res.RulesWithFindings++
		cat := res.ByCategory[jackal.CategoryGeneral]
		cat.Category = jackal.CategoryGeneral
		cat.Findings += len(ghosts)
		cat.TotalSize += ghostWaste
		res.ByCategory[jackal.CategoryGeneral] = cat
	}

	elapsed := time.Since(start)

	// Persist findings to disk so dashboard/judge can read them.
	if err := jackal.Persist(res, elapsed); err != nil {
		output.Warn("Could not persist findings: %v", err)
	}

	// Inscribe to Stele for dashboard awareness.
	catBreakdown := make(map[string]string)
	for cat, summary := range res.ByCategory {
		catBreakdown[string(cat)] = fmt.Sprintf("%d findings, %s", summary.Findings, jackal.FormatSize(summary.TotalSize))
	}
	catBreakdown["total_size"] = fmt.Sprintf("%d", res.TotalSize)
	catBreakdown["total_findings"] = fmt.Sprintf("%d", len(res.Findings))
	catBreakdown["rules_ran"] = fmt.Sprintf("%d", res.RulesRan)
	stele.Inscribe("anubis", stele.TypeAnubisScan, "", catBreakdown)

	// JSON output mode — full structured results.
	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	// Terminal output — summary table + top findings.
	dashMap := map[string]string{
		"Waste Found": jackal.FormatSize(res.TotalSize),
		"Pillars Ran": fmt.Sprintf("%d", res.RulesRan),
		"Findings":    fmt.Sprintf("%d", len(res.Findings)),
	}
	if len(ghosts) > 0 {
		dashMap["Ghosts"] = fmt.Sprintf("%d (%s)", len(ghosts), jackal.FormatSize(ghostWaste))
	}
	output.Dashboard(dashMap)

	// Show category breakdown.
	if len(res.ByCategory) > 0 {
		var catRows [][]string
		for cat, summary := range res.ByCategory {
			catRows = append(catRows, []string{
				string(cat),
				fmt.Sprintf("%d", summary.Findings),
				jackal.FormatSize(summary.TotalSize),
			})
		}
		sort.Slice(catRows, func(i, j int) bool { return catRows[i][2] > catRows[j][2] })
		output.Table([]string{"Category", "Findings", "Size"}, catRows)
	}

	// Show top 10 individual findings.
	limit := 10
	if len(res.Findings) < limit {
		limit = len(res.Findings)
	}
	if limit > 0 {
		output.Info("Top %d findings:", limit)
		for _, f := range res.Findings[:limit] {
			sev := "🟢"
			if f.Severity == jackal.SeverityCaution {
				sev = "🟡"
			} else if f.Severity == jackal.SeverityWarning {
				sev = "🟠"
			}
			output.Dim("  %s %s — %s (%s)", sev, f.Description, output.ShortenPath(f.Path), jackal.FormatSize(f.SizeBytes))
		}
		if len(res.Findings) > limit {
			output.Dim("  ... and %d more. Run: sirsi scan --json", len(res.Findings)-limit)
		}
	}

	output.Footer(elapsed)
	return nil
}

func runJudge(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Divine Judgment")

	// Load latest scan results.
	persisted, err := jackal.LoadLatest()
	if err != nil {
		output.Error("No scan results found. Run `sirsi scan` first.")
		return fmt.Errorf("load findings: %w", err)
	}

	if len(persisted.Findings) == 0 {
		output.Success("No findings to judge. System is clean.")
		return nil
	}

	output.Info("Loaded %d findings from scan at %s (%s waste)",
		len(persisted.Findings),
		persisted.Timestamp.Format("15:04:05"),
		jackal.FormatSize(persisted.TotalSize))

	// Rebuild Finding structs from persisted data for the engine.
	var findings []jackal.Finding
	for _, pf := range persisted.Findings {
		f := jackal.Finding{
			RuleName:    pf.RuleName,
			Category:    pf.Category,
			Description: pf.Description,
			Path:        pf.Path,
			SizeBytes:   pf.SizeBytes,
			Severity:    pf.Severity,
			IsDir:       pf.IsDir,
			FileCount:   pf.FileCount,
		}
		if pf.LastModified != "" {
			f.LastModified, _ = time.Parse(time.RFC3339, pf.LastModified)
		}
		findings = append(findings, f)
	}

	// Filter to safe findings only unless --confirm is set.
	var safe, caution []jackal.Finding
	for _, f := range findings {
		switch f.Severity {
		case jackal.SeveritySafe:
			safe = append(safe, f)
		case jackal.SeverityCaution:
			caution = append(caution, f)
		}
		// SeverityWarning items are never auto-cleaned
	}

	target := safe
	if anubisConfirm {
		target = append(target, caution...)
	}

	if len(target) == 0 {
		output.Info("No safe findings to clean. Use --confirm to include caution items.")
		return nil
	}

	// Show what would be cleaned.
	var totalCleanable int64
	for _, f := range target {
		totalCleanable += f.SizeBytes
	}

	output.Info("Cleaning %d findings (%s):", len(target), jackal.FormatSize(totalCleanable))
	limit := 10
	if len(target) < limit {
		limit = len(target)
	}
	for _, f := range target[:limit] {
		output.Dim("  %s — %s (%s)", f.Description, output.ShortenPath(f.Path), jackal.FormatSize(f.SizeBytes))
	}
	if len(target) > limit {
		output.Dim("  ... and %d more", len(target)-limit)
	}

	// Dry-run mode (default) — just show the plan.
	if anubisDryRun && !anubisConfirm {
		output.Info("")
		output.Info("Dry run — no changes made. Run with --confirm to apply.")
		output.Footer(time.Since(start))
		return nil
	}

	// Confirm interactively.
	fmt.Fprintf(os.Stderr, "\n  Proceed? Items will be moved to Trash. [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		output.Info("Canceled.")
		return nil
	}

	// Execute cleanup.
	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	result, err := engine.Clean(ctx, target, jackal.CleanOptions{
		DryRun:  false,
		Confirm: true,
		UseTrash: true,
	})
	if err != nil {
		return fmt.Errorf("clean failed: %w", err)
	}

	output.Success("Cleaned %d items. Reclaimed %s.", result.Cleaned, jackal.FormatSize(result.BytesFreed))
	if result.Skipped > 0 {
		output.Warn("Skipped %d items (protected or errors)", result.Skipped)
	}

	// Inscribe judgment to Stele.
	stele.Inscribe("anubis", stele.TypeAnubisJudge, "", map[string]string{
		"cleaned":    fmt.Sprintf("%d", result.Cleaned),
		"bytes_freed": fmt.Sprintf("%d", result.BytesFreed),
		"skipped":    fmt.Sprintf("%d", result.Skipped),
	})

	output.Footer(time.Since(start))
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
