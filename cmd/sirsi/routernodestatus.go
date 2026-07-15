package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

// sirsi router node-status [--json] — the Horus ops-view operator surface
// (ADR-026). Wraps router.CollectNodeStatus(); --json output is
// contract-identical to GET /api/node-status (same schema; CLI uses
// pretty-printed JSON, the HTTP body uses compact encoding — codex
// arch-verify 2026-06-02 PASS).
//
// Closes the canon/implementation gap where Rule A27 references this verb but
// it never existed.

var routerNodeStatusJSON bool

var routerNodeStatusCmd = &cobra.Command{
	Use:   "node-status",
	Short: "Show the Horus local-node operator view (router queue, threads, drift, auth)",
	Long: `Aggregates router queue state, the CTR thread registry (with OS-truth
	PID liveness, ADR-022), LaunchAgent/binary-drift detection (ADR-023), and agent CLI
auth health into one read-model. Mirrors GET /api/node-status (ADR-026);
--json output is byte-identical to the HTTP body.

This is a read-only verb: it never registers a thread, writes the inbox, or
mutates registry state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		ns, err := router.CollectNodeStatus(repoRoot, nil)
		if err != nil {
			return fmt.Errorf("collect node-status: %w", err)
		}
		if routerNodeStatusJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(ns)
		}
		renderNodeStatus(ns)
		return nil
	},
}

// renderNodeStatus prints the human-readable lipgloss-style summary
// (Rule A10). Compact, no styling lib needed for v1 — same shape as
// `sirsi router status`, additive in the same module.
func renderNodeStatus(ns *router.NodeStatus) {
	fmt.Printf("𓂀  Horus Node Status   (schema %s)\n\n", ns.SchemaVersion)
	fmt.Printf("  Repo:        %s\n", ns.RepoRoot)
	fmt.Printf("  Router home: %s\n", ns.RouterHome)
	fmt.Println()

	fmt.Printf("  Agents:           %d registered\n", ns.AgentCount)
	fmt.Printf("  Queue:            %d pending across %d agents (%d completed topics)\n",
		ns.TotalPending, len(ns.PendingByAgent), ns.CompletedCount)
	fmt.Printf("  Live threads:     %d (stale: %d)\n", ns.LiveThreadCount, len(ns.StaleThreads))
	if len(ns.RecentFailures) > 0 {
		fmt.Printf("  Recent failures:  %d (newest first)\n", len(ns.RecentFailures))
	}
	if len(ns.LaunchAgents) > 0 {
		installed := 0
		for _, h := range ns.LaunchAgents {
			if h.Installed {
				installed++
			}
		}
		fmt.Printf("  LaunchAgents:     %d installed (%d known)\n", installed, len(ns.LaunchAgents))
	}
	fmt.Println()

	if len(ns.LiveThreads) > 0 {
		fmt.Println("  Live threads:")
		for _, t := range ns.LiveThreads {
			// Honest liveness: make a heartbeat-fresh-but-loop-dead thread visible
			// instead of silently "live" — exactly the "claims live but is idle" case
			// the owner flagged. armed_reason is the surface-native verdict.
			armed := "armed (" + t.ArmedReason + ")"
			if !t.Armed {
				armed = "⚠ NOT ARMED (" + t.ArmedReason + ")"
			}
			fmt.Printf("    • %s  agent=%s  surface=%s  pid=%d  os=%s  loop=%s  idle=%.0fs  %s\n",
				t.ThreadID, t.AgentID, t.Surface, t.PID, t.OSState, t.LoopState, t.IdleSeconds, armed)
		}
		fmt.Println()
	}
	if len(ns.StaleThreads) > 0 {
		fmt.Println("  Stale threads:")
		for _, t := range ns.StaleThreads {
			fmt.Printf("    • %s  agent=%s  pid=%d  idle=%.0fs\n",
				t.ThreadID, t.AgentID, t.PID, t.IdleSeconds)
		}
		fmt.Println()
	}
	if len(ns.PendingByAgent) > 0 {
		fmt.Println("  Pending by agent:")
		for agent, ids := range ns.PendingByAgent {
			fmt.Printf("    %s: %d\n", agent, len(ids))
		}
		fmt.Println()
	}
	if len(ns.StrandedInbox) > 0 {
		// Make stranded work VISIBLE: these agents have open items but no armed
		// thread watching them — the work sits until they are (re)armed.
		fmt.Println("  ⚠ Stranded inbox (open items, NO armed watcher):")
		for _, s := range ns.StrandedInbox {
			fmt.Printf("    %s: %d item(s) waiting — not armed\n", s.AgentID, s.OpenItems)
		}
		fmt.Println()
	}
	if len(ns.LaunchAgents) > 0 {
		fmt.Println("  Router/App helpers:")
		for _, h := range ns.LaunchAgents {
			status := "missing"
			if h.Installed {
				status = "installed"
				if h.Loaded {
					status = "loaded"
				}
			}
			if h.Legacy && !h.Installed {
				status = "legacy-missing"
			}
			line := fmt.Sprintf("    %s: %s — %s", h.Label, status, h.Role)
			if h.Program != "" {
				line += fmt.Sprintf(" (%s)", h.Program)
			}
			fmt.Println(line)
		}
		fmt.Println()
	}
	if len(ns.AgentHealth) > 0 {
		fmt.Println("  Agent CLI health:")
		for _, h := range ns.AgentHealth {
			status := "ok"
			if !h.CLIFound {
				status = "not-found"
			} else if h.NeedsLogin {
				status = "needs-login"
			} else if h.Degraded {
				// Inconclusive probe (cold-start timeout / env) — NOT a logout.
				status = "probe-inconclusive"
			}
			line := fmt.Sprintf("    %s: %s", h.AgentType, status)
			if h.AuthError != "" {
				line += "  — " + h.AuthError
			}
			if h.BlockedItems > 0 {
				line += fmt.Sprintf("  (blocking %d items)", h.BlockedItems)
			}
			fmt.Println(line)
		}
	}
}

func init() {
	routerNodeStatusCmd.Flags().BoolVar(&routerNodeStatusJSON, "json", false,
		"Output the raw NodeStatus JSON (byte-identical to GET /api/node-status)")
	routerCmd.AddCommand(routerNodeStatusCmd)
}
