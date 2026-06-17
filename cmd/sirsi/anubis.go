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
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var (
	anubisSudo           bool
	anubisAll            bool
	anubisDryRun         bool
	anubisConfirm        bool
	anubisIncludeCaution bool
	anubisDocs           bool

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
	Long: `Anubis — Scan, clean, and manage disk space.

  sirsi anubis scan           Find infrastructure waste
  sirsi anubis ghosts         Find remnants of uninstalled apps
  sirsi anubis clean          Preview and remove safe items
  sirsi anubis duplicates     Find duplicate files
  sirsi anubis monitor        Watch processes and RAM pressure`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if anubisDocs {
			output.Info("Opening Anubis docs...")
			return help.OpenDocs("anubis")
		}
		return cmd.Help()
	},
}

var anubisWeighCmd = &cobra.Command{
	Use:     "scan",
	Aliases: []string{"weigh"},
	Short:   "Scan your workstation for infrastructure waste",
	RunE:    func(cmd *cobra.Command, args []string) error { return runWeigh(cmd.Context()) },
}

var anubisJudgeCmd = &cobra.Command{
	Use:     "clean",
	Aliases: []string{"judge"},
	Short:   "Clean artifacts and reclaim storage space",
	RunE:    func(cmd *cobra.Command, args []string) error { return runJudge(cmd.Context()) },
}

var anubisKaCmd = &cobra.Command{
	Use:     "ghosts",
	Aliases: []string{"ka"},
	Short:   "Find remnants of uninstalled apps",
	RunE:    func(cmd *cobra.Command, args []string) error { return runKa(cmd.Context()) },
}

var anubisMirrorCmd = &cobra.Command{
	Use:     "duplicates [directories...]",
	Aliases: []string{"mirror", "dedup"},
	Short:   "Find duplicate files",
	RunE:    runAnubisMirror,
}

var anubisGuardCmd = &cobra.Command{
	Use:     "monitor",
	Aliases: []string{"guard"},
	Short:   "Watch processes and RAM pressure",
	RunE:    runAnubisGuard,
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
	anubisJudgeCmd.Flags().BoolVar(&anubisConfirm, "confirm", false, "Confirm and apply (does NOT change scope — preview matches apply)")
	anubisJudgeCmd.Flags().BoolVar(&anubisIncludeCaution, "include-caution", false, "Also target caution-tier items (applies to BOTH preview and apply)")
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
		output.Header("Infrastructure Scan")
	}

	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	stopSpin := output.Spinner("Scanning for infrastructure waste...")
	res, scanErr := engine.Scan(ctx, jackal.ScanOptions{})
	stopSpin()
	if scanErr != nil {
		output.Warn("Scan error (partial results may follow): %v", scanErr)
	}

	// Ghost scan is part of a full scan — dead app remnants are waste too.
	stopSpin = output.Spinner("Scanning for ghost app remnants...")
	ghostScanner := ka.NewScanner()
	ghosts, ghostErr := ghostScanner.Scan(ctx, false)
	stopSpin()
	if ghostErr != nil {
		output.Warn("Ghost scan error: %v", ghostErr)
	}
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

	// Enrich every finding with advisory intelligence.
	jackal.EnrichAdvisory(res)

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

	// Show top 10 individual findings with advisories.
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
			fix := ""
			if f.CanFix {
				fix = " → " + f.Remediation
			}
			output.Dim("  %s %s — %s (%s)", sev, f.Description, output.ShortenPath(f.Path), jackal.FormatSize(f.SizeBytes))
			if f.Advisory != "" {
				output.Dim("    %s%s", f.Advisory, fix)
			}
		}
		if len(res.Findings) > limit {
			output.Dim("  ... and %d more findings.", len(res.Findings)-limit)
		}
	}

	// Structured result contract
	result := &output.CommandResult{
		Command:  "sirsi scan",
		Summary:  fmt.Sprintf("Found %s of reclaimable waste across %d findings", jackal.FormatSize(res.TotalSize), len(res.Findings)),
		Duration: elapsed,
	}
	result.AddEvidence("Waste found", jackal.FormatSize(res.TotalSize))
	result.AddEvidence("Rules ran", fmt.Sprintf("%d", res.RulesRan))
	result.AddEvidence("Findings", fmt.Sprintf("%d", len(res.Findings)))
	if len(ghosts) > 0 {
		result.AddEvidence("Ghost apps", fmt.Sprintf("%d (%s)", len(ghosts), jackal.FormatSize(ghostWaste)))
	}
	if scanErr != nil {
		result.AddWarning("Scan completed with errors: %v", scanErr)
	}
	if ghostErr != nil {
		result.AddWarning("Ghost scan completed with errors: %v", ghostErr)
	}
	safeCount := 0
	for _, f := range res.Findings {
		if f.Severity == jackal.SeveritySafe && f.CanFix {
			safeCount++
		}
	}
	if safeCount > 0 {
		result.AddNextAction("sirsi clean", fmt.Sprintf("Remove %d safe items (moved to Trash)", safeCount))
	}
	if len(ghosts) > 0 {
		result.AddNextAction("sirsi ghosts", "Review and exorcise ghost app remnants")
	}
	result.AddNextAction("sirsi purge", "Remove stale build artifacts (node_modules, target, etc.)")
	result.AddNextAction("sirsi diagnose", "Check system health (RAM, disk, kernel panics)")
	result.Render()
	return nil
}

