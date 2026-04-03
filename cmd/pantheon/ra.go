package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var raDocs bool

var raCmd = &cobra.Command{
	Use:   "ra",
	Short: "\u2600\uFE0F Ra \u2014 Supreme Overseer & Cross-Repo Orchestrator",
	Long: `☀️ Ra — Supreme Overseer & Cross-Repo Orchestrator

Ra orchestrates all Pantheon deities across all Sirsi repositories using
the Sirsi Orchestrator (claude-code-sdk). He dispatches parallel work,
runs fleet-wide health checks, tests, lints, and custom tasks.

  pantheon ra health                Health check across all repos
  pantheon ra test                  Run tests across all repos in parallel
  pantheon ra lint                  Run linters across all repos in parallel
  pantheon ra task <repo> <prompt>  Dispatch task to specific repo
  pantheon ra broadcast <prompt>    Run prompt across all repos
  pantheon ra nightly               Full nightly CI check
  pantheon ra status                Show orchestrator status and repo config

Prerequisites: python3, claude-code-sdk (pip3 install claude-code-sdk)`,
	Run: func(cmd *cobra.Command, args []string) {
		if raDocs {
			_ = help.OpenDocs("ra")
			return
		}
		_ = cmd.Help()
	},
}

// ── Repo configuration (mirrors the Python orchestrator) ────────────

type repoEntry struct {
	Path string
	Desc string
}

func raRepos() map[string]repoEntry {
	home, _ := os.UserHomeDir()
	dev := filepath.Join(home, "Development")
	return map[string]repoEntry{
		"pantheon":    {Path: filepath.Join(dev, "sirsi-pantheon"), Desc: "Infrastructure hygiene CLI"},
		"nexus":       {Path: filepath.Join(dev, "SirsiNexusApp"), Desc: "Platform monorepo"},
		"finalwishes": {Path: filepath.Join(dev, "FinalWishes"), Desc: "Estate planning application"},
		"assiduous":   {Path: filepath.Join(dev, "Assiduous"), Desc: "Real estate platform"},
	}
}

// ── Orchestrator path resolution ────────────────────────────────────

func findOrchestrator() (string, error) {
	candidates := []string{
		filepath.Join(".", "scripts", "sirsi-orchestrator.py"),
	}

	if root := os.Getenv("PANTHEON_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "scripts", "sirsi-orchestrator.py"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Development", "sirsi-pantheon", "scripts", "sirsi-orchestrator.py"))
	}

	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	return "", fmt.Errorf("sirsi-orchestrator.py not found.\n\nSearched:\n  %s\n\nInstall:\n  1. Clone sirsi-pantheon: git clone https://github.com/SirsiMaster/sirsi-pantheon\n  2. Set PANTHEON_ROOT to the repo root, or run from the repo directory.\n  3. Install deps: pip3 install claude-code-sdk",
		strings.Join(candidates, "\n  "))
}

// ── Orchestrator runner ─────────────────────────────────────────────

func runOrchestrator(subcmd string, extraArgs ...string) error {
	scriptPath, err := findOrchestrator()
	if err != nil {
		output.Error("%v", err)
		return err
	}

	cmdArgs := append([]string{scriptPath, subcmd}, extraArgs...)
	proc := exec.Command("python3", cmdArgs...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.Stdin = os.Stdin

	output.Header(fmt.Sprintf("\u2600\uFE0F Ra \u2014 %s", subcmd))
	output.Info("Orchestrator: %s", scriptPath)
	fmt.Println()

	start := time.Now()
	if err := proc.Run(); err != nil {
		output.Error("Orchestrator failed: %v", err)
		return err
	}
	output.Footer(time.Since(start))
	return nil
}

// ── Subcommands ─────────────────────────────────────────────────────

var raHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Health check across all repos",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrchestrator("health")
	},
}

var raTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Run tests across all repos in parallel",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrchestrator("test")
	},
}

var raLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Run linters across all repos in parallel",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrchestrator("lint")
	},
}

var raTaskCmd = &cobra.Command{
	Use:   "task <repo> <prompt>",
	Short: "Dispatch task to a specific repo",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := args[0]
		prompt := strings.Join(args[1:], " ")
		return runOrchestrator("task", repo, prompt)
	},
}

var raBroadcastCmd = &cobra.Command{
	Use:   "broadcast <prompt>",
	Short: "Run prompt across all repos",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt := strings.Join(args, " ")
		return runOrchestrator("broadcast", prompt)
	},
}

var raNightlyCmd = &cobra.Command{
	Use:   "nightly",
	Short: "Full nightly CI check (health + lint + test)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrchestrator("nightly")
	},
}

var raStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show orchestrator status and repo config",
	Run: func(cmd *cobra.Command, args []string) {
		output.Header("\u2600\uFE0F Ra \u2014 Orchestrator Status")

		repos := raRepos()

		// Check python3 availability
		python3Ok := false
		if _, err := exec.LookPath("python3"); err == nil {
			python3Ok = true
		}

		// Check orchestrator script
		scriptPath, scriptErr := findOrchestrator()

		// Check claude-code-sdk
		sdkOk := false
		sdkCheck := exec.Command("python3", "-c", "import claude_code_sdk; print('ok')")
		if out, err := sdkCheck.Output(); err == nil && strings.TrimSpace(string(out)) == "ok" {
			sdkOk = true
		}

		// Prerequisites
		output.Section("Prerequisites")
		if python3Ok {
			output.Success("python3 found")
		} else {
			output.Error("python3 not found")
		}
		if scriptErr == nil {
			output.Success("orchestrator: %s", scriptPath)
		} else {
			output.Error("orchestrator script not found")
		}
		if sdkOk {
			output.Success("claude-code-sdk installed")
		} else {
			output.Warn("claude-code-sdk not found (pip3 install claude-code-sdk)")
		}

		fmt.Println()
		output.Section("Repository Fleet")
		fmt.Println()

		// Table of repos
		for name, repo := range repos {
			exists := "\u2705"
			if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
				exists = "\u274C"
			}
			fmt.Printf("  %s  %-15s  %-35s  %s\n", exists, name, repo.Desc, repo.Path)
		}

		fmt.Println()
	},
}

func init() {
	raCmd.PersistentFlags().BoolVar(&raDocs, "docs", false, "Open Ra web documentation")

	raCmd.AddCommand(raHealthCmd)
	raCmd.AddCommand(raTestCmd)
	raCmd.AddCommand(raLintCmd)
	raCmd.AddCommand(raTaskCmd)
	raCmd.AddCommand(raBroadcastCmd)
	raCmd.AddCommand(raNightlyCmd)
	raCmd.AddCommand(raStatusCmd)
}
