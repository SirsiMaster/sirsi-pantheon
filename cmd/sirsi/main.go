package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"github.com/SirsiMaster/sirsi-pantheon/internal/workstream"
)

var version = "v0.17.0"

// versionCmd prints the version and optionally checks for updates.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Pantheon version and check for updates",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("𓉴 Sirsi Pantheon %s\n", version)
		fmt.Println("  Unified DevOps Intelligence Platform")
		fmt.Println("  \"One Install. All Deities.\"")
		fmt.Println()
		fmt.Println("  Module Versions:")
		// Display modules in a two-column layout for readability.
		type entry struct {
			display string
			key     string
		}
		layout := []entry{
			{"Ra", "ra"},
			{"Net", "net"},
			{"Thoth", "thoth"},
			{"Ma'at", "maat"},
			{"Isis", "isis"},
			{"Seshat", "seshat"},
			{"Anubis", "anubis"},
			{"Seba", "seba"},
			{"Osiris", "osiris"},
		}
		for i := 0; i < len(layout); i += 2 {
			left := layout[i]
			line := fmt.Sprintf("    %-10s%-8s", left.display, modversion.Get(left.key))
			if i+1 < len(layout) {
				right := layout[i+1]
				line += fmt.Sprintf("%-10s%s", right.display, modversion.Get(right.key))
			}
			fmt.Println(line)
		}
	},
}

var rootCmd = &cobra.Command{
	Use:   "sirsi",
	Short: "Sirsi Pantheon — Infrastructure Hygiene & Developer Intelligence",
	Long: `Sirsi Pantheon — Infrastructure Hygiene & Developer Intelligence

  sirsi scan               Find infrastructure waste (58 rules, 7 domains)
  sirsi ghosts             Detect remnants of uninstalled apps
  sirsi dedup [dirs...]    Find duplicate files with three-phase hashing
  sirsi doctor             System health diagnostic
  sirsi network            Network security audit (DNS, WiFi, TLS, firewall)
  sirsi hardware           CPU, GPU, RAM, Neural Engine detection
  sirsi guard              Real-time resource monitoring
  sirsi quality            Code governance audit
  sirsi thoth init/sync    AI project memory
  sirsi mcp                MCP server for AI IDEs
  sirsi seshat ingest      Knowledge ingestion
  sirsi diagram            Architecture diagrams (Mermaid/HTML)
  sirsi rtk filter         Output noise reduction for AI context
  sirsi vault store/search Context sandbox with FTS5 search
  sirsi horus outline/scan Structural code graph
  sirsi version            Show version`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			showGateway(cmd)
			return
		}
		output.Banner()
		_ = cmd.Help()
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(verboseMode, quietMode, JsonOutput)
	},
}

// Top-level aliases for the core user-facing commands.
// These delegate to the internal deity commands so users don't need to
// know the mythology to use the tool.
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
	Use:   "judge",
	Short: "Clean artifacts and reclaim storage space",
	RunE:  func(cmd *cobra.Command, args []string) error { return runJudge(cmd.Context()) },
}

var cleanCmd = &cobra.Command{
	Use:   "clean [all|safe]",
	Short: "Clean scan findings (default: safe items only)",
	Long: `Clean infrastructure waste found by the last scan.

  sirsi clean          Clean safe items only (caches, logs, temp files)
  sirsi clean all      Clean safe + caution items
  sirsi clean safe     Clean safe items only (same as default)

Loads findings from the last scan. Run sirsi scan first.`,
	RunE: runClean,
}

var dedupCmd = &cobra.Command{
	Use:   "dedup [directories...]",
	Short: "Find duplicate files",
	RunE:  runAnubisMirror,
}

