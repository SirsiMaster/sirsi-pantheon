package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/tui"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// version is sourced from the shared build-version contract (internal/version),
// stamped via ldflags — no more hand-edited literal.
var version = modversion.Version

// versionCmd prints the version and optionally checks for updates.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Pantheon version and check for updates",
	RunE: func(cmd *cobra.Command, args []string) error {
		type entry struct {
			display string
			key     string
		}
		layout := []entry{
			{"Fleet", "ra"},
			{"Alignment", "net"},
			{"Memory", "thoth"},
			{"Quality", "maat"},
			{"Health", "isis"},
			{"Knowledge", "seshat"},
			{"Cleanup", "anubis"},
			{"Hardware", "seba"},
			{"Recovery", "osiris"},
		}

		if JsonOutput {
			modules := make(map[string]string, len(layout))
			for _, e := range layout {
				modules[e.key] = modversion.Get(e.key)
			}
			info := modversion.Current("sirsi")
			info.RouterSchemaMax = routerstore.MaxSupportedSchemaVersion()
			result := map[string]interface{}{
				"binary":            info.Binary,
				"version":           info.Version,
				"commit":            info.Commit,
				"date":              info.Date,
				"path":              info.Path,
				"go":                info.GoVer,
				"dirty":             info.Dirty,
				"router_schema_max": info.RouterSchemaMax,
				"product":           "Sirsi Pantheon",
				"modules":           modules,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("𓉴 Sirsi Pantheon %s\n", version)
		fmt.Println("  Unified DevOps Intelligence Platform")
		fmt.Println("  \"One Install. Everything Clean.\"")
		fmt.Println()
		fmt.Println("  Module Versions:")
		for i := 0; i < len(layout); i += 2 {
			left := layout[i]
			line := fmt.Sprintf("    %-10s%-8s", left.display, modversion.Get(left.key))
			if i+1 < len(layout) {
				right := layout[i+1]
				line += fmt.Sprintf("%-10s%s", right.display, modversion.Get(right.key))
			}
			fmt.Println(line)
		}
		return nil
	},
}

var rootCmd = &cobra.Command{
	Use:   "sirsi",
	Short: "Sirsi Pantheon — Infrastructure Hygiene & Developer Intelligence",
	// A failed command must read as a failed command — not as a help dump.
	// Without these, cobra prints the full usage on every RunE error, so a
	// command that hit a real error looks like it "just printed help and did
	// nothing." main() renders the error cleanly instead (one visible failure
	// line, not a wall of usage). Per-command RunE should still return clear,
	// actionable errors.
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Sirsi Pantheon — Infrastructure Hygiene & Developer Intelligence

  Clean My Machine
  sirsi scan               Find infrastructure waste
  sirsi clean              Remove safe items (caches, logs, temp)
  sirsi ghosts             Find remnants of uninstalled apps
  sirsi duplicates         Find duplicate files
  sirsi purge              Remove project build artifacts
  sirsi analyze            Visual disk space explorer
  sirsi installer          Find and remove installer files

  Fix My Environment
  sirsi diagnose           Full system health check
  sirsi vitals             Fast memory snapshot (RAM, pressure, top hogs)
  sirsi fix                Auto-fix DNS, firewall, security
  sirsi network            Network security audit
  sirsi monitor            Watch processes and RAM pressure
  sirsi status             Live system dashboard
  sirsi activity           Recent operations — what sirsi actually changed

  Keep Shipping
  sirsi audit              Code quality and governance scan
  sirsi risk               Uncommitted work risk assessment
  sirsi hardware           CPU, GPU, RAM, Neural Engine detection
  sirsi diagram            Architecture diagrams

  Advanced (by module)
  sirsi anubis <verb>      Storage & cleanup module
  sirsi isis <verb>        Health & networking module
  sirsi maat <verb>        Quality & governance module
  sirsi ra <verb>          Fleet orchestration module
  sirsi version            Show version`,
	Run: func(cmd *cobra.Command, args []string) {
		// sirsi no-args launches the interactive operator console (the TUI,
		// ADR-020) when it is on a real terminal. It falls back to printing help
		// verbatim in every non-interactive context — piped, redirected, JSON,
		// quiet, or when SIRSI_NO_TUI=1 opts out — so scripts and CI see the exact
		// same help text they did before (CLI_COMPATIBILITY: help is byte-stable).
		if shouldLaunchTUI() {
			if err := tui.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "sirsi: %v\n", err)
			}
			return
		}
		_ = cmd.Help()
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(verboseMode, quietMode, JsonOutput)
		output.SetOutputMode(JsonOutput, quietMode)

		// First-run FDA check for scan-related commands on macOS.
		// Warns once if Full Disk Access is not granted.
		if !JsonOutput && !quietMode && setup.NeedsFDA(cmd.Name()) && !setup.FullDiskAccessGranted() {
			fmt.Fprintf(os.Stderr, "\n  ⚠ Full Disk Access not granted — some directories may be inaccessible.\n")
			fmt.Fprintf(os.Stderr, "    Run 'sirsi permissions' to fix this once.\n\n")
		}
	},
}

// Top-level aliases for the core user-facing commands.
// These delegate to the internal module commands so users don't need to
// know the module names to use the tool.
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for infrastructure waste",
	RunE:  func(cmd *cobra.Command, args []string) error { return runWeigh(cmd.Context()) },
}

var ghostsCmd = &cobra.Command{
	Use:   "ghosts",
	Short: "Detect remnants of uninstalled applications",
	RunE:  func(cmd *cobra.Command, args []string) error { return runKa(cmd.Context()) },
}

var judgeCmd = &cobra.Command{
	Use:     "judge",
	Aliases: []string{},
	Short:   "Clean artifacts and reclaim storage space (alias for anubis clean)",
	Hidden:  true, // prefer `sirsi clean` or `sirsi anubis clean`
	RunE:    func(cmd *cobra.Command, args []string) error { return runJudge(cmd.Context()) },
}

var cleanCmd = &cobra.Command{
	Use:   "clean [safe|all]",
	Short: "Preview and clean scan findings (safe by default; --confirm to apply)",
	Long: `Clean infrastructure waste found by the last scan.

  sirsi clean                     Preview safe items (caches, logs, temp files)
  sirsi clean --confirm           Apply: move safe items to Trash (asks first)
  sirsi clean --include-caution   Also target caution-tier items (preview + apply)
  sirsi clean all                 Alias for --include-caution

Default is a dry-run preview; the amount shown is exactly what --confirm moves
to Trash (Rule A1: preview matches apply). Run sirsi scan first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Back-compat: `sirsi clean all` == `--include-caution`.
		if len(args) > 0 && args[0] == "all" {
			anubisIncludeCaution = true
		}
		return runJudge(cmd.Context())
	},
}