// selectCleanTargets returns the findings `sirsi clean` targets, given whether
// caution-tier items are included. Scope is a function of severity + the
// include-caution toggle ONLY — there is deliberately NO `confirm` parameter,
// so the dry-run preview and the --confirm apply operate on the IDENTICAL set
// (Rule A1: preview must match apply; apply cannot widen scope). SeverityWarning
// findings are never auto-cleaned.
func selectCleanTargets(findings []jackal.Finding, includeCaution bool) []jackal.Finding {
	var safe, caution []jackal.Finding
	for _, f := range findings {
		switch f.Severity {
		case jackal.SeveritySafe:
			safe = append(safe, f)
		case jackal.SeverityCaution:
			caution = append(caution, f)
		}
	}
	target := safe
	if includeCaution {
		target = append(target, caution...)
	}
	return target
}

func runJudge(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("Cleanup")

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

	// Scope is governed by --include-caution ONLY (not --confirm), so the preview
	// (dry-run) and the apply (--confirm) target the IDENTICAL set — the amount
	// shown is exactly what moves to Trash (Rule A1: preview == apply). The pure
	// selector has no confirm parameter, so apply structurally cannot widen scope.
	target := selectCleanTargets(findings, anubisIncludeCaution)

	if len(target) == 0 {
		output.Info("No safe findings to clean. Use --include-caution to also target caution items.")
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
		cr := &output.CommandResult{
			Command:  "sirsi clean",
			Summary:  fmt.Sprintf("Dry run: %d items (%s) would be cleaned", len(target), jackal.FormatSize(totalCleanable)),
			Duration: time.Since(start),
		}
		cr.AddEvidence("Cleanable items", fmt.Sprintf("%d", len(target)))
		cr.AddEvidence("Reclaimable space", jackal.FormatSize(totalCleanable))
		cr.AddNextAction("sirsi clean --confirm", "Apply cleanup (items moved to Trash)")
		cr.AddNextAction("sirsi scan", "Run a fresh scan to update findings")
		cr.Render()
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

	// Execute cleanup. Safety here is real, not a demo brake: scan-derived
	// findings only, filtered to safe (or caution via --include-caution),
	// protected paths hardcoded in internal/cleaner, the user just answered
	// [y/N], and every move is trash-first (recoverable).
	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	cleanResult, err := engine.Clean(ctx, target, jackal.CleanOptions{
		DryRun:   false,
		Confirm:  true,
		UseTrash: true,
	})
	if err != nil {
		return fmt.Errorf("clean failed: %w", err)
	}

	// Inscribe judgment to Stele.
	stele.Inscribe("anubis", stele.TypeAnubisJudge, "", map[string]string{
		"cleaned":     fmt.Sprintf("%d", cleanResult.Cleaned),
		"bytes_freed": fmt.Sprintf("%d", cleanResult.BytesFreed),
		"skipped":     fmt.Sprintf("%d", cleanResult.Skipped),
	})

	cr := &output.CommandResult{
		Command:  "sirsi clean",
		Summary:  fmt.Sprintf("Cleaned %d items. Reclaimed %s.", cleanResult.Cleaned, jackal.FormatSize(cleanResult.BytesFreed)),
		Duration: time.Since(start),
	}
	cr.AddEvidence("Items cleaned", fmt.Sprintf("%d", cleanResult.Cleaned))
	cr.AddEvidence("Space reclaimed", jackal.FormatSize(cleanResult.BytesFreed))
	if cleanResult.Skipped > 0 {
		cr.AddWarning("Skipped %d items (protected or errors)", cleanResult.Skipped)
	}
	cr.AddNextAction("sirsi scan", "Verify cleanup with a fresh scan")
	cr.AddNextAction("sirsi ghosts", "Hunt remaining ghost app residuals")
	cr.AddNextAction("sirsi diagnose", "Check overall system health")
	cr.Render()
	return nil
}

func cleanupApplyPaused() bool {
	return os.Getenv("SIRSI_ALLOW_CLEAN_APPLY") != "1"
}

func runKa(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("Ghost App Detection")

	stopSpin := output.Spinner("Detecting ghost app remnants...")
	scanner := ka.NewScanner()
	ghosts, err := scanner.Scan(ctx, anubisSudo)
	stopSpin()
	if err != nil {
		output.Warn("Ghost scan error: %v", err)
	}

	var totalWaste int64
	for _, g := range ghosts {
		totalWaste += g.TotalSize
	}

	elapsed := time.Since(start)

	result := &output.CommandResult{
		Command:  "sirsi ghosts",
		Duration: elapsed,
	}
	if len(ghosts) == 0 {
		result.Summary = "No ghost app remnants detected"
	} else {
		result.Summary = fmt.Sprintf("Found %d ghost apps with %s of reclaimable waste", len(ghosts), jackal.FormatSize(totalWaste))
	}
	result.AddEvidence("Ghost apps", fmt.Sprintf("%d", len(ghosts)))
	result.AddEvidence("Waste", jackal.FormatSize(totalWaste))
	if err != nil {
		result.AddWarning("Scan completed with errors: %v", err)
	}
	for _, g := range ghosts {
		result.AddEvidence(g.AppName, fmt.Sprintf("%d residuals, %s", len(g.Residuals), jackal.FormatSize(g.TotalSize)))
	}
	if len(ghosts) > 0 {
		result.AddNextAction("sirsi scan", "Run a full scan, then sirsi clean --confirm to remove (trash-first)")
	}
	result.AddNextAction("sirsi scan", "Full infrastructure waste scan")
	result.AddNextAction("sirsi diagnose", "Check system health")
	result.Render()
	return nil
}

func runAnubisMirror(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("Duplicate File Detection")

	// Default to the CURRENT DIRECTORY when no path is given. Duplicate
	// detection (3-phase hash) over all of $HOME can take minutes and read as
	// "hung" with no visible completion — a command should scope to where you
	// are and finish promptly. Pass directories to widen (e.g.
	// `sirsi duplicates ~`). Without any path, mirror.Scan returns "no paths
	// specified" and the discarded error left res nil → panic on first deref.
	paths := args
	spinMsg := "Scanning for duplicate files..."
	if len(paths) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			paths = []string{cwd}
			spinMsg = "Scanning current directory for duplicates (pass a path to widen)..."
		}
	}

	stopSpin := output.Spinner(spinMsg)
	opts := mirror.ScanOptions{Paths: paths, DryRun: true}
	res, err := mirror.Scan(opts)
	stopSpin()

	elapsed := time.Since(start)

	if err != nil || res == nil {
		cr := &output.CommandResult{Command: "sirsi duplicates", Duration: elapsed}
		cr.Summary = "Duplicate scan could not run"
		if err != nil {
			cr.AddWarning("scan error: %v", err)
		}
		cr.AddNextAction("sirsi duplicates ~/Downloads", "Scan a specific directory")
		cr.Render()
		return nil
	}

	cr := &output.CommandResult{
		Command:  "sirsi duplicates",
		Duration: elapsed,
	}
	if res.TotalDuplicates == 0 {
		cr.Summary = "No duplicate files found"
	} else {
		cr.Summary = fmt.Sprintf("Found %d duplicate files wasting %s", res.TotalDuplicates, mirror.FormatBytes(res.TotalWasteBytes))
	}
	cr.AddEvidence("Duplicates", fmt.Sprintf("%d", res.TotalDuplicates))
	cr.AddEvidence("Wasted space", mirror.FormatBytes(res.TotalWasteBytes))
	if res.TotalDuplicates > 0 {
		cr.AddNextAction("sirsi scan", "Run a full scan, then sirsi clean --confirm to remove (trash-first)")
	}
	cr.AddNextAction("sirsi scan", "Full infrastructure waste scan")
	cr.AddNextAction("sirsi clean", "Clean safe items from last scan")
	cr.Render()
	return nil
}

