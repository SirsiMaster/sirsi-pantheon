package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		reg, _ := router.LoadRegistry(routerRoot) // best-effort; wake readiness is advisory in report mode
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
				readiness := "no wake mechanism configured — would mark wake-unavailable"
				if reg != nil {
					if cfg, lookErr := reg.Lookup(s.AgentID); lookErr == nil {
						if h := router.ProbeWakeReadiness(*cfg); h.Ready {
							readiness = fmt.Sprintf("would wake via %s", h.Adapter)
						} else {
							readiness = "wake-unavailable: " + h.Detail
						}
					}
				}
				fmt.Printf("    %s: %d item(s) waiting — %s\n", s.AgentID, s.OpenItems, readiness)
			}
			fmt.Println("    → `sirsi router doctor --fix` runs the wake-or-declare-unavailable pass (never blind-spawns interactive agents).")
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

		// Wake-or-declare-unavailable pass (PR#2). Runs from the DOCTOR tick, never
		// from `router send`. Wakes agents with a ready, explicit, non-blind-spawn
		// adapter; records wake-unavailable on every other stranded item so nothing
		// is silently stranded. Interactive claude-* are never blind-spawned.
		wp, werr := router.WakePass(routerRoot, time.Now().UTC())
		if werr != nil {
			return fmt.Errorf("wake pass: %w", werr)
		}
		fmt.Printf("✔ Wake pass: %d woken · %d already-armed · %d wake-unavailable (recorded on the items).\n",
			len(wp.Attempted), len(wp.Armed), len(wp.Unavailable))
		for _, u := range wp.Unavailable {
			fmt.Printf("    ✗ %s → %s: %s\n", u.ItemID, u.AgentID, u.Detail)
		}
		fmt.Println("  Interactive agents are never blind-spawned — re-arm their /loop or route via the claude-home conduit.")
		return nil
	},
}
