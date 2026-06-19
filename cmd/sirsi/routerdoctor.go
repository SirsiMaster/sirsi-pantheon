package main

import (
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var routerDoctorFix bool

// `sirsi router doctor` — the router's self-check (router hardening PR #3). It
// aggregates the contract violations the honest-liveness work (#79/#80) and
// stranded-inbox surface (#85) expose into ONE report:
//   - live threads that claim live but whose watcher loop is dead (armed:false)
//   - inboxes with open items but no armed watcher (stranded work)
//   - stale records whose heartbeat aged out
//
// Report-only by default; --fix runs the SAFE, non-destructive repair (reap
// OS-dead records via ADR-022 OS-truth — never PIDAlive/suspended/terminal).
// Anything that needs operator consent is surfaced, not auto-done.
var routerDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Self-check the router fabric (unarmed threads, stranded inboxes, stale records)",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		ns, err := router.CollectNodeStatus(repoRoot, nil)
		if err != nil {
			return fmt.Errorf("collect node-status: %w", err)
		}

		var unarmed []router.ThreadSummary
		for _, t := range ns.LiveThreads {
			if !t.Armed {
				unarmed = append(unarmed, t)
			}
		}

		fmt.Printf("𓂀 Router Doctor — %d agents registered · %d live · %d stale\n\n",
			ns.AgentCount, ns.LiveThreadCount, len(ns.StaleThreads))

		issues := 0
		if len(unarmed) > 0 {
			issues++
			fmt.Printf("⚠ %d live thread(s) NOT armed (claim live, watcher loop dead):\n", len(unarmed))
			for _, t := range unarmed {
				fmt.Printf("    %s  agent=%s  reason=%s\n", t.ThreadID, t.AgentID, t.ArmedReason)
			}
			fmt.Println("    → that agent must re-arm its /loop watcher (it is not consuming its inbox).")
			fmt.Println()
		}
		if len(ns.StrandedInbox) > 0 {
			issues++
			fmt.Printf("⚠ %d agent(s) with a stranded inbox (open items, no armed watcher):\n", len(ns.StrandedInbox))
			for _, s := range ns.StrandedInbox {
				fmt.Printf("    %s: %d item(s) waiting\n", s.AgentID, s.OpenItems)
			}
			fmt.Println("    → bring that agent up (or it stays stranded until it is).")
			fmt.Println()
		}
		if len(ns.StaleThreads) > 0 {
			issues++
			fmt.Printf("⚠ %d stale thread record(s) — heartbeat aged out (OS-dead ones are reapable).\n\n", len(ns.StaleThreads))
		}

		if issues == 0 {
			fmt.Println("✅ Router healthy — every live thread armed, no stranded inboxes, no stale records.")
			return nil
		}

		if !routerDoctorFix {
			fmt.Printf("Found %d issue group(s). Re-run `sirsi router doctor --fix` to reap OS-dead records (safe, non-destructive); stranded/unarmed agents need an operator to bring them up.\n", issues)
			return nil
		}

		host, _ := os.Hostname()
		reaped, rerr := router.ReapDeadThreads(repoRoot, host)
		if rerr != nil {
			return fmt.Errorf("reap dead threads: %w", rerr)
		}
		fmt.Printf("✔ Reaped %d OS-dead thread record(s) (ADR-022 OS-truth — live/suspended/terminal untouched).\n", len(reaped))
		fmt.Println("  Stranded/unarmed agents are NOT auto-fixed — they need an operator to (re)arm them.")
		return nil
	},
}
