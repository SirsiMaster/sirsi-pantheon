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
		Use:   "anubis",
		Short: "𓂀 Anubis — Pantheon Infrastructure Hygiene",
		Long: `𓂀 Anubis is the foundational deity of the Sirsi Pantheon.
It manages workstation cleanup, artifact purging, and rule-based governance.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Init(verboseMode, false, false)
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Enable debug logging")

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Anubis failed: %v\n", err)
		os.Exit(1)
	}
}
