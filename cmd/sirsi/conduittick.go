package main

// conduittick.go — `sirsi conduit tick` (ADR-039 P3): ONE iteration of the
// continuous work loop. Plan the open queue → dispatch what is authorized (only
// under autonomous ON) → surface what needs the owner. This is the side-effecting
// tick the trigger mesh (P4) fires; run by hand it is a safe, explicit single
// step. Every dispatch goes through router.ExecuteActionable, which re-asserts
// the honest gate + the unforgeable authorization, so an owner-gated item can
// never be acted on here.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

var conduitCmd = &cobra.Command{
	Use:   "conduit",
	Short: "The continuous work surface — plan, dispatch non-gated work, surface the owner-queue (ADR-039)",
	Long: `The continuous wake-and-work surface. Each tick reconciles the open router
queue, dispatches everything that is not owner-gated (only when autonomous is on),
and surfaces exactly what is waiting on the owner. Continuity comes from the
trigger mesh firing ticks, not from a resident daemon.

  sirsi conduit tick           Run one loop iteration now
  sirsi conduit plan           (see: sirsi router plan) the read-only plan`,
}

var conduitTickCmd = &cobra.Command{
	Use:   "tick",
	Short: "Run one iteration of the continuous work loop",
	RunE:  runConduitTick,
}

func init() {
	conduitCmd.AddCommand(conduitTickCmd)
	rootCmd.AddCommand(conduitCmd)
}

func runConduitTick(_ *cobra.Command, _ []string) error {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("no idea-router found: %w", err)
	}
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	items, err := work.ListInbox(routerRoot, "")
	if err != nil {
		return fmt.Errorf("read open items: %w", err)
	}

	autonomous := false
	if cfg, cErr := brain.LoadConfig(); cErr == nil {
		autonomous = cfg.AutonomousMode()
	}

	plans := router.PlanAll(items, autonomous)
	owner := router.OwnerQueue(plans)
	actionable := router.Actionable(plans)

	dispatched := 0
	var wakeErr error
	if autonomous && len(actionable) > 0 {
		rep, dErr := router.ExecuteActionable(routerRoot, actionable)
		if dErr != nil {
			wakeErr = dErr
		} else {
			dispatched = len(rep.Armed) + len(rep.Attempted)
		}
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		out := map[string]any{
			"autonomous":  autonomous,
			"open_total":  len(plans),
			"need_owner":  len(owner),
			"actionable":  len(actionable),
			"dispatched":  dispatched,
			"owner_queue": owner,
		}
		if wakeErr != nil {
			out["dispatch_error"] = wakeErr.Error()
		}
		return enc.Encode(out)
	}

	mode := "off"
	if autonomous {
		mode = "on"
	}
	fmt.Printf("𓁢 conduit tick (autonomous %s): %d open · %d need-owner · %d actionable · %d dispatched\n",
		mode, len(plans), len(owner), len(actionable), dispatched)
	if !autonomous && len(actionable) > 0 {
		fmt.Printf("   %d non-gated item(s) held — `sirsi autonomous on` to let the loop dispatch them.\n", len(actionable))
	}
	if autonomous && len(actionable) == 0 && len(owner) == len(plans) && len(plans) > 0 {
		fmt.Println("   ✅ honest idle — every remaining item needs the owner; nothing else is stuck.")
	}
	if wakeErr != nil {
		fmt.Fprintf(os.Stderr, "   ⚠ dispatch: %v\n", wakeErr)
	}
	return nil
}
