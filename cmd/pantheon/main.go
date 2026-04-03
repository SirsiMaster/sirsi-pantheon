package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

var version = "v0.9.0-rc1"

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
			{"Ka", "ka"},
			{"Anubis", "anubis"},
			{"Thoth", "thoth"},
			{"Ma'at", "maat"},
			{"Seshat", "seshat"},
			{"Hapi", "hapi"},
			{"Seba", "seba"},
			{"Horus", "horus"},
			{"Sekhmet", "sekhmet"},
			{"Khepri", "khepri"},
			{"Isis", "isis"},
			{"Neith", "neith"},
			{"Ra", "ra"},
			{"Osiris", "osiris"},
			{"Hathor", "hathor"},
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
  pantheon maat --help        Governance and compliance auditing
  pantheon hapi --help        Hardware profiling and accelerator detection
  pantheon seba --help        Architecture mapping and diagrams`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			if err := output.LaunchDashboard(); err != nil {
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
	Short: "𓁵 One-shot system health diagnostic (Sekhmet)",
	Long: `𓁵 Sekhmet Doctor — System Health Diagnostic

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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
