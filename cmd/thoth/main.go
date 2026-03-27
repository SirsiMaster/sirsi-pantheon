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
		Short: "Synchronize memory.yaml with codebase facts and generate journal entries from git",
		Run: func(cmd *cobra.Command, args []string) {
			root := findRepoRoot()

			// Phase 1: Sync memory.yaml with codebase facts
			err := thoth.Sync(thoth.SyncOptions{
				RepoRoot:   root,
				UpdateDate: true,
			})
			if err != nil {
				fmt.Printf("Memory sync failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("𓁟 Thoth: memory.yaml synchronized with codebase facts.")

			// Phase 2: Auto-generate journal entries from git history
			since, _ := cmd.Flags().GetString("since")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			count, journalErr := thoth.SyncJournal(thoth.JournalSyncOptions{
				RepoRoot: root,
				Since:    since,
				DryRun:   dryRun,
			})
			if journalErr != nil {
				fmt.Printf("Journal sync failed: %v\n", journalErr)
				os.Exit(1)
			}
			if count > 0 {
				fmt.Printf("𓁟 Thoth: journal.md updated with %d commits from git history.\n", count)
			} else {
				fmt.Println("𓁟 Thoth: no new commits to journal.")
			}
		},
	}
	syncCmd.Flags().String("since", "24 hours ago", "Git log time window (e.g. '7 days ago')")
	syncCmd.Flags().Bool("dry-run", false, "Preview journal entries without writing")

	rootCmd.AddCommand(syncCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Thoth failed: %v\n", err)
		os.Exit(1)
	}
}

// findRepoRoot walks up from cwd to find the .thoth directory.
func findRepoRoot() string {
	cwd, _ := os.Getwd()
	root := cwd
	for {
		if _, err := os.Stat(filepath.Join(root, ".thoth")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			fmt.Println("Error: .thoth directory not found in parent path")
			os.Exit(1)
		}
		root = parent
	}
}
