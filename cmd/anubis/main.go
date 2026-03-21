package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/output"
	"github.com/SirsiMaster/sirsi-anubis/internal/updater"
)

// version is set by goreleaser at build time via -ldflags.
var version = "dev"

// Global flags
var (
	jsonOutput bool
	quietMode  bool
)

// rootCmd is the base command for anubis.
var rootCmd = &cobra.Command{
	Use:   "anubis",
	Short: "𓂀 Sirsi Anubis — The Guardian of Infrastructure Hygiene",
	Long: `𓂀 Sirsi Anubis — The Guardian of Infrastructure Hygiene
"Weigh. Judge. Purge."

Scan, judge, and purge infrastructure waste across workstations,
containers, VMs, networks, and storage backends.

  anubis weigh          Scan your workstation (The Weighing)
  anubis judge          Clean artifacts (The Judgment)
  anubis guard          Manage RAM pressure (The Guardian)
  anubis sight          Fix ghost apps in Spotlight (The Sight)
  anubis hapi           Optimize VRAM & storage (The Flow)
  anubis scarab         Fleet sweep across networks (The Transformer)
  anubis scales         Enforce policies (The Judgment)`,
	Run: func(cmd *cobra.Command, args []string) {
		output.Banner()
		_ = cmd.Help()
	},
}

// versionCmd prints the version and optionally checks for updates.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Anubis version and check for updates",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("𓂀 Sirsi Anubis %s\n", version)
		fmt.Println("  The Guardian of Infrastructure Hygiene")
		fmt.Println("  \"Weigh. Judge. Purge.\"")
		fmt.Printf("  Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

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
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quietMode, "quiet", false, "Suppress all output except errors and summary")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(weighCmd)
	rootCmd.AddCommand(judgeCmd)
	rootCmd.AddCommand(kaCmd)
	rootCmd.AddCommand(guardCmd)
	rootCmd.AddCommand(sightCmd)
	rootCmd.AddCommand(profileCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
