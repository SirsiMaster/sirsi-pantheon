package main

// ctr.go — CTR ("Check The Router"), the universal manual wake primitive.
//
// Owner directive 2026-07-13: background wake (monitors, arming, watchers,
// launchd daemons) has repeatedly failed to keep threads consuming their
// inboxes — a level-triggered daemon that dies silently strands every item
// addressed to it. CTR inverts the model: instead of keeping a process alive,
// ANY event (a human typing, a hook, a git commit, a cron, another agent) pokes
// the router with ONE synchronous pass. There is nothing to keep alive, so
// there is nothing to die.
//
// CTR is the same primitive on every surface — only the caller differs:
//   • `sirsi ctr`  — this Go verb, the single source of truth (cross-platform).
//   • `ctr`        — a thin PATH shim → `sirsi ctr` (any shell / IDE terminal).
//   • `/ctr`       — a Claude Code skill that runs `ctr` and acts on the items.
//
// Rule 0 (wrap, don't rebuild): CTR orchestrates the EXISTING substrate —
// router.CollectNodeStatus (pending + stranded truth) and router.WakePass (the
// wake-or-declare-unavailable pass, ADR wake constraints: never blind-spawns an
// interactive session; a closed session is reported "needs-owner", not faked).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var (
	ctrNoWake  bool
	ctrJSON    bool
	ctrQuiet   bool
	ctrInstall bool
)

