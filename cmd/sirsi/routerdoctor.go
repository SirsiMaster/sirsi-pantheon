package main

import (
	"fmt"
	"path/filepath"
	"strings"
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

		// Loop-dead is a PER-AGENT verdict, not per-session (router item
		// 20260714-210359): an agent needs ONE armed watcher; redundant live
		// sessions of the same agent (CCD duplicate records) are not each
		// obligated to run /loop. Only threads of agents with open items and
		// ZERO armed watchers are worth an alarm.
		loopDead := map[string]bool{}
		var unarmed []router.ThreadSummary
		for _, t := range ns.LiveThreads {
			if t.Armed {
				continue
			}
			verdict, seen := loopDead[t.AgentID]
			if !seen {
				verdict = router.AgentLoopDead(routerRoot, t.AgentID, ns.PendingByAgent)
				// Credit a loaded per-agent launchd wake job as armed — for an
				// app-hosted session (CLI respawned each turn, no durable process)
				// the wake LaunchAgent is the ONLY durable consumer available, and
				// it is one of the two remedies the doctor itself recommends.
				// Ignoring it causes false loop-dead alarms and misroutes peers.
				if verdict && router.DefaultLaunchctlChecker("list", "ai.sirsi.router.wake."+t.AgentID) == nil {
					verdict = false
				}
				loopDead[t.AgentID] = verdict
			}
			if verdict {
				unarmed = append(unarmed, t)
			}
		}

		fmt.Printf("𓂀 Router Doctor — %d agents registered · %d live · %d stale\n",
			ns.AgentCount, ns.LiveThreadCount, len(ns.StaleThreads))
		fmt.Printf("   Dispatch authority: %s\n\n", cutoverModeLine())

		issues := 0
		threadReg, threadErr := router.LoadThreadRegistry(routerRoot)
		if threadErr != nil {
			issues++
			fmt.Printf("⚠ supervision truth UNKNOWN — cannot read thread registry: %v\n\n", threadErr)
		} else if staleSupervisors := router.StaleActiveSupervisors(threadReg, time.Now().UTC(), router.DefaultThreadStaleAfter); len(staleSupervisors) > 0 {
			issues++
			fmt.Printf("⚠ %d active supervisory registration(s) missed their own heartbeat contract — not healthy:\n", len(staleSupervisors))
			for _, thread := range staleSupervisors {
				fmt.Printf("    %s  agent=%s  surface=%s  wake=%s  last_seen=%s\n",
					thread.ThreadID, thread.AgentID, thread.Surface, thread.WakeMechanism, thread.LastSeenAt.Format(time.RFC3339))
			}
			fmt.Println("    → restore that exact loop or demit the obsolete registration; a live shared host PID is not heartbeat proof.")
			fmt.Println()
		}
		if len(unarmed) > 0 {
			issues++
			fmt.Printf("⚠ %d live thread(s) of loop-dead agent(s) — open items, zero armed watchers:\n", len(unarmed))
			for _, t := range unarmed {
				fmt.Printf("    %s  agent=%s  reason=%s\n", t.ThreadID, t.AgentID, t.ArmedReason)
			}
			fmt.Println("    → that agent must arm ONE watcher (/loop, or `sirsi router wake-install <agent>`) — its inbox has no consumer.")
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

		// REGISTRY DRIFT — the router reads the WORKING TREE, so a registry fix
		// that merged to main has not necessarily reached the live registry. That
		// landmine armed three times in six days and every remedy was a copy;
		// this is the check instead, so it announces itself rather than
		// re-arming silently. Report-only, deliberately: --fix must never
		// overwrite the working tree from origin, because the working tree is
		// legitimately ahead sometimes and clobbering it would destroy
		// unpushed registrations.
		if drift := router.CheckRegistryDrift(repoRoot); !drift.Checked {
			// IO7a: unknown is NOT clean, and must not be rendered as clean.
			issues++
			fmt.Printf("⚠ registry drift UNKNOWN — could not compare the live registry against origin/main: %s\n", drift.Unknown)
			fmt.Println("    → `git fetch origin main` and re-run; an unchecked registry is not a clean one.")
			fmt.Println()
		} else if drift.Drifted() {
			issues++
			fmt.Printf("⚠ registry DRIFT — %s is the live registry and it is missing what origin/main has:\n", router.RegistryPath)
			for _, a := range drift.MissingAgents {
				fmt.Printf("    agent %s: merged but ABSENT live — the router cannot address it\n", a)
			}
			for _, f := range drift.LostFields {
				fmt.Printf("    %s: lost %s (origin/main has %q)\n", f.AgentID, f.Path, f.Upstream)
			}
			for _, c := range drift.ChangedFields {
				fmt.Printf("    %s: %s differs — live %q, origin/main %q\n", c.AgentID, c.Path, c.Live, c.Upstream)
			}
			fmt.Println("    → a merged registry change is NOT a deployed one. Reconcile the branch; do not hand-copy the file.")
			fmt.Println()
		} else if len(drift.AheadAgents) > 0 {
			// Not an issue: unpushed registrations are normal. Visible anyway,
			// because "live-only" is exactly what someone needs to know when an
			// agent works here and nowhere else.
			fmt.Printf("ℹ registry is ahead of origin/main (not drift): %s\n\n", strings.Join(drift.AheadAgents, ", "))
		}

		if issues == 0 {
			fmt.Println("✅ Router healthy — every live thread armed, no stranded inboxes, no stale records.")
			return nil
		}

		if !routerDoctorFix {
			fmt.Printf("Found %d issue group(s). Re-run `sirsi router doctor --fix` to reap OS-dead records (safe, non-destructive); stranded/unarmed agents need an operator to bring them up.\n", issues)
			return nil
		}

		reaped, rerr := router.ReapDeadThreads(repoRoot)
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
		// These are ITEM counts, and the label must say so: "62 already-armed"
		// read as 62 armed AGENTS, overstating fleet health — "armed" here means
		// only that the item's recipient had a heartbeat-fresh watcher when the
		// pass ran (claude-home, router item 20260730-052314).
		fmt.Printf("✔ Wake pass: %d item(s) wake-attempted · %d item(s) on heartbeat-armed agents · %d item(s) wake-unavailable (recorded on the items).\n",
			len(wp.Attempted), len(wp.Armed), len(wp.Unavailable))
		for _, u := range wp.Unavailable {
			fmt.Printf("    ✗ %s → %s: %s\n", u.ItemID, u.AgentID, u.Detail)
		}
		fmt.Println("  Interactive agents are never blind-spawned — re-arm their /loop or route via the claude-home conduit.")
		return nil
	},
}
