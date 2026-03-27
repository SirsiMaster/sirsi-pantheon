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
		Use:   "maat",
		Short: "🪶 Ma'at — Pantheon QA Governance",
		Long: `🪶 Ma'at is the governance deity of the Sirsi Pantheon.
It weighs your project against the standard, assesses coverage, and generates proofs of truth.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Init(verboseMode, false, false)
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Enable debug logging")

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Ma'at failed: %v\n", err)
		os.Exit(1)
	}
}