var ctrCmd = &cobra.Command{
	Use:   "ctr [agent]",
	Short: "Check The Router — one synchronous pass that surfaces pending items and wakes stranded threads",
	Long: `𓁢 CTR — Check The Router

A single, synchronous router pass: surface every open inbox item, then wake the
agents that have items waiting but no live watcher. No daemon, no resident
process — call it and it acts, then exits. Safe to run repeatedly.

Callable everywhere as the same primitive:
  ctr                 (any shell / IDE terminal — a thin shim over this verb)
  /ctr                (inside a Claude Code session — a skill that runs ctr)
  sirsi ctr           (this verb — the cross-platform source of truth)

  sirsi ctr                    Check + wake all local agents with pending items
  sirsi ctr claude-pantheon    Scope the surface to one agent
  sirsi ctr --no-wake          Surface only; do not attempt any wake
  sirsi ctr --json             Machine-readable (for hooks / other processes)
  sirsi ctr --install          Wire the 'ctr' shim + '/ctr' skill on this machine

The router is located from the current repo, or — from any other directory —
the persisted marker (~/.sirsi/pantheon-repo), so 'ctr' works from anywhere.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCtr,
}

func init() {
	ctrCmd.Flags().BoolVar(&ctrNoWake, "no-wake", false, "Surface pending items but do not attempt any wake")
	ctrCmd.Flags().BoolVar(&ctrJSON, "json", false, "Emit the result as JSON (for hooks and other processes)")
	ctrCmd.Flags().BoolVar(&ctrQuiet, "quiet", false, "One-line summary only")
	ctrCmd.Flags().BoolVar(&ctrInstall, "install", false, "Install the 'ctr' shell shim and the '/ctr' Claude skill on this machine, then exit")
}

// ctrResult is the JSON contract for process/hook consumers.
type ctrResult struct {
	Repo          string                 `json:"repo"`
	Scope         string                 `json:"scope,omitempty"` // agent id when scoped
	PendingTotal  int                    `json:"pending_total"`
	AgentsPending int                    `json:"agents_pending"`
	LiveThreads   int                    `json:"live_threads"`
	StaleThreads  int                    `json:"stale_threads"`
	Woke          []router.WakeOutcome   `json:"woke,omitempty"`         // newly attempted this pass
	AlreadyLive   []router.WakeOutcome   `json:"already_live,omitempty"` // armed — already watching
	NeedsOwner    []router.WakeOutcome   `json:"needs_owner,omitempty"`  // wake-unavailable
	Stranded      []router.StrandedAgent `json:"stranded,omitempty"`
}

func runCtr(_ *cobra.Command, args []string) error {
	if ctrInstall {
		return runCtrInstall()
	}

	scope := ""
	if len(args) == 1 {
		scope = args[0]
	}

	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("locate the router: %w\n  (no repo here and no ~/.sirsi/pantheon-repo marker — run `sirsi setup` once)", err)
	}
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	ns, err := router.CollectNodeStatus(repoRoot, nil)
	if err != nil {
		return fmt.Errorf("read router state: %w", err)
	}

	var wp router.WakePassReport
	if !ctrNoWake {
		// The wake-or-declare-unavailable pass. Idempotent: an item already woken
		// within the retry window is skipped, so spamming ctr never re-spawns.
		if wp, err = router.WakePass(routerRoot, time.Now().UTC()); err != nil {
			return fmt.Errorf("wake pass: %w", err)
		}
	}

	res := buildCtrResult(repoRoot, scope, ns, wp)

	if ctrJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if ctrQuiet {
		fmt.Printf("ctr: %d pending across %d agent(s) · woke %d · watching %d · needs-owner %d\n",
			res.PendingTotal, res.AgentsPending, len(res.Woke), len(res.AlreadyLive), len(res.NeedsOwner))
		return nil
	}
	renderCtr(res)
	return nil
}

// buildCtrResult folds the node-status surface and the wake pass into one
// result, filtered to a single agent when scoped.
func buildCtrResult(repoRoot, scope string, ns *router.NodeStatus, wp router.WakePassReport) ctrResult {
	keep := func(agentID string) bool { return scope == "" || agentID == scope }

	res := ctrResult{
		Repo:         filepath.Base(repoRoot),
		Scope:        scope,
		LiveThreads:  ns.LiveThreadCount,
		StaleThreads: len(ns.StaleThreads),
	}
	for agent, ids := range ns.PendingByAgent {
		if !keep(agent) || len(ids) == 0 {
			continue
		}
		res.PendingTotal += len(ids)
		res.AgentsPending++
	}
	for _, s := range ns.StrandedInbox {
		if keep(s.AgentID) {
			res.Stranded = append(res.Stranded, s)
		}
	}
	for _, o := range wp.Attempted {
		if keep(o.AgentID) {
			res.Woke = append(res.Woke, o)
		}
	}
	for _, o := range wp.Armed {
		if keep(o.AgentID) {
			res.AlreadyLive = append(res.AlreadyLive, o)
		}
	}
	for _, o := range wp.Unavailable {
		if keep(o.AgentID) {
			res.NeedsOwner = append(res.NeedsOwner, o)
		}
	}
	return res
}

func renderCtr(res ctrResult) {
	scopeNote := ""
	if res.Scope != "" {
		scopeNote = "  [" + res.Scope + "]"
	}
	fmt.Printf("𓁢 CTR — Check The Router   %s%s\n", res.Repo, scopeNote)
	fmt.Printf("   %d pending across %d agent(s) · %d live · %d stale\n\n",
		res.PendingTotal, res.AgentsPending, res.LiveThreads, res.StaleThreads)

	if len(res.Woke) > 0 {
		fmt.Printf("⏰ Woke this pass (%d):\n", len(res.Woke))
		for _, o := range byAgentCount(res.Woke) {
			via := ""
			if o.adapter != "" {
				via = " via " + o.adapter
			}
			fmt.Printf("    • %-22s %d item(s)%s\n", o.agent, o.count, via)
		}
		fmt.Println()
	}
	if len(res.Stranded) > 0 {
		fmt.Printf("🚧 Stranded — open items, no live watcher (%d):\n", len(res.Stranded))
		for _, s := range res.Stranded {
			fmt.Printf("    • %-22s %d item(s) waiting\n", s.AgentID, s.OpenItems)
		}
		fmt.Println("    → that agent's session must run `ctr` / `/ctr` to consume them.")
		fmt.Println()
	}
	if len(res.AlreadyLive) > 0 {
		// "Heartbeat-fresh" ≠ "consuming its inbox": a session can check in yet
		// never pull. This is the exact failure the owner hit, so say it plainly.
		fmt.Printf("✓ Heartbeat-fresh (%d) — a session is checked in:\n", len(res.AlreadyLive))
		for _, o := range byAgentCount(res.AlreadyLive) {
			fmt.Printf("    • %-22s %d item(s)\n", o.agent, o.count)
		}
		fmt.Println("    → if these items still aren't moving, that session must run `/ctr` to pull them.")
		fmt.Println()
	}
	if len(res.NeedsOwner) > 0 {
		fmt.Printf("⚠ Needs owner — wake-unavailable (%d):\n", len(res.NeedsOwner))
		for _, o := range res.NeedsOwner {
			detail := ""
			if o.Detail != "" {
				detail = " — " + o.Detail
			}
			fmt.Printf("    • %-22s %s%s\n", o.AgentID, o.ItemID, detail)
		}
		fmt.Println()
	}

	switch {
	case res.PendingTotal == 0:
		fmt.Println("Router is clear — no items waiting. Nothing to wake.")
	case ctrNoWake:
		fmt.Println("→ Surface only (--no-wake). Run `ctr` (no flag) to wake, or `/ctr` inside Claude.")
	default:
		fmt.Println("→ Re-run `ctr` anytime (or `/ctr` in Claude). No daemon to keep alive — the trigger is you.")
	}
}

// agentCount collapses per-item wake outcomes into per-agent counts for display.
type agentCount struct {
	agent   string
	adapter string
	count   int
}

func byAgentCount(outcomes []router.WakeOutcome) []agentCount {
	idx := map[string]int{}
	var out []agentCount
	for _, o := range outcomes {
		if i, ok := idx[o.AgentID]; ok {
			out[i].count++
			continue
		}
		idx[o.AgentID] = len(out)
		out = append(out, agentCount{agent: o.AgentID, adapter: o.Adapter, count: 1})
	}
	return out
}
