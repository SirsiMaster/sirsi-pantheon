package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
	"github.com/spf13/cobra"
)

var version = "v0.4.0-standalone"

func main() {
	_ = version // set via ldflags
	var verboseMode bool

	rootCmd := &cobra.Command{
		Use:   "thoth",
		Short: "𓁟 Thoth — Pantheon Persistent Knowledge",
		Long: `𓁟 Thoth is the knowledge deity of the Sirsi Pantheon.
It provides MCP-compatible project memory, AI context management, and canon synchronization.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Init(verboseMode, false, false)
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Enable debug logging")

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize memory.yaml with codebase facts",
		Run: func(cmd *cobra.Command, args []string) {
			cwd, _ := os.Getwd()
			// Find repo root (assume .thoth exists in root)
			root := cwd
			for {
				if _, err := os.Stat(filepath.Join(root, ".thoth")); err == nil {
					break
				}
				parent := filepath.Dir(root)
				if parent == root {
					fmt.Println("Error: .thoth directory not found in parent path")
					os.Exit(1)
				}
				root = parent
			}

			err := thoth.Sync(thoth.SyncOptions{
				RepoRoot:   root,
				UpdateDate: true,
			})
			if err != nil {
				fmt.Printf("Sync failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("𓁟 Thoth: memory.yaml synchronized with codebase facts.")
		},
	}

	rootCmd.AddCommand(syncCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Thoth failed: %v\n", err)
		os.Exit(1)
	}
}
