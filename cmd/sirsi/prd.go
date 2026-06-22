package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/prd"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var (
	prdSyncAgent   string
	prdSyncAll     bool
	prdSyncDryRun  bool
	prdSyncVerbose bool
	prdSyncRepoDir string
)

var prdCmd = &cobra.Command{
	Use:   "prd",
	Short: "📋 PRD — Canon-derived completion contracts per agent thread",
	Long: `PRD (Product Requirements Document) management for Sirsi agents.

A PRD is a machine-readable projection of each repo's canon + architecture.
Canon is the single source of truth; the PRD is its agent-consumable view.
The prd-keepalive hook reads the PRD and holds each thread to its canon-derived
tasks until done or owner-gated.

  sirsi prd sync --agent claude-pantheon    Sync one agent's PRD from its repo canon
  sirsi prd sync --all                      Sync all registered agents' PRDs`,
}

var prdSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Derive and refresh .agents/prd/<agent>.json from repo canon + architecture",
	Long: `sirsi prd sync reads each repo's governing canon files and derives a PRD:

  INPUTS (from each repo's top dir):
    CLAUDE.md / AGENTS.md / <NAME>_RULES.md  — canon / scope
    docs/ARCHITECTURE*.md                     — Neith-Triad implementation order
    docs/ADR-INDEX.md                         — decisions / what's planned
    *_ROADMAP.md / product-roadmap.*          — roadmap phases

  DERIVES:
    tasks = architecture implementation-order items + roadmap items
            + Proposed/Partially-In-Force ADRs not yet shipped
    status = seeded from git log evidence; open if no evidence found

  OUTPUT:
    .agents/prd/<agent>.json — kept in sync with canon; never drifts

Re-running sync reconciles the PRD against canon: existing manually-set
statuses (done, owner-gated, blocked) are preserved; only truly new items
from canon are added. Canon is always the authority.`,
	RunE: runPRDSync,
}

func runPRDSync(cmd *cobra.Command, _ []string) error {
	start := time.Now()

	if !prdSyncAll && prdSyncAgent == "" {
		return fmt.Errorf("specify --agent <id> or --all")
	}

	opts := prd.SyncOptions{
		Agent:   prdSyncAgent,
		RepoDir: prdSyncRepoDir,
		DryRun:  prdSyncDryRun,
		Verbose: prdSyncVerbose,
	}

	if prdSyncAll {
		output.Header("PRD Sync — all agents")
		results, err := prd.SyncAll(opts)
		if err != nil {
			return fmt.Errorf("prd sync --all: %w", err)
		}
		// Sort by agent name for stable output.
		sort.Slice(results, func(i, j int) bool {
			return results[i].Agent < results[j].Agent
		})
		for _, r := range results {
			printSyncResult(r, prdSyncDryRun, prdSyncVerbose)
		}
	} else {
		output.Header(fmt.Sprintf("PRD Sync — %s", prdSyncAgent))
		r, err := prd.Sync(opts)
		if err != nil {
			return fmt.Errorf("prd sync: %w", err)
		}
		printSyncResult(r, prdSyncDryRun, prdSyncVerbose)
	}

	output.Footer(time.Since(start))
	output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "prd", Subcommand: "sync"}))
	return nil
}

func printSyncResult(r *prd.SyncResult, dryRun, verbose bool) {
	if r == nil {
		return
	}
	tag := ""
	if dryRun {
		tag = " (dry-run)"
	}

	added := len(r.Added)
	kept := len(r.Kept)

	if added == 0 {
		output.Success("  %s: %d kept, 0 new%s", r.Agent, kept, tag)
	} else {
		output.Success("  %s: %d kept, %d new%s", r.Agent, kept, added, tag)
	}

	if verbose {
		if len(r.Added) > 0 {
			fmt.Printf("    + added: %s\n", strings.Join(r.Added, ", "))
		}
		if len(r.Updated) > 0 {
			fmt.Printf("    ~ updated: %s\n", strings.Join(r.Updated, ", "))
		}
	}

	if r.Written {
		output.Info("    wrote .agents/prd/%s.json", r.Agent)
	} else if dryRun {
		output.Info("    (dry-run) would write .agents/prd/%s.json", r.Agent)
	}
}

func init() {
	prdSyncCmd.Flags().StringVar(&prdSyncAgent, "agent", "", "Agent ID to sync (e.g. claude-pantheon)")
	prdSyncCmd.Flags().BoolVar(&prdSyncAll, "all", false, "Sync all registered agents' PRDs")
	prdSyncCmd.Flags().BoolVar(&prdSyncDryRun, "dry-run", false, "Preview without writing")
	prdSyncCmd.Flags().BoolVarP(&prdSyncVerbose, "verbose", "v", false, "Show per-task detail")
	prdSyncCmd.Flags().StringVar(&prdSyncRepoDir, "repo-dir", "", "Override repo directory (for testing or non-standard layouts)")

	prdCmd.AddCommand(prdSyncCmd)
}
