package main

// routerplan.go — `sirsi router plan` (ADR-039 P5): the owner-queue surface.
//
// Runs the tiered executor's PURE planner over the live open queue and shows the
// honest split: what is waiting on the owner (and why), what the loop WOULD act
// on this tick, and — when autonomous is off — what it is holding as proposals.
// Read-only: it plans, it never acts. This is the window the owner watches to see
// "these need me, and nothing else is stuck."

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

var routerPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show the continuous loop's plan for the open queue — owner-queue vs actionable (ADR-039)",
	Long: `Runs the tiered executor over every open router item and shows what the
continuous work surface would do with each: hold it for the owner (the honest
gate), act on it (dispatch to its agent), or — when autonomous is off — keep it
as a proposal. Read-only: it plans, it never acts.

  sirsi router plan            Human view of the plan for the live queue
  sirsi router plan --json     The plan as JSON (owner_queue / actionable / proposals)`,
	RunE: runRouterPlan,
}

func init() { routerCmd.AddCommand(routerPlanCmd) }

func runRouterPlan(_ *cobra.Command, _ []string) error {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("no idea-router found: %w", err)
	}
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	items, err := work.ListInbox(routerRoot, "")
	if err != nil {
		return fmt.Errorf("read open items: %w", err)
	}

	// Autonomous state is the single source of truth (#203). A missing/omitted
	// config reads as OFF — the safe default.
	autonomous := false
	if cfg, cErr := brain.LoadConfig(); cErr == nil {
		autonomous = cfg.AutonomousMode()
	}

	plans := router.PlanAll(items, autonomous)
	owner := router.OwnerQueue(plans)
	actionable := router.Actionable(plans)

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"autonomous":  autonomous,
			"open_total":  len(plans),
			"owner_queue": owner,
			"actionable":  actionable,
			"proposals":   proposalPlans(plans),
		})
	}

	mode := "OFF (propose-only)"
	if autonomous {
		mode = "ON (full-auto)"
	}
	fmt.Printf("𓁢 Continuous loop — plan   autonomous: %s\n", mode)
	fmt.Printf("   %d open · %d need owner · %d actionable\n\n", len(plans), len(owner), len(actionable))

	// Owner-queue — the honest gate, grouped by class so the owner sees the
	// shape of what's waiting (safety first).
	if len(owner) > 0 {
		fmt.Printf("🔒 Needs owner (%d) — the only work that legitimately waits:\n", len(owner))
		for _, class := range []router.GateClass{router.GateSafety, router.GateFounder, router.GateIrreversible, router.GateEscalate} {
			group := ownerByClass(owner, class)
			if len(group) == 0 {
				continue
			}
			fmt.Printf("   %s (%d):\n", class.String(), len(group))
			for _, p := range group {
				fmt.Printf("      • %-46s %s\n", truncID(p.ItemID), p.Gate.Reason)
			}
		}
		fmt.Println()
	}

	if autonomous {
		if len(actionable) > 0 {
			fmt.Printf("⚙️  Loop would act (%d) — dispatch to the target agent:\n", len(actionable))
			for _, p := range actionable {
				fmt.Printf("      → %-46s dispatch → %s\n", truncID(p.ItemID), p.To)
			}
		} else if len(owner) == len(plans) {
			fmt.Println("✅ Honest idle — every remaining item is owner-gated; nothing else is stuck.")
		}
	} else {
		props := proposalPlans(plans)
		if len(props) > 0 {
			fmt.Printf("💤 Held as proposals (%d) — autonomous is OFF, so the loop won't act:\n", len(props))
			fmt.Println("   turn on with `sirsi autonomous on` to let the loop dispatch these.")
		}
	}
	return nil
}

func proposalPlans(plans []router.ExecPlan) []router.ExecPlan {
	var p []router.ExecPlan
	for _, pl := range plans {
		if pl.Action == router.ActPropose {
			p = append(p, pl)
		}
	}
	return p
}

func ownerByClass(owner []router.ExecPlan, class router.GateClass) []router.ExecPlan {
	var g []router.ExecPlan
	for _, p := range owner {
		if p.Gate.Class == class {
			g = append(g, p)
		}
	}
	sort.Slice(g, func(i, j int) bool { return g[i].ItemID < g[j].ItemID })
	return g
}

// truncID keeps long router item ids readable in a fixed-width column.
func truncID(id string) string {
	const max = 46
	if len(id) <= max {
		return id
	}
	return id[:max-1] + "…"
}
