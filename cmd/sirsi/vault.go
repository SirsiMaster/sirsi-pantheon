package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/vault"
)

var (
	vaultSource    string
	vaultTag       string
	vaultLimit     int
	vaultOlderThan string
)

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Context vault — sandbox output + code search (subsumes Context Mode + Claude Context)",
	Long: `Vault — Context Sandbox & Code Search

Stores large tool output in SQLite FTS5 instead of the AI context window.
Also indexes source code for BM25-ranked full-text search.

  sirsi vault store --source=test < output.log    Sandbox output
  sirsi vault search "error compilation"          FTS5 search
  sirsi vault index .                             Index codebase
  sirsi vault code-search "RegisterTool"          Search code`,
}

var vaultStoreCmd = &cobra.Command{
	Use:   "store",
	Short: "Store output from stdin into the vault",
	RunE:  runVaultStore,
}

var vaultSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search the vault",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultSearch,
}

var vaultGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Retrieve a vault entry by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultGet,
}

var vaultStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show vault statistics",
	RunE:  runVaultStats,
}

var vaultPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old vault entries",
	RunE:  runVaultPrune,
}

var vaultIndexCmd = &cobra.Command{
	Use:   "index <path>",
	Short: "Build code search index for a project",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runVaultIndex,
}

var vaultCodeSearchCmd = &cobra.Command{
	Use:   "code-search <query>",
	Short: "BM25 full-text search over indexed source code",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultCodeSearch,
}

func init() {
	vaultStoreCmd.Flags().StringVar(&vaultSource, "source", "", "Source identifier (e.g. 'npm test')")
	vaultStoreCmd.Flags().StringVar(&vaultTag, "tag", "", "Category tag (e.g. 'logs', 'build')")
	vaultSearchCmd.Flags().IntVar(&vaultLimit, "limit", 10, "Max results")
	vaultPruneCmd.Flags().StringVar(&vaultOlderThan, "older-than", "7d", "Remove entries older than (e.g. 7d, 24h)")
	vaultCodeSearchCmd.Flags().IntVar(&vaultLimit, "limit", 5, "Max results")

	vaultCmd.AddCommand(vaultStoreCmd, vaultSearchCmd, vaultGetCmd, vaultStatsCmd, vaultPruneCmd)
	vaultCmd.AddCommand(vaultIndexCmd, vaultCodeSearchCmd)
}

func runVaultStore(_ *cobra.Command, _ []string) error {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	s, err := vault.Open(vault.DefaultPath())
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	defer s.Close()

	tokens := len(input) / 4
	entry, err := s.Store(vaultSource, vaultTag, string(input), tokens)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}

	fmt.Printf("Stored in vault: ID=%d, ~%d tokens, %d bytes\n", entry.ID, tokens, len(input))
	return nil
}

func runVaultSearch(_ *cobra.Command, args []string) error {
	s, err := vault.Open(vault.DefaultPath())
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	defer s.Close()

	result, err := s.Search(args[0], vaultLimit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	output.Banner()
	fmt.Printf("Vault search: %q — %d results\n\n", args[0], result.TotalHits)
	for _, e := range result.Entries {
		fmt.Printf("[%d] source=%s tag=%s (%s)\n  %s\n\n", e.ID, e.Source, e.Tag, e.CreatedAt, e.Snippet)
	}
	return nil
}

func runVaultGet(_ *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	s, err := vault.Open(vault.DefaultPath())
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	defer s.Close()

	entry, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	fmt.Printf("Entry %d (source=%s, tag=%s, %d tokens)\n\n%s\n", entry.ID, entry.Source, entry.Tag, entry.Tokens, entry.Content)
	return nil
}

func runVaultStats(_ *cobra.Command, _ []string) error {
	s, err := vault.Open(vault.DefaultPath())
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	defer s.Close()

	stats, err := s.Stats()
	if err != nil {
		return fmt.Errorf("stats: %w", err)
	}

	output.Banner()
	fmt.Printf("Vault Statistics\n\n")
	fmt.Printf("  Entries: %d\n", stats.TotalEntries)
	fmt.Printf("  Total bytes: %d\n", stats.TotalBytes)
	fmt.Printf("  Tokens saved: %d\n", stats.TotalTokens)
	if stats.OldestEntry != "" {
		fmt.Printf("  Range: %s to %s\n", stats.OldestEntry, stats.NewestEntry)
	}
	if len(stats.TagCounts) > 0 {
		fmt.Printf("\n  Tags:\n")
		for tag, count := range stats.TagCounts {
			label := tag
			if label == "" {
				label = "(untagged)"
			}
			fmt.Printf("    %s: %d\n", label, count)
		}
	}
	return nil
}

func runVaultPrune(_ *cobra.Command, _ []string) error {
	dur, err := time.ParseDuration(vaultOlderThan)
	if err != nil {
		// Try "7d" format.
		if len(vaultOlderThan) > 1 && vaultOlderThan[len(vaultOlderThan)-1] == 'd' {
			days, err2 := strconv.Atoi(vaultOlderThan[:len(vaultOlderThan)-1])
			if err2 != nil {
				return fmt.Errorf("invalid duration %q: %w", vaultOlderThan, err)
			}
			dur = time.Duration(days) * 24 * time.Hour
		} else {
			return fmt.Errorf("invalid duration %q: %w", vaultOlderThan, err)
		}
	}

	s, err := vault.Open(vault.DefaultPath())
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	defer s.Close()

	removed, err := s.Prune(dur)
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}

	fmt.Printf("Pruned %d entries older than %s\n", removed, vaultOlderThan)
	return nil
}

func runVaultIndex(_ *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	ci, err := vault.OpenCodeIndex(vault.DefaultCodeIndexPath())
	if err != nil {
		return fmt.Errorf("open code index: %w", err)
	}
	defer ci.Close()

	output.Banner()
	fmt.Printf("Indexing %s...\n", path)

	stats, err := ci.IndexDir(path)
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}

	fmt.Printf("Code index built: %d files, %d chunks in %s\n", stats.FilesIndexed, stats.ChunksCreated, stats.Duration)
	return nil
}

func runVaultCodeSearch(_ *cobra.Command, args []string) error {
	ci, err := vault.OpenCodeIndex(vault.DefaultCodeIndexPath())
	if err != nil {
		return fmt.Errorf("open code index: %w", err)
	}
	defer ci.Close()

	chunks, err := ci.Search(args[0], vaultLimit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	output.Banner()
	fmt.Printf("Code search: %q — %d results\n\n", args[0], len(chunks))
	for _, c := range chunks {
		label := c.File
		if c.Name != "" {
			label = fmt.Sprintf("%s:%s (%s)", c.File, c.Name, c.Kind)
		}
		fmt.Printf("── %s [lines %d-%d] ──\n%s\n\n", label, c.StartLine, c.EndLine, c.Content)
	}
	return nil
}
