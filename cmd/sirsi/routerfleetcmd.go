package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

// `router fleet` exists so the fleet board has ONE producer.
//
// Before this, Horus rendered the fleet from ledger.Build while the SwiftUI
// menubar rendered its own view from `router node-status`, and a Python board
// outside the repo rendered a third from yet another read. Three renderings of
// one router cannot agree, and no fix to any single surface makes them agree —
// the divergence is in having three. This command is the shared seam: every
// surface decodes the same JSON instead of re-deriving it.
//
// Deliberately stateless. The activity feed is a diff of consecutive reads and
// therefore belongs to a long-lived process (Horus), not to a one-shot CLI
// invocation that has no previous read to diff against. `activity` is always
// empty here and `seeded` false — which is exactly what an honest one-shot
// should report, rather than implying the fleet has been quiet.
// laneLabel renders a supervision state for humans. Every state must map
// explicitly: a default that says "stopped — no open work" turned a lane with
// 24 open items into a finished one the moment the state vocabulary grew.
func laneLabel(state string) string {
	switch state {
	case dashboard.LaneWorking:
		return "WORKING"
	case dashboard.LaneAssigned:
		return "assigned — claimed"
	case dashboard.LaneIdleWithWork:
		return "IDLE — work waiting"
	case dashboard.LaneBlocked:
		return "blocked"
	case dashboard.LaneUnroutable:
		return "UNROUTABLE"
	case dashboard.LaneComplete:
		return "complete — no open work"
	default:
		// An unmapped state is a bug, and saying so is better than silently
		// picking a benign-sounding label for it.
		return "unknown state: " + state
	}
}

var routerFleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Fleet board: per-lane status and totals (the shared producer for Horus + menubar)",
	Long: `Prints the fleet board — every agent lane with open/active/stalled/blocked
counts and recency, plus fleet totals.

This is the SAME computation Horus serves at /api/fleet, so surfaces that render
it agree by construction instead of by coincidence.

The activity feed is not included: it is derived by diffing consecutive reads and
needs a long-lived process to hold the baseline. Use the Horus board for that.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		snap, err := ledger.Build(repoRoot, "", time.Now().UTC(), ledger.DefaultStaleAfter)
		if err != nil {
			return fmt.Errorf("build ledger: %w", err)
		}
		out := dashboard.NewFleetTracker(unroutableAgents(repoRoot)).Observe(snap, time.Now())

		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		s := out.Summary
		fmt.Printf("⚑ Fleet — %d of %d lanes actively working\n\n", s.LanesWork, s.LanesTotal)
		fmt.Printf("  COMPLETED / IN FLIGHT    %d / %d   (%d%% done, %d still in flight)\n", s.Done, s.Total, s.PctDone, s.InFlight)
		fmt.Printf("  IN PROGRESS / ASSIGNED   %d / %d   (%d assigned but not started)\n", s.Active, s.Active+s.Assigned, s.Assigned)
		fmt.Printf("  STALLED / BLOCKED        %d   (%d stalled · %d blocked · %d idle lanes)\n\n", s.Stalled+s.Blocked, s.Stalled, s.Blocked, s.IdleLanes)
		for _, l := range out.Lanes {
			state := laneLabel(l.State)
			line := fmt.Sprintf("  %-26s %-24s %d open", l.Agent, state, l.Open)
			if l.Inbox > 0 {
				line += fmt.Sprintf(" · %d inbox", l.Inbox)
			}
			if l.Active > 0 {
				line += fmt.Sprintf(" · %d active", l.Active)
			}
			if l.Stalled > 0 {
				line += fmt.Sprintf(" · %d stalled", l.Stalled)
			}
			if l.Blocked > 0 {
				line += fmt.Sprintf(" · %d blocked", l.Blocked)
			}
			if l.TouchedAgo != "" {
				line += " · touched " + l.TouchedAgo
			}
			fmt.Println(line)
		}
		return nil
	},
}
