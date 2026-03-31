package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
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
	},
}

var rootCmd = &cobra.Command{
	Use:   "pantheon",
	Short: "𓉴 Sirsi Pantheon — Unified DevOps Intelligence Platform",
	Long: `𓉴 Sirsi Pantheon — Unified DevOps Intelligence Platform
"One Install. All Deities."

Pantheon unifies the entire Sirsi ecosystem into a single, hardened platform.
Functionality is organized by Deity pillars:

  𓁢 Anubis     Infrastructure Hygiene, Resource Guarding & Mirroring
  𓆄 Ma'at      Governance, Compliance & Autonomous Healing
  𓁟 Thoth      Knowledge Management & AI Intelligence
  𓈗 Hapi       Hardware Profiling & ML Accelerated Compute
  𓇽 Seba       Architectural Mapping & Fleet Network Discovery
  𓁆 Seshat     Gemini Knowledge Bridge & AI Context Server

Run any deity for subcommands:
  pantheon anubis --help`,
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

func init() {
	rootCmd.PersistentFlags().BoolVar(&JsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress output")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Debug logging")

	rootCmd.AddCommand(anubisCmd)
	rootCmd.AddCommand(thothCmd)
	rootCmd.AddCommand(maatCmd)
	rootCmd.AddCommand(hapiCmd)
	rootCmd.AddCommand(sebaCmd)
	rootCmd.AddCommand(seshatCmd)
	rootCmd.AddCommand(neithCmd)
	rootCmd.AddCommand(initiateCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
