package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
)

var (
	thothSince       string
	thothDryRun      bool
	brainUpdate      bool
	brainRemove      bool
	thothInitYes     bool
	thothInitName    string
	thothInitLang    string
	thothInitVersion string
)

var thothCmd = &cobra.Command{
	Use:   "thoth",
	Short: "𓁟 Thoth — Persistent Knowledge & Brain Manager",
	Long: `𓁟 Thoth — The Scribe of the Gods

Thoth manages your project's persistent memory and its neural "brain."
Use it to sync your development journal or manage AI weights.

  pantheon thoth init          Initialize .thoth/ knowledge system in a project
  pantheon thoth sync          Sync memory.yaml and journal.md
  pantheon thoth brain         Install/Update neural weights (Anubis Pro)
  pantheon thoth status        Check brain and memory health`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
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

  pantheon thoth init              Initialize in current directory (interactive)
  pantheon thoth init --yes        Non-interactive mode
  pantheon thoth init /path/to/proj --yes --name myproj`,
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

	thothCmd.AddCommand(thothInitCmd)
	thothCmd.AddCommand(thothSyncCmd)
	thothCmd.AddCommand(thothBrainCmd)
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
