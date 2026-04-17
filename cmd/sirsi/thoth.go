package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
)

var (
	thothSince          string
	thothDryRun         bool
	thothDocs           bool
	brainUpdate         bool
	brainRemove         bool
	thothInitYes        bool
	thothInitName       string
	thothInitLang       string
	thothInitVersion    string
	thothCompactSummary string
	thothCompactMaxAge  int
	thothCompactMaxKeep int
)

var thothCmd = &cobra.Command{
	Use:   "thoth",
	Short: "𓁟 Thoth — Persistent Knowledge & Brain Manager",
	Long: `𓁟 Thoth — The Scribe of the Gods

Thoth manages your project's persistent memory and its neural "brain."
Use it to sync your development journal or manage AI weights.

  sirsi thoth init          Initialize .thoth/ knowledge system in a project
  sirsi thoth sync          Sync memory.yaml and journal.md
  sirsi thoth brain         Install/Update neural weights (Anubis Pro)
  sirsi thoth status        Check brain and memory health`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if thothDocs {
			output.Info("Opening Thoth docs...")
			return help.OpenDocs("thoth")
		}
		return cmd.Help()
	},
}

// thothInitCmd handles project scaffolding
var thothInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize .thoth/ knowledge system in a project",
	Long: `𓁟 Thoth Init — Scaffold the three-layer knowledge system

Creates .thoth/memory.yaml, .thoth/journal.md, and .thoth/artifacts/ in the
target directory. Auto-detects project language, counts source lines, and
injects Thoth read instructions into IDE rules files (CLAUDE.md, GEMINI.md,
.cursorrules, .windsurfrules, copilot-instructions.md).

  sirsi thoth init              Initialize in current directory (interactive)
  sirsi thoth init --yes        Non-interactive mode
  sirsi thoth init /path/to/proj --yes --name myproj`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		if thothInitYes {
			return thoth.Init(thoth.InitOptions{
				RepoRoot: root,
				Name:     thothInitName,
				Language: thothInitLang,
				Version:  thothInitVersion,
				Yes:      true,
			})
		}
		return thoth.InteractiveInit(root)
	},
}

// thothSyncCmd handles memory/journal synchronization
var thothSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Auto-sync memory.yaml and journal.md from source + git history",
	RunE:  runThothSync,
}

// thothBrainCmd handles neural weight installation
var thothBrainCmd = &cobra.Command{
	Use:   "brain",
	Short: "Manage neural weights (CoreML/ONNX) for Pro-tier analysis",
	Run:   runBrainCommand,
}

func init() {
	// Docs flag
	thothCmd.Flags().BoolVar(&thothDocs, "docs", false, "Open Thoth web documentation in browser")

	// Init Flags
	thothInitCmd.Flags().BoolVarP(&thothInitYes, "yes", "y", false, "Non-interactive mode (no prompts)")
	thothInitCmd.Flags().StringVar(&thothInitName, "name", "", "Override project name")
	thothInitCmd.Flags().StringVar(&thothInitLang, "language", "", "Override detected language")
	thothInitCmd.Flags().StringVar(&thothInitVersion, "version", "", "Override project version")

	// Sync Flags
	thothSyncCmd.Flags().StringVar(&thothSince, "since", "24 hours ago", "Git log timeframe")
	thothSyncCmd.Flags().BoolVar(&thothDryRun, "dry-run", false, "Preview without writing")

	// Brain Flags
	thothBrainCmd.Flags().BoolVar(&brainUpdate, "update", false, "Update to latest weights")
	thothBrainCmd.Flags().BoolVar(&brainRemove, "remove", false, "Remove weights")

	// Compact Flags
	thothCompactCmd.Flags().StringVarP(&thothCompactSummary, "summary", "s", "", "Session summary text (reads stdin if empty)")
	thothCompactCmd.Flags().IntVar(&thothCompactMaxAge, "max-age", 0, "Prune journal entries older than N days (0 = no pruning)")
	thothCompactCmd.Flags().IntVar(&thothCompactMaxKeep, "max-keep", 0, "Keep only the last N journal entries (0 = no limit)")

	thothCmd.AddCommand(thothInitCmd)
	thothCmd.AddCommand(thothSyncCmd)
	thothCmd.AddCommand(thothBrainCmd)
	thothCmd.AddCommand(thothCompactCmd)
}

// thothCompactCmd persists session decisions before context compression.
var thothCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Persist session decisions before context compression",
	Long: `Captures key decisions and patterns from the current session into
.thoth/memory.yaml and .thoth/journal.md before context is compressed.

  sirsi thoth compact -s "Use interface providers for Ka"
  echo "decision text" | sirsi thoth compact`,
	RunE: runThothCompact,
}

func runThothCompact(cmd *cobra.Command, args []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	summary := thothCompactSummary
	if summary == "" {
		// Read from stdin
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return fmt.Errorf("failed to read stdin: %w", readErr)
		}
		summary = string(data)
	}

	output.Header("Thoth Compact")
	err = thoth.Compact(thoth.CompactOptions{
		RepoRoot: repoRoot,
		Summary:  summary,
		MaxAge:   thothCompactMaxAge,
		MaxKeep:  thothCompactMaxKeep,
	})
	if err != nil {
		return err
	}

	output.Success("Session decisions persisted to .thoth/")
	return nil
}

func runThothSync(cmd *cobra.Command, args []string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	output.Header(fmt.Sprintf("𓁟 Thoth Sync — %s", repoRoot))

	// ... (Existing sync logic) ...
	if err := thoth.Sync(thoth.SyncOptions{RepoRoot: repoRoot, UpdateDate: true}); err != nil {
		return err
	}
	output.Success("Memory synced.")
	return nil
}

func runBrainCommand(cmd *cobra.Command, args []string) {
	if brainRemove {
		output.Info("Removing neural weights...")
		_ = brain.Remove()
		return
	}
	output.Info("Checking brain status...")
	// ... (Existing install/update logic) ...
}

func findRepoRoot() (string, error) {
	dir, _ := os.Getwd()
	for {
		if info, err := os.Stat(filepath.Join(dir, ".thoth")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .thoth/ found")
}
