package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

var version = "v0.12.0"

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
			{"Hapi", "hapi"},
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
	Use:   "pantheon",
	Short: "Sirsi Pantheon — Infrastructure Hygiene & Developer Intelligence",
	Long: `Sirsi Pantheon — Infrastructure Hygiene & Developer Intelligence

Core commands:
  pantheon scan               Scan for infrastructure waste (caches, build artifacts, orphaned files)
  pantheon ghosts             Detect remnants of uninstalled applications
  pantheon dedup [dirs...]    Find duplicate files across directories
  pantheon guard              Monitor system resources and memory pressure
  pantheon doctor             One-shot system health diagnostic
  pantheon thoth init         Initialize AI project memory (.thoth/)
  pantheon thoth sync         Sync project memory from source + git history
  pantheon thoth compact      Persist session decisions before context compression
  pantheon mcp                Start MCP server for IDE integration (Claude, Cursor, VS Code)

Advanced:
  pantheon ra --help          Supreme Overseer — cross-repo orchestration
  pantheon anubis --help      Full hygiene engine (scan, judge, clean)
  pantheon isis --help        Health & remediation (diagnostics, network, auto-fix)
  pantheon maat --help        Governance and compliance auditing
  pantheon hapi --help        Hardware profiling and accelerator detection
  pantheon seba --help        Architecture mapping and diagrams`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			if err := output.LaunchTUI(); err != nil {
				output.Banner()
				_ = cmd.Help()
			}
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
  • Pantheon background process health

  pantheon doctor              Run full diagnostic
  pantheon doctor --json       Output as JSON`,
	RunE: runDoctor,
}

var isisCmd = &cobra.Command{
	Use:   "isis",
	Short: "𓁐 Health & Remediation — diagnostics, network security, auto-fix",
	Long: `𓁐 Isis — Health & Remediation

System health diagnostics, network security auditing, and autonomous remediation.

  pantheon isis network          Network security posture audit
  pantheon isis network --fix    Audit and auto-fix safe issues
  pantheon isis heal             Auto-remediate governance failures
  pantheon doctor                One-shot system health diagnostic`,
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

  pantheon isis network          Run audit (read-only)
  pantheon isis network --fix    Auto-apply safe fixes (DNS, firewall)
  pantheon isis network --json   Output as JSON`,
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
      "pantheon": {
        "command": "pantheon",
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

		server := mcp.NewServer()
		if err := server.Run(); err != nil {
			output.Error("MCP server error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&JsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress output")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Debug logging")

	// Core commands
	scanCmd.Flags().BoolVar(&anubisAll, "all", false, "Scan all categories")
	ghostsCmd.Flags().BoolVar(&anubisSudo, "sudo", false, "Include system directories (requires sudo)")
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(ghostsCmd)
	rootCmd.AddCommand(dedupCmd)
	rootCmd.AddCommand(guardCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(thothCmd)
	rootCmd.AddCommand(maatCmd)
	rootCmd.AddCommand(seshatCmd)
	rootCmd.AddCommand(raCmd)
	rootCmd.AddCommand(anubisCmd)
	rootCmd.AddCommand(hapiCmd)
	rootCmd.AddCommand(sebaCmd)
	rootCmd.AddCommand(benchmarkCmd)
	rootCmd.AddCommand(versionCmd)

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