func runAnubisGuard(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("Resource Monitor")
	stats, _ := guard.GetStats()

	elapsed := time.Since(start)

	cr := &output.CommandResult{
		Command:  "sirsi monitor",
		Summary:  fmt.Sprintf("RAM: %s / %s — Pressure: %s", stats.UsedMemory, stats.TotalMemory, stats.PressureLevel),
		Duration: elapsed,
	}
	cr.AddEvidence("RAM usage", stats.UsedMemory)
	cr.AddEvidence("Total RAM", stats.TotalMemory)
	cr.AddEvidence("Pressure", stats.PressureLevel)
	cr.AddNextAction("sirsi status", "Live system dashboard with real-time updates")
	cr.AddNextAction("sirsi diagnose", "Full system health diagnostic")
	cr.AddNextAction("sirsi scan", "Scan for infrastructure waste")
	cr.Render()
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
		output.Header("Application Registry")
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
	output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "anubis", Subcommand: "apps"}))
	return nil
}

func runAnubisUninstall(ctx context.Context, appName string, complete bool) error {
	output.Banner()
	output.Header("Application Removal")

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

	report, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("doctor failed: %w", err)
	}

	// Binary-drift check (ADR-023): local, no network. Surfaces the CTR
	// deploy-drift class — a stale sibling binary (e.g. sirsi-menubar) running
	// old internal/router code while source is fixed. Flows into the
	// SessionStart health line via the standard findings surface.
	if drift, derr := selfupdate.ScanHost(); derr == nil {
		f := guard.DiagnosticFinding{
			Check:    "binary-drift",
			Severity: guard.SeverityOK,
			Message:  "Sirsi binaries " + drift.Summary(),
		}
		if !drift.Healthy {
			f.Severity = guard.SeverityWarn
			f.Message = "Sirsi binary drift detected"
			f.Detail = drift.Summary() + " → run `brew upgrade sirsi-pantheon` to update the Sirsi binaries"
			f.Fix = "sirsi self-update"  // appended after the doctor post-pass; set its remediation here
			f.FixKind = guard.FixInstant // self-update replaces the drifted binary → resolves now
		}
		report.Findings = append(report.Findings, f)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	elapsed := time.Since(start)

	result := &output.CommandResult{
		Command:    "sirsi diagnose",
		BriefTitle: "Pantheon System Brief",
		Summary:    fmt.Sprintf("%d signals checked", len(report.Findings)),
		Status:     doctorStatus(report),
		Confidence: "High",
		Duration:   elapsed,
	}
	result.AddEvidence("Health", fmt.Sprintf("%d/100", report.Score))
	result.AddEvidence("Signals", fmt.Sprintf("%d checked", len(report.Findings)))

	warnCount := 0
	for _, f := range report.Findings {
		if f.Severity >= guard.SeverityWarn {
			warnCount++
			result.Priority = append(result.Priority, fmt.Sprintf("%s: %s", f.Check, f.Message))
		}
	}
	if warnCount == 0 {
		result.Priority = append(result.Priority, "No immediate action required.")
	}

	// Recommended action = a concrete, runnable remediation for the WORST actual
	// finding — not "go read the JSON". The brief surfaces NextActions[0], so add
	// Critical findings first, then Warn; dedup commands. Every command is
	// verified to exist (A23: never recommend a broken command).
	// Order remediations by how much they RESOLVE, not by finding order — so the
	// headline (NextActions[0]) is the most useful action. clean/monitor actually
	// fix things; `diagnose --json` only inspects (last resort, e.g. kernel panics).
	rank := map[string]int{
		"sirsi clean":                   0,
		"sirsi clean --include-caution": 0,
		"sirsi monitor":                 1,
		"brew upgrade sirsi-pantheon":   2,
		"sirsi diagnose --json":         3,
	}
	type remedy struct{ cmd, desc string }
	var rems []remedy
	seen := map[string]bool{}
	for _, f := range report.Findings {
		if f.Severity < guard.SeverityWarn {
			continue
		}
		cmd, desc := remediationFor(f.Check)
		if cmd == "" || seen[cmd] {
			continue
		}
		seen[cmd] = true
		rems = append(rems, remedy{cmd, desc})
	}
	sort.SliceStable(rems, func(i, j int) bool { return rank[rems[i].cmd] < rank[rems[j].cmd] })
	for _, r := range rems {
		result.AddNextAction(r.cmd, r.desc)
	}
	if len(result.NextActions) == 0 {
		result.AddNextAction("sirsi scan", "Run a workstation hygiene scan when ready.")
	}
	result.Render()
	return nil
}