var dedupCmd = &cobra.Command{
	Use:     "duplicates [directories...]",
	Aliases: []string{"dedup"},
	Short:   "Find duplicate files",
	RunE:    runAnubisMirror,
}

var guardCmd = &cobra.Command{
	Use:    "guard",
	Short:  "Monitor system resources and memory pressure",
	Hidden: true, // prefer `sirsi monitor`
	RunE:   runAnubisGuard,
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Watch processes and RAM pressure",
	RunE:  runAnubisGuard,
}

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Assess the system and resolve what's safely fixable (crashes, disk, caches)",
	Long: `𓁢 Pantheon Fix — assess AND resolve, in one flow.

Diagnoses the workstation, then reclaims the disk/crash backlog (trash-first,
recoverable) and gives a concrete next step for anything it must not auto-touch
(memory hogs, kernel panics). Network security auto-fix lives at 'sirsi isis fix'.`,
	RunE: runFix,
}

var riskCmd = &cobra.Command{
	Use:   "risk",
	Short: "Uncommitted work risk assessment",
	RunE:  runOsirisAssess,
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Code quality and governance scan",
	RunE:  runMaatAudit,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "System status summary",
	Long: `Show a quick system status summary with health score.

  sirsi status          One-shot status summary with next actions
  sirsi status --json   Output status as JSON`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	start := time.Now()

	report, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	elapsed := time.Since(start)

	cr := &output.CommandResult{
		Command:    "sirsi status",
		BriefTitle: "Pantheon Status",
		Summary:    fmt.Sprintf("%d signals checked", len(report.Findings)),
		Status:     doctorStatus(report),
		Confidence: "High",
		Duration:   elapsed,
	}
	cr.AddEvidence("Health", fmt.Sprintf("%d/100", report.Score))

	warnCount := 0
	for _, f := range report.Findings {
		if f.Severity >= guard.SeverityWarn {
			warnCount++
		}
	}
	if warnCount > 0 {
		cr.AddEvidence("Attention", fmt.Sprintf("%d findings", warnCount))
		cr.Priority = append(cr.Priority, "Run a diagnostic brief before taking action.")
	} else {
		cr.Priority = append(cr.Priority, "No immediate action required.")
	}

	cr.AddNextAction("sirsi diagnose", "Open the diagnostic brief.")
	cr.Render()
	return nil
}

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove project build artifacts (node_modules, target, venv, etc.)",
	Long: `Scan for project build artifacts and remove selected ones.

Searches ~/Development, ~/Projects, ~/Documents for:
  node_modules, target, build, dist, venv, .build, Pods, DerivedData

Projects modified within 7 days are marked Recent and skipped by default.
Removed items are moved to Trash.

  sirsi purge          Interactive selection (launches TUI)
  sirsi purge --json   List artifacts as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		roots := jackal.DefaultPurgeRoots()
		if !JsonOutput {
			output.Banner()
			output.Header("Build Artifact Purge")
		}
		stopSpin := output.Spinner("Scanning for build artifacts...")
		res, err := jackal.ScanArtifacts(roots)
		stopSpin()
		if err != nil {
			return err
		}
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}
		cr := &output.CommandResult{
			Command:  "sirsi purge",
			Duration: time.Since(start),
		}
		if len(res.Artifacts) == 0 {
			cr.Summary = "No project build artifacts found"
		} else {
			cr.Summary = fmt.Sprintf("Found %d build artifacts totaling %s", len(res.Artifacts), jackal.FormatSize(res.TotalSize))
			cr.AddEvidence("Artifacts", fmt.Sprintf("%d", len(res.Artifacts)))
			cr.AddEvidence("Total size", jackal.FormatSize(res.TotalSize))
			for _, a := range res.Artifacts {
				tag := ""
				if a.IsRecent {
					tag = " (recent)"
				}
				cr.AddEvidence(a.ProjectName, fmt.Sprintf("%s %s%s", jackal.FormatSize(a.Size), string(a.Type), tag))
			}
			cr.AddNextAction("sirsi scan", "Run a full scan, then sirsi clean --confirm to remove (trash-first)")
		}
		cr.AddNextAction("sirsi scan", "Full infrastructure waste scan")
		cr.AddNextAction("sirsi analyze", "Disk usage explorer")
		cr.Render()
		return nil
	},
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Visual disk space explorer",
	Long: `Analyze disk usage with proportional bar display.

  sirsi analyze              Analyze home directory
  sirsi analyze ~/Documents  Analyze specific path
  sirsi analyze --json       Output as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := os.UserHomeDir()
		if len(args) > 0 {
			target = args[0]
		}
		if !JsonOutput {
			output.Banner()
			output.Header("Disk Analyzer")
		}
		stopSpin := output.Spinner("Analyzing disk usage...")
		res, err := jackal.Analyze(target, 0)
		stopSpin()
		if err != nil {
			return err
		}
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}
		cr := &output.CommandResult{
			Command:  "sirsi analyze",
			Summary:  fmt.Sprintf("Analyzed %s — %s total", output.ShortenPath(target), jackal.FormatSize(res.TotalSize)),
			Duration: res.ScanTime,
		}
		cr.AddEvidence("Path", output.ShortenPath(target))
		cr.AddEvidence("Total size", jackal.FormatSize(res.TotalSize))
		cr.AddEvidence("Entries", fmt.Sprintf("%d", len(res.Entries)))
		// Show top 5 entries as evidence
		limit := 5
		if len(res.Entries) < limit {
			limit = len(res.Entries)
		}
		for _, e := range res.Entries[:limit] {
			pct := float64(0)
			if res.TotalSize > 0 {
				pct = float64(e.Size) / float64(res.TotalSize) * 100
			}
			cr.AddEvidence(e.Name, fmt.Sprintf("%s (%.1f%%)", jackal.FormatSize(e.Size), pct))
		}
		if len(res.Entries) > limit {
			cr.AddEvidence("...", fmt.Sprintf("+%d more entries", len(res.Entries)-limit))
		}
		cr.AddNextAction("sirsi scan", "Run a full scan, then sirsi clean --confirm to remove (trash-first)")
		cr.AddNextAction("sirsi purge", "Remove stale build artifacts")
		cr.AddNextAction("sirsi scan", "Full infrastructure waste scan")
		cr.Render()
		return nil
	},
}

