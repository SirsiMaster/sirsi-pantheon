package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ra"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var raDocs bool
var raRecord bool

var raCmd = &cobra.Command{
	Use:   "ra",
	Short: "𓇶 Ra — Supreme Overseer & Cross-Repo Orchestrator",
	Long: `𓇶 Ra — Supreme Overseer & Cross-Repo Orchestrator

Ra orchestrates Pantheon fleet checks with a shell-free Go executor.
External agent dispatch remains an explicit developer-only capability and
fails closed when no provider is configured.

  sirsi ra health                Health check across all repos
  sirsi ra test                  Run tests across all repos in parallel
  sirsi ra lint                  Run linters across all repos in parallel
  sirsi ra task <repo> <prompt>  Dispatch task to specific repo
  sirsi ra broadcast <prompt>    Run prompt across all repos
  sirsi ra nightly               Full nightly CI check
  sirsi ra status                Show orchestrator status and repo config
`,
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

// ── Orchestrator runner ─────────────────────────────────────────────

func runOrchestrator(subcmd string, extraArgs ...string) error {
	output.Header(fmt.Sprintf("Fleet — %s", subcmd))
	output.Info("Executor: native Go fleet")
	fmt.Println()

	if raRecord {
		return runOrchestratorWithPipeline(subcmd, extraArgs...)
	}

	start := time.Now()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve fleet home: %w", err)
	}
	provider, providerErr := ra.ExternalProviderFromEnv()
	if subcmd != "task" && subcmd != "broadcast" {
		providerErr = nil
	}
	if providerErr != nil {
		return providerErr
	}
	results, err := ra.RunNativeFleetWithProvider(context.Background(), ra.DefaultFleetRepos(home), subcmd, extraArgs, provider)
	for _, result := range results {
		fmt.Printf("  %-12s %-5s %s\n", result.Repo, result.Status, strings.TrimSpace(result.Output))
	}
	if err != nil {
		output.Error("Native fleet failed: %v", err)
		return err
	}
	output.Footer(time.Since(start))
	actions := suggest.After(suggest.Context{Deity: "ra", Subcommand: "status"})
	var steps [][]string
	for _, a := range actions {
		steps = append(steps, []string{a.Command, a.Description})
	}
	output.NextSteps(steps)
	return nil
}

// runOrchestratorWithPipeline executes the orchestrator through the Ra pipeline,
// automatically feeding results to Seshat for ingestion and Thoth for persistence.
func runOrchestratorWithPipeline(subcmd string, extraArgs ...string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		// Fallback to cwd if no .thoth/ found.
		repoRoot, _ = os.Getwd()
	}

	pipeline := ra.NewPipeline(repoRoot)
	task := ra.Task{
		Subcmd:    subcmd,
		ExtraArgs: extraArgs,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve fleet home: %w", err)
	}
	provider, providerErr := ra.ExternalProviderFromEnv()
	if subcmd != "task" && subcmd != "broadcast" {
		providerErr = nil
	}
	if providerErr != nil {
		return providerErr
	}
	runner := func(ctx context.Context, repos []ra.NativeRepo, operation string) ([]ra.NativeResult, error) {
		return ra.RunNativeFleetWithProvider(ctx, repos, operation, extraArgs, provider)
	}
	result, err := pipeline.RunNativeAndRecord(context.Background(), ra.DefaultFleetRepos(home), task, runner)
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
	actions := suggest.After(suggest.Context{Deity: "ra", Subcommand: "deploy"})
	var steps [][]string
	for _, a := range actions {
		steps = append(steps, []string{a.Command, a.Description})
	}
	output.NextSteps(steps)
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
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		repos := raRepos()

		reposPresent := 0
		for _, repo := range repos {
			if _, err := os.Stat(repo.Path); err == nil {
				reposPresent++
			}
		}

		// CommandResult contract (owner surface law 2026-07-09): a structured
		// summary + evidence + real levers, not a raw text dump that ignored
		// --json and dead-ended at a Refresh.
		res := &output.CommandResult{Command: "sirsi ra status", BriefTitle: "Fleet Orchestrator"}
		ready := reposPresent == len(repos)
		if ready {
			res.Summary = fmt.Sprintf("Fleet orchestrator ready — %d repositories in the fleet.", reposPresent)
			res.Status = "ok"
		} else {
			res.Summary = fmt.Sprintf("Fleet orchestrator not fully set up (%d repositories tracked).", reposPresent)
			res.Status = "warn"
		}
		res.AddEvidence("native Go executor", "ready")
		providerState := "developer-only; not configured by default"
		if provider, err := ra.ExternalProviderFromEnv(); err == nil {
			providerState = "configured: " + provider.Executable
		}
		res.AddEvidence("external provider", providerState)
		res.AddEvidence("repositories", fmt.Sprintf("%d present", reposPresent))
		// Per-node capacity (ADR-031-B): the fleet lord reads what THIS node
		// can carry before placing work — free RAM, the dynamic reserve, and
		// the live pressure level with its source.
		nc := guard.SampleNodeCapacity()
		res.AddEvidence("node capacity", fmt.Sprintf("%s free · reserve %s · pressure %s (%s)",
			guard.FormatBytes(nc.FreeRAM), guard.FormatBytes(nc.DynamicReserve()),
			nc.Pressure, nc.PressureSource))
		for name, repo := range repos {
			mark := "present"
			if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
				mark = "missing"
			}
			res.AddEvidence(name, fmt.Sprintf("%s — %s", mark, repo.Desc))
		}
		res.NextActions = append(res.NextActions,
			output.NextAction{Label: "Check fleet health", Command: "sirsi ra health", Description: "Probe each repo's build/test/lint state."},
			output.NextAction{Label: "Pipeline status", Command: "sirsi ra pipeline", Description: "Last recording, knowledge-item count, Thoth sync."},
		)
		res.Duration = time.Since(start)
		res.Render()
		return nil
	},
}

// boolMark renders a check/cross for a boolean prerequisite in evidence rows.
func boolMark(ok bool) string {
	if ok {
		return "ready"
	}
	return "not found"
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

		output.Header("Fleet Pipeline Status")

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

  sirsi ra deploy                    Spawn all 4 windows
  sirsi ra deploy --scope assiduous  Spawn one specific scope
  sirsi ra deploy --wait --record    Spawn, wait, then pipeline
  sirsi ra deploy --dry-run          Show assembled prompts, don't spawn`,
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

		output.Header("Fleet Deploy")
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
		start := time.Now()
		output.Header("Fleet — Kill All Windows")
		if err := ra.KillAll(ra.RADir()); err != nil {
			return err
		}
		output.Footer(time.Since(start))
		output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "ra", Subcommand: "kill"}))
		return nil
	},
}

var raCollectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect results from completed windows and run pipeline",
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Header("Fleet — Collect Results")

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
		output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "ra", Subcommand: "collect"}))
		return nil
	},
}

// raWatchCmd launches the Ra Command Center TUI — live sprint monitoring.
var raWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "𓇶 Ra Command Center — live sprint monitoring TUI",
	Long: `Launch the Ra Command Center, a full-screen terminal UI that shows:
  - Live sprint progress per scope (Sprint 2/5, running, etc.)
  - Governance loop status (Ma'at QA, Thoth compact, Seshat scribe)
  - Agent activity (last tool call, log tail)
  - Post-sprint acceptance flow

  sirsi ra watch         Launch the command center
  sirsi ra deploy        Deploy agents (auto-opens watch in a new window)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("ra watch TUI is sunset; the interactive surface is the forthcoming native macOS app (ADR-018)")
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
	raCmd.AddCommand(raWatchCmd)
}
