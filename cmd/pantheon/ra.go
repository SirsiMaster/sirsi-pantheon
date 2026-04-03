package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ra"
)

var raDocs bool
var raRecord bool

var raCmd = &cobra.Command{
	Use:   "ra",
	Short: "𓇶 Ra — Supreme Overseer & Cross-Repo Orchestrator",
	Long: `𓇶 Ra — Supreme Overseer & Cross-Repo Orchestrator

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

	output.Header(fmt.Sprintf("\u2600\uFE0F Ra \u2014 %s", subcmd))
	output.Info("Orchestrator: %s", scriptPath)
	fmt.Println()

	// If --record is set, use the pipeline for automatic knowledge capture.
	if raRecord {
		return runOrchestratorWithPipeline(subcmd, scriptPath, extraArgs...)
	}

	cmdArgs := append([]string{scriptPath, subcmd}, extraArgs...)
	proc := exec.Command("python3", cmdArgs...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.Stdin = os.Stdin

	start := time.Now()
	if err := proc.Run(); err != nil {
		output.Error("Orchestrator failed: %v", err)
		return err
	}
	output.Footer(time.Since(start))
	return nil
}

// runOrchestratorWithPipeline executes the orchestrator through the Ra pipeline,
// automatically feeding results to Seshat for ingestion and Thoth for persistence.
func runOrchestratorWithPipeline(subcmd, scriptPath string, extraArgs ...string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		// Fallback to cwd if no .thoth/ found.
		repoRoot, _ = os.Getwd()
	}

	pipeline := ra.NewPipeline(repoRoot)
	pipeline.OrchestratorPath = scriptPath

	task := ra.Task{
		Subcmd:    subcmd,
		ExtraArgs: extraArgs,
	}

	result, err := pipeline.RunAndRecord(context.Background(), task)
	if err != nil {
		output.Error("Pipeline failed: %v", err)
		return err
	}

	// Print the feedback loop summary.
	thothStatus := "synced"
	if !result.ThothSynced {
		thothStatus = "skipped (no .thoth/memory.yaml)"
	}
	fmt.Fprintf(os.Stderr, "\n  %s Ra complete -> %s Seshat ingested %d items -> %s Thoth %s\n",
		"\u2600\uFE0F", "\U000130C6", result.ItemsIngested, "\U0001305F", thothStatus)

	output.Footer(result.Duration)
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

// raPipelineCmd shows the pipeline status — last recording, KI count, Thoth sync time.
var raPipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Show Ra pipeline status (last recording, KI count, Thoth sync)",
	Run: func(cmd *cobra.Command, args []string) {
		repoRoot, err := findRepoRoot()
		if err != nil {
			repoRoot, _ = os.Getwd()
		}

		output.Header("\u2600\uFE0F Ra Pipeline Status")

		pipeline := ra.NewPipeline(repoRoot)
		status, err := pipeline.ReadStatus()
		if err != nil {
			output.Error("Failed to read pipeline status: %v", err)
			return
		}

		if status == nil {
			output.Info("No pipeline runs recorded yet.")
			output.Dim("Run any Ra subcommand with --record to start the feedback loop.")
			fmt.Println()
			return
		}

		fmt.Println()
		output.Section("Last Recording")
		if !status.LastRecorded.IsZero() {
			output.Info("Time:  %s", status.LastRecorded.Format("2006-01-02 15:04:05"))
			output.Info("Age:   %s ago", time.Since(status.LastRecorded).Round(time.Second))
		}

		output.Info("Items: %d knowledge items ingested", status.ItemCount)

		if !status.ThothSynced.IsZero() {
			output.Success("Thoth synced at %s", status.ThothSynced.Format("2006-01-02 15:04:05"))
		} else {
			output.Warn("Thoth not synced in last pipeline run")
		}

		// Show Seshat artifacts count.
		seshatDir := filepath.Join(repoRoot, ".thoth", "seshat")
		if entries, err := os.ReadDir(seshatDir); err == nil {
			count := 0
			for _, e := range entries {
				if !e.IsDir() {
					count++
				}
			}
			output.Info("Seshat store: %d artifacts in .thoth/seshat/", count)
		}

		fmt.Println()
	},
}

// ── Deploy commands (Neith → Ra → Ma'at governance loop) ───────────

var raDeployScopes []string
var raDeployITerm2 bool
var raDeployWait bool
var raDeployRecord bool
var raDeployDryRun bool

var raDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "𓁯 Neith weaves scopes, 𓇶 Ra spawns terminal windows",
	Long: `Neith assembles scope prompts from each repo's canon documents
(CLAUDE.md, Thoth memory, ADRs, blueprints, continuation prompts).
Ra then spawns a macOS terminal window for each scope.

  pantheon ra deploy                    Spawn all 4 windows
  pantheon ra deploy --scope assiduous  Spawn one specific scope
  pantheon ra deploy --wait --record    Spawn, wait, then pipeline
  pantheon ra deploy --dry-run          Show assembled prompts, don't spawn`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := findRepoRoot()
		if err != nil {
			repoRoot, _ = os.Getwd()
		}

		configDir := filepath.Join(repoRoot, "configs", "scopes")
		if _, statErr := os.Stat(configDir); os.IsNotExist(statErr) {
			// Try relative to PANTHEON_ROOT
			if root := os.Getenv("PANTHEON_ROOT"); root != "" {
				configDir = filepath.Join(root, "configs", "scopes")
			}
		}

		output.Header("𓇶 Ra — Deploy")
		if raDeployDryRun {
			output.Info("Dry run — Neith will weave prompts but Ra will not spawn windows")
		}
		fmt.Println()

		opts := ra.DeployOptions{
			ConfigDir:  configDir,
			ScopeNames: raDeployScopes,
			UseITerm2:  raDeployITerm2,
			Wait:       raDeployWait,
			Record:     raDeployRecord,
			DryRun:     raDeployDryRun,
			RepoRoot:   repoRoot,
		}

		result, err := ra.Deploy(opts)
		if err != nil {
			output.Error("Deploy failed: %v", err)
			return err
		}

		fmt.Printf("\n  𓇶 Deployed %d scope(s)\n\n", len(result.Spawned))
		return nil
	},
}

var raKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Terminate all deployed Ra windows",
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Header("𓇶 Ra — Kill All Windows")
		return ra.KillAll(ra.RADir())
	},
}

var raCollectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect results from completed windows and run pipeline",
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Header("𓇶 Ra — Collect Results")

		results, err := ra.CollectResults(ra.RADir())
		if err != nil {
			return fmt.Errorf("collect: %w", err)
		}

		for _, r := range results {
			icon := "✅"
			if r.ExitCode != 0 {
				icon = "❌"
			}
			fmt.Printf("  %s %s — exit %d (%s)\n", icon, r.Name, r.ExitCode, r.Duration.Round(time.Second))
		}

		repoRoot, err := findRepoRoot()
		if err != nil {
			repoRoot, _ = os.Getwd()
		}

		pr, err := ra.IngestWindowResults(repoRoot, results)
		if err != nil {
			output.Error("Pipeline: %v", err)
			return err
		}

		fmt.Printf("\n  𓇶 Ra → 𓁆 Seshat ingested %d items → 𓁟 Thoth %s\n\n",
			pr.ItemsIngested, func() string {
				if pr.ThothSynced {
					return "synced ✅"
				}
				return "skipped ⚠️"
			}())
		return nil
	},
}

func init() {
	raCmd.PersistentFlags().BoolVar(&raDocs, "docs", false, "Open Ra web documentation")
	raCmd.PersistentFlags().BoolVar(&raRecord, "record", false, "Record results through the Seshat/Thoth knowledge pipeline")

	raDeployCmd.Flags().StringSliceVar(&raDeployScopes, "scope", nil, "Deploy specific scope(s) only (repeatable)")
	raDeployCmd.Flags().BoolVar(&raDeployITerm2, "iterm2", false, "Use iTerm2 instead of Terminal.app")
	raDeployCmd.Flags().BoolVar(&raDeployWait, "wait", false, "Block until all windows complete")
	raDeployCmd.Flags().BoolVar(&raDeployRecord, "record", false, "Run Seshat/Thoth pipeline after completion")
	raDeployCmd.Flags().BoolVar(&raDeployDryRun, "dry-run", false, "Show assembled prompts without spawning")

	raCmd.AddCommand(raHealthCmd)
	raCmd.AddCommand(raTestCmd)
	raCmd.AddCommand(raLintCmd)
	raCmd.AddCommand(raTaskCmd)
	raCmd.AddCommand(raBroadcastCmd)
	raCmd.AddCommand(raNightlyCmd)
	raCmd.AddCommand(raStatusCmd)
	raCmd.AddCommand(raPipelineCmd)
	raCmd.AddCommand(raDeployCmd)
	raCmd.AddCommand(raKillCmd)
	raCmd.AddCommand(raCollectCmd)
}
