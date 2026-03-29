package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
)

var (
	thothSince       string
	thothDryRun      bool
	thothMemoryOnly  bool
	thothJournalOnly bool
)

var thothCmd = &cobra.Command{
	Use:   "thoth",
	Short: "𓁟 Thoth — Persistent Knowledge & Memory Sync",
	Long: `𓁟 Thoth — The Scribe of the Gods

Thoth manages persistent project memory across AI sessions.
The sync command auto-updates memory.yaml and journal.md from
source code analysis and git history.

  pantheon thoth sync              Full sync (memory + journal)
  pantheon thoth sync --dry-run    Preview changes without writing
  pantheon thoth sync --since "48 hours ago"  Custom journal window`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var thothSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Auto-sync memory.yaml and journal.md from source + git history",
	Long: `Sync performs a two-phase update of Thoth's project memory:

Phase 1 (Memory): Discovers facts from source code (module count,
test count, line count, binary count, command count) and updates
the identity section of .thoth/memory.yaml.

Phase 2 (Journal): Harvests recent git commits and appends structured
journal entries to .thoth/journal.md. This closes the "ghost transcript"
gap — even if the IDE loses conversations, git commits persist as
journal entries.

Use --memory-only or --journal-only to run a single phase.`,
	RunE: runThothSync,
}

func init() {
	thothSyncCmd.Flags().StringVar(&thothSince, "since", "24 hours ago", "Git log timeframe for journal entries")
	thothSyncCmd.Flags().BoolVar(&thothDryRun, "dry-run", false, "Preview changes without writing")
	thothSyncCmd.Flags().BoolVar(&thothMemoryOnly, "memory-only", false, "Only sync memory.yaml (skip journal)")
	thothSyncCmd.Flags().BoolVar(&thothJournalOnly, "journal-only", false, "Only sync journal.md (skip memory)")

	thothCmd.AddCommand(thothSyncCmd)
}

func runThothSync(cmd *cobra.Command, args []string) error {
	start := time.Now()

	// Find repo root — walk up from cwd looking for .thoth/
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("𓁟 thoth sync: %w", err)
	}

	fmt.Printf("𓁟 Thoth Sync — %s\n", repoRoot)
	fmt.Println()

	syncMemory := !thothJournalOnly
	syncJournal := !thothMemoryOnly

	// Phase 1: Memory sync
	if syncMemory {
		fmt.Print("  Phase 1: Memory sync (memory.yaml) ... ")
		if thothDryRun {
			fmt.Println("DRY RUN — would update identity fields")
		} else {
			if err := thoth.Sync(thoth.SyncOptions{
				RepoRoot:   repoRoot,
				UpdateDate: true,
			}); err != nil {
				fmt.Printf("FAILED: %v\n", err)
				return err
			}
			fmt.Println("✅ updated")
		}
	}

	// Phase 2: Journal sync
	if syncJournal {
		fmt.Printf("  Phase 2: Journal sync (journal.md, --since %q) ... ", thothSince)
		if thothDryRun {
			count, err := thoth.SyncJournal(thoth.JournalSyncOptions{
				RepoRoot: repoRoot,
				Since:    thothSince,
				DryRun:   true,
			})
			if err != nil {
				fmt.Printf("FAILED: %v\n", err)
				return err
			}
			if count == 0 {
				fmt.Println("no new commits")
			} else {
				fmt.Printf("DRY RUN — would append %d commits\n", count)
			}
		} else {
			count, err := thoth.SyncJournal(thoth.JournalSyncOptions{
				RepoRoot: repoRoot,
				Since:    thothSince,
			})
			if err != nil {
				fmt.Printf("FAILED: %v\n", err)
				return err
			}
			if count == 0 {
				fmt.Println("no new commits")
			} else {
				fmt.Printf("✅ appended %d commits\n", count)
			}
		}
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("\n  Completed in %s\n", elapsed)
	return nil
}

// findRepoRoot walks up from cwd looking for a .thoth/ directory.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}

	for {
		thothDir := filepath.Join(dir, ".thoth")
		if info, err := os.Stat(thothDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no .thoth/ directory found (run from inside a Pantheon project)")
}