var installerCmd = &cobra.Command{
	Use:   "installer",
	Short: "Find and remove installer files (.dmg, .pkg, .zip)",
	Long: `Scan Downloads, Desktop, Homebrew cache, and iCloud for installer files.

Finds .dmg, .pkg, .iso, .zip, .tar.gz, and .app.zip files > 10MB.

  sirsi installer          Interactive selection (launches TUI)
  sirsi installer --json   List installers as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		if !JsonOutput {
			output.Banner()
			output.Header("Installer Cleanup")
		}
		stopSpin := output.Spinner("Scanning for installer files...")
		res, err := jackal.ScanInstallers()
		stopSpin()
		if err != nil {
			return err
		}
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}
		cr := &output.CommandResult{
			Command:  "sirsi installer",
			Duration: time.Since(start),
		}
		if len(res.Files) == 0 {
			cr.Summary = "No installer files found"
		} else {
			cr.Summary = fmt.Sprintf("Found %d installer files totaling %s", len(res.Files), jackal.FormatSize(res.TotalSize))
			cr.AddEvidence("Installers", fmt.Sprintf("%d", len(res.Files)))
			cr.AddEvidence("Total size", jackal.FormatSize(res.TotalSize))
			limit := 5
			if len(res.Files) < limit {
				limit = len(res.Files)
			}
			for _, f := range res.Files[:limit] {
				cr.AddEvidence(f.Name, fmt.Sprintf("%s (%s)", jackal.FormatSize(f.Size), f.Source))
			}
			if len(res.Files) > limit {
				cr.AddEvidence("...", fmt.Sprintf("+%d more files", len(res.Files)-limit))
			}
			cr.AddNextAction("sirsi scan", "Run a full scan, then sirsi clean --confirm to remove (trash-first)")
		}
		cr.AddNextAction("sirsi scan", "Full infrastructure waste scan")
		cr.AddNextAction("sirsi purge", "Remove build artifacts")
		cr.Render()
		return nil
	},
}

var doctorCmd = &cobra.Command{
	Use:     "diagnose",
	Aliases: []string{"doctor"},
	Short:   "Full system health diagnostic",
	Long: `Isis Diagnose — System Health Diagnostic

Runs a comprehensive one-shot health check covering:
  • RAM pressure and swap usage
  • Disk space
  • Top memory consumers
  • Recent kernel panics and Jetsam events
  • Sirsi background process health

  sirsi diagnose              Run full diagnostic
  sirsi isis diagnose         Same, under the Isis module
  sirsi diagnose --json       Output as JSON`,
	RunE: runDoctor,
}

// Feature aliases — users type features, not deity names.
var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Network security audit (DNS, WiFi, TLS, firewall, VPN)",
	RunE:  runIsisNetwork,
}

var hardwareCmd = &cobra.Command{
	Use:   "hardware",
	Short: "CPU, GPU, RAM, Neural Engine detection",
	RunE:  runSebaHardware,
}

var qualityCmd = &cobra.Command{
	Use:   "quality",
	Short: "Code governance audit",
	RunE:  runMaatAudit,
}

var diagramCmd = &cobra.Command{
	Use:   "diagram",
	Short: "Generate architecture diagrams",
	RunE:  runSebaDiagram,
}

var isisCmd = &cobra.Command{
	Use:   "isis",
	Short: "𓁐 Isis — Health, diagnostics, network security",
	Long: `Isis — Diagnose, fix, and monitor system health.

  sirsi isis diagnose         Full system health check
  sirsi isis network          Network security posture audit
  sirsi isis fix              Auto-fix DNS, firewall, security
  sirsi isis monitor          Watch processes and RAM pressure`,
}

var isisDiagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Full system health diagnostic",
	RunE:  runDoctor,
}

var isisFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Auto-fix DNS, firewall, and security issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		isisNetworkFix = true
		return runIsisNetwork(cmd, args)
	},
}

var isisNetworkCmd = &cobra.Command{
	Use:   "network",
	Short: "Audit network security posture (DNS, WiFi, TLS, firewall, VPN)",
	Long: `𓁐 Isis Network — Security Posture Audit

Checks your network configuration for public WiFi safety:
  • DNS: Is encrypted DNS (DoH/DoT) configured?
  • WiFi: WPA3/WPA2 or open network?
  • TLS: Verifies TLS 1.3 to api.anthropic.com
  • CA Certificates: Audits root certificate store for anomalies
  • VPN: Detects active VPN tunnels
  • Firewall: Is macOS application firewall enabled?

  sirsi isis network          Run audit (read-only)
  sirsi isis network --fix    Auto-apply safe fixes (DNS, firewall)
  sirsi isis network --json   Output as JSON`,
	RunE: runIsisNetwork,
}

var isisNetworkFix bool
var isisNetworkRollback bool

func runIsisNetwork(cmd *cobra.Command, args []string) error {
	start := time.Now()

	// Handle rollback before anything else
	if isisNetworkRollback {
		if !JsonOutput {
			output.Banner()
			output.Header("Network Rollback")
		}
		msg, err := guard.RollbackNetwork(platform.Current())
		if err != nil {
			return err
		}
		if !JsonOutput {
			output.Success("%s", msg)
			output.Footer(time.Since(start))
		} else {
			fmt.Printf("{\"rollback\": %q}\n", msg)
		}
		return nil
	}

	if !JsonOutput {
		output.Banner()
		output.Header("Network Security Audit")
	}

	var report *guard.NetworkReport
	var err error
	if isisNetworkFix {
		report, err = guard.NetworkAuditFix()
	} else {
		report, err = guard.NetworkAudit()
	}
	if err != nil {
		return fmt.Errorf("network audit failed: %w", err)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

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

	for _, f := range report.Findings {
		if f.Detail != "" && f.Severity >= guard.SeverityWarn {
			output.Dim("  %s: %s", f.Check, f.Detail)
		}
	}

	scoreIcon := "🟢"
	switch {
	case report.Score < 50:
		scoreIcon = "🔴"
	case report.Score < 75:
		scoreIcon = "🟡"
	}

	elapsed := time.Since(start)

	cr := &output.CommandResult{
		Command:  "sirsi network",
		Summary:  fmt.Sprintf("Network security score: %s %d/100 (%d checks)", scoreIcon, report.Score, len(report.Findings)),
		Duration: elapsed,
	}
	cr.AddEvidence("Security score", fmt.Sprintf("%d/100", report.Score))
	cr.AddEvidence("Checks run", fmt.Sprintf("%d", len(report.Findings)))

	for _, f := range report.Findings {
		if f.Severity >= guard.SeverityWarn {
			cr.AddWarning("%s: %s", f.Check, f.Message)
		}
	}

	if report.Score < 75 {
		cr.AddNextAction("sirsi fix", "Auto-fix DNS, firewall, and security issues")
	}
	cr.AddNextAction("sirsi diagnose", "Full system health diagnostic")
	cr.AddNextAction("sirsi scan", "Scan for infrastructure waste")
	cr.Render()
	return nil
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for IDE integration",
	Long: `Start the Model Context Protocol server for AI IDE integration.

Pantheon exposes scanning, ghost detection, project memory, and system
health as MCP tools that any compatible IDE can call.

Configure in your IDE:
  {
    "mcpServers": {
      "sirsi": {
        "command": "/absolute/path/to/sirsi",
        "args": ["mcp"]
      }
    }
  }`,
	Run: func(cmd *cobra.Command, args []string) {
		unlock, err := platform.TryLock("mcp-cli")
		if err != nil {
			output.Error("MCP server is already running.")
			return
		}
		defer unlock()

		// Hint on stderr (won't interfere with JSON-RPC on stdout).
		// If stdin is a terminal, the user is running this manually — not from an IDE.
		if isTerminal(os.Stdin.Fd()) {
			fmt.Fprintln(os.Stderr, "MCP server starting on stdio. Waiting for IDE connection...")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "This command is designed to be called by an AI IDE (Claude, Cursor, Windsurf).")
			fmt.Fprintln(os.Stderr, "Add this to your IDE's MCP config:")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintf(os.Stderr, "  %s\n", setup.MCPConfigSnippet())
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Press Ctrl+C to exit.")
		}

		server := mcp.NewServer()
		if err := server.Run(); err != nil {
			output.Error("MCP server error: %v", err)
			os.Exit(1)
		}
	},
}

// The Pantheon TUI and brand-gateway menu were removed 2026-05-21 per ADR-018.
// The forthcoming native macOS app (cmd/sirsi-app/) replaces them; `sirsi`
// with no arguments now prints help. `sirsi pantheon` / `sirsi tui` aliases
// are no longer registered.

// (not piped from an IDE or redirected from a file).
// isTerminal reports whether fd is a real terminal. It uses golang.org/x/term
// rather than an os.ModeCharDevice stat, which wrongly classifies /dev/null
// (a character device, but not a TTY) as interactive.
func isTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

// shouldLaunchTUI decides whether `sirsi` with no args opens the interactive
// operator console. It launches ONLY on a genuine interactive terminal (both
// stdin and stdout are TTYs) and never when output is being consumed by a tool
// or a human wants the plain help: JSON/quiet flags, or the SIRSI_NO_TUI opt-out
// each force the help path. This keeps `echo "" | sirsi` and `sirsi > f` printing
// help byte-for-byte unchanged.
func shouldLaunchTUI() bool {
	if os.Getenv("SIRSI_NO_TUI") != "" {
		return false
	}
	if JsonOutput || quietMode {
		return false
	}
	return isTerminal(os.Stdin.Fd()) && isTerminal(os.Stdout.Fd())
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&JsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress output")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Debug logging")

	// Feature aliases — the primary user interface
	auditCmd.Flags().BoolVar(&auditFull, "full", false, "Run a live go test -cover pass (slow); default uses cached coverage")
	auditCmd.Flags().BoolVar(&auditSkipTests, "skip-test", false, "Deprecated: fast/cached is now the default")
	_ = auditCmd.Flags().MarkHidden("skip-test")
	networkCmd.Flags().BoolVar(&isisNetworkFix, "fix", false, "Auto-apply safe fixes (DNS, firewall)")
	networkCmd.Flags().BoolVar(&isisNetworkRollback, "rollback", false, "Restore DNS to pre-fix state")
	diagramCmd.Flags().StringVar(&diagramType, "type", "all", "Diagram type (hierarchy|dataflow|modules|memory|governance|pipeline|all)")
	diagramCmd.Flags().BoolVar(&diagramHTML, "html", false, "Generate self-contained HTML")

	// Core commands
	scanCmd.Flags().BoolVar(&anubisAll, "all", false, "Scan all categories")
	ghostsCmd.Flags().BoolVar(&anubisSudo, "sudo", false, "Include system directories (requires sudo)")
	// Safe ghost-clean lever (Rule A1): dry-run default, trash-first, protected-aware.
	ghostsCleanCmd.Flags().BoolVar(&ghostsCleanConfirm, "confirm", false, "Actually move remnants to Trash (default is a dry-run preview)")
	ghostsCleanCmd.Flags().StringVar(&ghostsCleanApp, "app", "", "Scope the clean to a single ghost app by name")
	ghostsCmd.AddCommand(ghostsCleanCmd)
	judgeCmd.Flags().BoolVar(&anubisDryRun, "dry-run", true, "Preview mode")
	judgeCmd.Flags().BoolVar(&anubisConfirm, "confirm", false, "Confirm and apply")
	judgeCmd.Flags().BoolVar(&anubisYes, "yes", false, "Skip the interactive [y/N] prompt (with --confirm)")
	// Top-level `sirsi clean` shares the one A1-correct engine (runJudge): preview
	// by default, --confirm to apply (asks first), --include-caution for scope.
	// --yes suppresses ONLY the [y/N] stdin prompt — for dispatchers (TUI/menubar/
	// dashboard) that render their own confirmation; scope is untouched and
	// --yes without --confirm stays a dry-run (TUI design proof gap V2).
	cleanCmd.Flags().BoolVar(&anubisDryRun, "dry-run", true, "Preview only (default); use --confirm to apply")
	cleanCmd.Flags().BoolVar(&anubisConfirm, "confirm", false, "Apply the cleanup — move items to Trash (asks first)")
	cleanCmd.Flags().BoolVar(&anubisYes, "yes", false, "Skip the interactive [y/N] prompt (with --confirm; for dispatchers that confirmed already)")
	cleanCmd.Flags().BoolVar(&anubisIncludeCaution, "include-caution", false, "Also target caution-tier items (preview and apply)")
	fixCmd.Flags().BoolVar(&fixYes, "yes", false, "Apply safe reclaim without the confirmation prompt")
	// ── User-facing commands (visible in sirsi --help) ──
	rootCmd.AddCommand(scanCmd, cleanCmd, ghostsCmd, dedupCmd, doctorCmd)
	rootCmd.AddCommand(purgeCmd, analyzeCmd, installerCmd)
	rootCmd.AddCommand(networkCmd, fixCmd, monitorCmd)
	rootCmd.AddCommand(vitalsCmd, activityCmd)
	rootCmd.AddCommand(launchdCmd)
	rootCmd.AddCommand(spotlightExcludeCmd)
	rootCmd.AddCommand(auditCmd, riskCmd, hardwareCmd, diagramCmd, statusCmd)
	rootCmd.AddCommand(versionCmd, quickstartCmd, setupCmd, routerCmd, agentCmd, threadCmd)
	rootCmd.AddCommand(newReqCmd()) // ADR-057 step 1: canonical requirement registry
	rootCmd.AddCommand(newADRCmd()) // ADR-057: durable ADR number allocation — no more cross-claims
	rootCmd.AddCommand(ctrCmd)      // 𓁢 CTR — Check The Router: universal on-demand wake primitive (ctr / /ctr)
	rootCmd.AddCommand(selfUpdateCmd)

	// ── Power-user deity modules (hidden from default help, still work) ──
	anubisCmd.Hidden = true
	isisCmd.Hidden = true
	maatCmd.Hidden = true
	osirisCmd.Hidden = true
	sebaCmd.Hidden = true
	seshatCmd.Hidden = true
	raCmd.Hidden = true
	netCmd.Hidden = true
	thothCmd.Hidden = true
	horusCmd.Hidden = true
	rootCmd.AddCommand(anubisCmd, sebaCmd, osirisCmd)
	rootCmd.AddCommand(brandCmd)      // 𓂀 canonical Pantheon palette + token emitter (ADR-038)
	rootCmd.AddCommand(gemmaCmd)      // human-facing 'sirsi gemma "<prompt>"' → local on-device model
	rootCmd.AddCommand(brainCmd)      // 𓁟 Orchestration Brain control plane (A29, ADR-034): tiered/pluggable LLM spectrum over the EXISTING router+wake substrate
	rootCmd.AddCommand(autonomousCmd) // 𓁟 master ACTION switch: observe-only vs. self-managing, orthogonal to the LLM Level (deterministic Tier-0 loop)
	rootCmd.AddCommand(relieveCmd)    // on-demand hang relief: renice the live CPU offender (A1-protected)
	rootCmd.AddCommand(hapiCmd)       // 𓁢 Hapi: the live MEMORY governor — stop a runaway before the kernel Jetsams (ADR-031-A Layer 4)
	rootCmd.AddCommand(thothCmd, maatCmd, seshatCmd, raCmd, netCmd)

	// ── Internal tools (hidden) ──
	guardCmd.Hidden = true
	judgeCmd.Hidden = true
	qualityCmd.Hidden = true
	mcpCmd.Hidden = true
	benchmarkCmd.Hidden = true
	rtkCmd.Hidden = true
	vaultCmd.Hidden = true
	notificationsCmd.Hidden = true
	dashboardCmd.Hidden = true
	rootCmd.AddCommand(guardCmd, judgeCmd, qualityCmd, mcpCmd, benchmarkCmd)
	rootCmd.AddCommand(rtkCmd, vaultCmd, horusCmd)
	rootCmd.AddCommand(notificationsCmd, dashboardCmd, ccdCmd)

	// Note: `sirsi dashboard` is branded as Horus (ADR-015).
	// `sirsi horus` remains the code graph subcommand for backward compat.
	// When code graph moves under dashboard as a tab, the horus command
	// will become the dashboard entry point.

	// Workstream manager (hidden). The pantheon/tui launcher commands were
	// removed 2026-05-21 per ADR-018.
	workCmd.Hidden = true
	rootCmd.AddCommand(workCmd)

	// Isis — Health & Remediation
	isisNetworkCmd.Flags().BoolVar(&isisNetworkFix, "fix", false, "Auto-apply safe fixes (DNS, firewall)")
	isisNetworkCmd.Flags().BoolVar(&isisNetworkRollback, "rollback", false, "Restore DNS to pre-fix state")
	isisCmd.AddCommand(isisNetworkCmd)
	isisCmd.AddCommand(isisDiagnoseCmd)
	isisCmd.AddCommand(isisFixCmd)
	rootCmd.AddCommand(isisCmd)

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// SilenceErrors is set on root, so cobra prints nothing — we render the
		// failure as one clean, visible line (styled, JSON-aware) so every
		// command resolves with an unambiguous outcome instead of a usage dump.
		output.Error("%v", err)
		os.Exit(1)
	}
}
