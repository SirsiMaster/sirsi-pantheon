package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stealth"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

// version is set by goreleaser at build time via -ldflags.
var version = "dev"

// rootCmd is the base command for pantheon.
var rootCmd = &cobra.Command{
	Use:   "pantheon",
	Short: "🏛️ Sirsi Pantheon — Unified DevOps Intelligence Platform",
	Long: `🏛️ Sirsi Pantheon — Unified DevOps Intelligence Platform
"One Install. All Deities."

Pantheon unifies every deity in the Sirsi ecosystem into a single binary.
Anubis (infrastructure hygiene) is the foundational module — scan, judge,
and purge waste across workstations, containers, VMs, and networks.

  𓂀 Anubis — Infrastructure Hygiene
  pantheon weigh          Scan your workstation (The Weighing)
  pantheon judge          Clean artifacts (The Judgment)
  pantheon guard          Manage RAM pressure (The Guardian)
  pantheon sight          Fix ghost apps in Spotlight (The Sight)
  pantheon hapi           Optimize VRAM & storage (The Flow)
  pantheon scarab         Fleet sweep across networks (The Transformer)
  pantheon scales         Enforce policies (The Judgment)

  🪶 Ma'at — QA/QC Governance
  pantheon maat           Run governance assessments

  𓁟 Thoth — Persistent Knowledge
  pantheon mcp            AI IDE integration (includes thoth_read_memory)`,
	Run: func(cmd *cobra.Command, args []string) {
		output.Banner()
		_ = cmd.Help()
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(verboseMode, quietMode, JsonOutput)
		logging.Debug("pantheon starting", "version", version, "platform", runtime.GOOS+"/"+runtime.GOARCH)
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if stealthMode {
			_ = stealth.CleanExit()
		}
	},
}

// versionCmd prints the version and optionally checks for updates.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Pantheon version and check for updates",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🏛️ Sirsi Pantheon %s\n", version)
		fmt.Println("  Unified DevOps Intelligence Platform")
		fmt.Println("  \"One Install. All Deities.\"")
		fmt.Printf("  Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Println()
		fmt.Println("  Deities:")
		fmt.Println("    𓂀 Anubis  — Infrastructure Hygiene (foundational)")
		fmt.Println("    🪶 Ma'at   — QA/QC Governance")
		fmt.Println("    𓁟 Thoth   — Persistent Knowledge")

		// Phone home — check for updates and advisories
		result := updater.Check(version)
		if notice := updater.FormatUpdateNotice(result); notice != "" {
			fmt.Print(notice)
		}
		if advisory := updater.FormatAdvisories(result.Advisories); advisory != "" {
			fmt.Print(advisory)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&JsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress all output except errors and summary")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Enable debug logging (stderr)")
	rootCmd.PersistentFlags().BoolVar(&stealthMode, "stealth", false, "Ephemeral mode — delete all Pantheon data after execution")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(weighCmd)
	rootCmd.AddCommand(judgeCmd)
	rootCmd.AddCommand(kaCmd)
	rootCmd.AddCommand(guardCmd)
	rootCmd.AddCommand(sightCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(sebaCmd)
	rootCmd.AddCommand(bookCmd)
	rootCmd.AddCommand(initiateCmd)
	rootCmd.AddCommand(hapiCmd)
	rootCmd.AddCommand(scarabCmd)
	rootCmd.AddCommand(installBrainCmd)
	rootCmd.AddCommand(uninstallBrainCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(scalesCmd)
	rootCmd.AddCommand(mirrorCmd)
	rootCmd.AddCommand(maatCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
