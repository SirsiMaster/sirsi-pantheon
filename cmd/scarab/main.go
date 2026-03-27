package main

import (
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/spf13/cobra"
)

var version = "v0.4.0-standalone"

func main() {
	_ = version // set via ldflags
	var verboseMode bool

	rootCmd := &cobra.Command{
		Use:   "scarab",
		Short: "𓃠 Scarab — Pantheon Fleet Network Discovery",
		Long: `𓃠 Scarab is the network intelligence deity of the Sirsi Pantheon.
It provides rapid subnet scanning, hardware discovery, and ARP table analysis.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Init(verboseMode, false, false)
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Enable debug logging")

	// Import the subcommand but wrap it for standalone use
	// Note: We'll need to export the subcommand vars from cmd/pantheon or move them to internal.
	// For now, I'm setting the foundation.

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Scarab failed: %v\n", err)
		os.Exit(1)
	}
}