// remediationFor maps a diagnostic check to a concrete, runnable remediation
// command + description. Empty command = no automatic remediation (skipped).
// Commands MUST exist (verified: clean, monitor, diagnose, scan, brew upgrade).
func remediationFor(check string) (string, string) {
	switch {
	case strings.Contains(check, "App Crash"), strings.Contains(check, "Crash Report"):
		return "sirsi clean --include-caution", "Preview crash-report cleanup (caution tier); re-run with --confirm to move them to Trash"
	case strings.Contains(check, "Jetsam"), strings.Contains(check, "RAM"),
		strings.Contains(check, "Swap"), strings.Contains(check, "Memory"):
		return "sirsi monitor", "Watch the processes driving memory pressure, then quit the worst offenders"
	case strings.Contains(check, "Disk"):
		return "sirsi clean", "Reclaim disk space from caches, logs, and old installers"
	case strings.Contains(check, "Kernel Panic"):
		return "sirsi diagnose --json", "Inspect the panic reports for the faulting driver or hardware"
	case strings.Contains(check, "drift"):
		return "brew upgrade sirsi-pantheon", "Update the Sirsi binaries to clear version drift"
	default:
		return "", ""
	}
}

// doctorStatus renders the canonical green/amber/red roll-up (the rubric lives in
// guard.HealthStatus) for a CommandResult Status line. Driven by report.Status,
// not a raw score threshold, so historical trends don't read as "Critical".
func doctorStatus(r *guard.DoctorReport) string {
	return r.Status.Icon() + " " + r.Status.Label()
}
