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
		Use:   "guard",
		Short: "🛡️ Guard — Pantheon Resource Governance",
		Long: `🛡️ Guard is the resource safety deity of the Sirsi Pantheon.
It monitors RAM pressure, identifies orphan processes, and enforces A1 self-preservation rules.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Init(verboseMode, false, false)
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Enable debug logging")

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Guard failed: %v\n", err)
		os.Exit(1)
	}
}