var guardCmd = &cobra.Command{
	Use:   "guard",
	Short: "Monitor system resources and memory pressure",
	RunE:  runAnubisGuard,
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "𓁐 One-shot system health diagnostic (Isis)",
	Long: `𓁐 Isis Doctor — System Health Diagnostic

Runs a comprehensive one-shot health check covering:
  • RAM pressure and swap usage
  • Disk space
  • Top memory consumers
  • Recent kernel panics and Jetsam events
  • Sirsi background process health

  sirsi doctor              Run full diagnostic
  sirsi doctor --json       Output as JSON`,
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
	Short: "𓁐 Health & Remediation — diagnostics, network security, auto-fix",
	Long: `𓁐 Isis — Health & Remediation

System health diagnostics, network security auditing, and autonomous remediation.

  sirsi isis network          Network security posture audit
  sirsi isis network --fix    Audit and auto-fix safe issues
  sirsi isis heal             Auto-remediate governance failures
  sirsi doctor                One-shot system health diagnostic`,
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
			output.Header("ISIS — Network Rollback")
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
		output.Header("ISIS — Network Security Audit")
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

	output.Dashboard(map[string]string{
		"Security Score": fmt.Sprintf("%s %d/100", scoreIcon, report.Score),
		"Checks Run":     fmt.Sprintf("%d", len(report.Findings)),
	})
	output.Footer(time.Since(start))
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
        "command": "sirsi",
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
			fmt.Fprintln(os.Stderr, `  { "mcpServers": { "sirsi": { "command": "sirsi", "args": ["mcp"] } } }`)
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

// showGateway presents the Sirsi brand gateway when no subcommand is given.
// Shows environment status (AI, IDE, workstreams) and routes the user.
// On a clean install with nothing configured, guides through setup.
func showGateway(cmd *cobra.Command) {
	gold := output.TitleStyle
	dim := output.DimStyle
	green := output.SuccessStyle
	red := output.ErrorStyle
	p := platform.Current()

	// ── Load or create inventory ──
	inv, err := workstream.LoadInventory()
	firstRun := err != nil
	if firstRun {
		fmt.Println()
		fmt.Println(gold.Render("  \U000130DF  Sirsi \u2014 First Run"))
		fmt.Println(dim.Render("  \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500"))
		fmt.Println()
		fmt.Println(dim.Render("  Scanning your system..."))
		fmt.Println()
		inv = workstream.ScanInventory(p)
		_ = workstream.SaveInventory(inv)
	}

	ai := inv.InstalledAI()
	ides := inv.InstalledIDEs()

	// Count workstreams
	store, _ := workstream.NewStore(workstream.DefaultConfigPath())
	activeCount := 0
	if store != nil {
		activeCount = len(store.Active())
	}

	// ── Status banner (skip on first run — already printed) ──
	if !firstRun {
		fmt.Println()
		fmt.Println(gold.Render("  𓁟  Sirsi"))
		fmt.Println(dim.Render("  ─────────────────────────────"))
		fmt.Println()
	}

	// System
	fmt.Printf("  %s  %s %s\n", dim.Render("Sys"), inv.OS+"/"+inv.Arch, dim.Render(inv.Shell))

	// AI
	if len(ai) > 0 {
		names := make([]string, len(ai))
		for i, t := range ai {
			names[i] = t.Name
		}
		fmt.Printf("  %s   %s\n", green.Render("AI"), strings.Join(names, ", "))
	} else {
		fmt.Printf("  %s   %s\n", red.Render("AI"), dim.Render("none installed"))
	}

	// IDE
	if len(ides) > 0 {
		names := make([]string, len(ides))
		for i, t := range ides {
			names[i] = t.Name
		}
		fmt.Printf("  %s  %s\n", green.Render("IDE"), strings.Join(names, ", "))
	} else {
		fmt.Printf("  %s  %s\n", red.Render("IDE"), dim.Render("none installed"))
	}

	// Repos
	if len(inv.GitRepos) > 0 {
		fmt.Printf("  %s  %s\n", dim.Render("Dev"), dim.Render(fmt.Sprintf("%d git repos found", len(inv.GitRepos))))
	}

	// Workstreams
	if activeCount > 0 {
		fmt.Printf("  %s %s\n", green.Render("Work"), dim.Render(fmt.Sprintf("%d active workstreams", activeCount)))
	} else {
		fmt.Printf("  %s %s\n", red.Render("Work"), dim.Render("no workstreams"))
	}

	// Stale warning
	if !firstRun && inv.IsStale() {
		days := int(inv.Age().Hours() / 24)
		fmt.Printf("\n  %s\n", dim.Render(fmt.Sprintf("Inventory is %d days old. Run: sirsi work inventory", days)))
	}

	fmt.Println()

	// First run or clean install → setup
	if firstRun || (len(ai) == 0 && len(ides) == 0) {
		_ = runSetupFlow()
		fmt.Println()
		fmt.Println(dim.Render("  Quick start — try any of these right now:"))
		fmt.Println()
		fmt.Printf("    %s  %s\n", gold.Render("sirsi scan"), dim.Render("Find waste on your machine"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi doctor"), dim.Render("Check system health"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi ghosts"), dim.Render("Find remnants of uninstalled apps"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi quickstart"), dim.Render("Guided first scan with recommendations"))
		fmt.Println()
		fmt.Println(dim.Render("  Run 'sirsi setup' anytime to configure AI tools and IDEs."))
		fmt.Println()
		return
	}

	// ── Menu ──
	fmt.Printf("  %s  Pantheon      %s\n", gold.Render("1"), dim.Render("Infrastructure & Developer Intelligence"))
	fmt.Printf("  %s  Workstreams   %s\n", gold.Render("2"), dim.Render("Open / manage AI sessions"))
	fmt.Printf("  %s  Setup         %s\n", gold.Render("3"), dim.Render("Install AI assistants & IDEs"))
	fmt.Println()
	fmt.Print(dim.Render("  Choice [2]: "))

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		nStore, _ := notify.Open(notify.DefaultPath())
		if err := output.LaunchTUIWithNotify(nStore); err != nil {
			output.Banner()
			_ = cmd.Help()
		}
		if nStore != nil {
			nStore.Close()
		}
	case "3":
		_ = runSetupFlow()
	default:
		_ = runWorkstreamInteractive(false)
	}
}

// isTerminal returns true if the file descriptor is connected to a terminal
// (not piped from an IDE or redirected from a file).
func isTerminal(fd uintptr) bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&JsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress output")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Debug logging")

	// Feature aliases — the primary user interface
	networkCmd.Flags().BoolVar(&isisNetworkFix, "fix", false, "Auto-apply safe fixes (DNS, firewall)")
	networkCmd.Flags().BoolVar(&isisNetworkRollback, "rollback", false, "Restore DNS to pre-fix state")
	diagramCmd.Flags().StringVar(&diagramType, "type", "all", "Diagram type (hierarchy|dataflow|modules|memory|governance|pipeline|all)")
	diagramCmd.Flags().BoolVar(&diagramHTML, "html", false, "Generate self-contained HTML")
	rootCmd.AddCommand(networkCmd)
	rootCmd.AddCommand(hardwareCmd)
	rootCmd.AddCommand(qualityCmd)
	rootCmd.AddCommand(diagramCmd)

	// Core commands
	scanCmd.Flags().BoolVar(&anubisAll, "all", false, "Scan all categories")
	ghostsCmd.Flags().BoolVar(&anubisSudo, "sudo", false, "Include system directories (requires sudo)")
	judgeCmd.Flags().BoolVar(&anubisDryRun, "dry-run", true, "Preview mode")
	judgeCmd.Flags().BoolVar(&anubisConfirm, "confirm", false, "Confirm and apply")
	rootCmd.AddCommand(scanCmd, ghostsCmd, dedupCmd, guardCmd, doctorCmd, judgeCmd, cleanCmd, mcpCmd)
	rootCmd.AddCommand(thothCmd, maatCmd, seshatCmd, raCmd, netCmd)
	rootCmd.AddCommand(anubisCmd, sebaCmd, osirisCmd)
	rootCmd.AddCommand(benchmarkCmd, versionCmd, quickstartCmd)

	// Token optimization — RTK, Vault, Horus
	rootCmd.AddCommand(rtkCmd, vaultCmd, horusCmd)

	// Notification history + setup + dashboard
	rootCmd.AddCommand(notificationsCmd, setupCmd, dashboardCmd)

	// Note: `sirsi dashboard` is branded as Horus (ADR-015).
	// `sirsi horus` remains the code graph subcommand for backward compat.
	// When code graph moves under dashboard as a tab, the horus command
	// will become the dashboard entry point.

	// Workstream manager (sirsi work / sirsi ws)
	rootCmd.AddCommand(workCmd)

	// Isis — Health & Remediation
	isisNetworkCmd.Flags().BoolVar(&isisNetworkFix, "fix", false, "Auto-apply safe fixes (DNS, firewall)")
	isisNetworkCmd.Flags().BoolVar(&isisNetworkRollback, "rollback", false, "Restore DNS to pre-fix state")
	isisCmd.AddCommand(isisNetworkCmd)
	rootCmd.AddCommand(isisCmd)

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
