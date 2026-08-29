package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/autoheal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// autonomous.go is the operator's master ACTION switch — the one plain-English
// on/off the owner asked for: "an autonomous mode the user can turn on or off."
//
// It is deliberately SEPARATE from `sirsi brain` (which selects how much
// intelligence drives decisions, Level 0–3) because autonomy is orthogonal: it
// says whether Pantheon may ACT on those decisions unattended (apply fixes,
// dispatch fleet work, manage the machine in real time) versus only observe and
// propose. Autonomy works at Level 0 (deterministic levers, zero AI) exactly as
// at Level 3 — the loop is Tier-0, never a model (A29 §deterministic-floor).
//
// The switch lives in the same owned config (~/.sirsi/brain.yaml) so it is
// visible, swappable, and troubleshootable from one place, and is read by every
// auto-apply path via brain.Config.AutonomousMode() as the single source of
// truth. OFF is the safe default and takes effect on the next unattended-loop
// read — no restart. The explicit `run` action is operator approval to perform
// one bounded fix pass even while the persistent switch remains OFF.

var autonomousCmd = &cobra.Command{
	Use:   "autonomous [on|off|status|run]",
	Short: "Turn Pantheon's unattended action on or off (observe-only vs. self-managing)",
	Long: `𓁟 Autonomous mode — the master action switch.

Pantheon always watches your Mac. Autonomous mode decides what it does with what
it sees:

  OFF (default)  Observe + propose. Pantheon reports problems and recommends
                 fixes, but never changes the machine on its own. You stay in
                 the loop for every action.
  ON             Self-managing. Pantheon may apply approved remediations and
                 manage the machine in real time without asking each time.

This is independent of the Orchestration Brain's LLM Level: autonomy runs on the
deterministic Tier-0 levers with zero AI configured, and equally when a model is
plugged in. The eternal loop is the router, never a model.

  sirsi autonomous            Show the current mode (same as 'status')
  sirsi autonomous on         Let Pantheon act unattended and run one safe fix pass now
  sirsi autonomous off        Back to observe + propose only
  sirsi autonomous status     Show the current mode
  sirsi autonomous run        Act now even when OFF: one bounded approved-fix pass; switch unchanged

Turning it OFF is always immediate and safe: the next thing any loop reads is
"observe only." Explicit repeated 'run' commands act each time and bypass the
unattended loop's cross-pass cooldown.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAutonomous,
}

var autonomousJSON bool

var (
	autonomousHealFn         = autoheal.RunReport
	autonomousApprovedHealFn = autoheal.RunApprovedReport
)

func runAutonomous(cmd *cobra.Command, args []string) error {
	action := "status"
	if len(args) == 1 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "on":
		if err := brain.SetAutonomous(true); err != nil {
			return err
		}
		outcomes, err := autonomousHealFn("", "")
		if err != nil {
			return renderAutonomousFailure(cmd, true, true, err)
		}
		return renderAutonomous(cmd, true, true, outcomes)
	case "off":
		on := action == "on"
		if err := brain.SetAutonomous(on); err != nil {
			return err
		}
		return renderAutonomous(cmd, on, true, nil)
	case "run":
		cfg, err := brain.LoadConfig()
		if err != nil {
			return err
		}
		outcomes, err := autonomousApprovedHealFn("", "")
		if err != nil {
			return err
		}
		if outcomes == nil {
			outcomes = []autoheal.Outcome{}
		}
		return renderAutonomous(cmd, cfg.AutonomousMode(), false, outcomes)
	case "status", "":
		cfg, err := brain.LoadConfig()
		if err != nil {
			return err
		}
		return renderAutonomous(cmd, cfg.AutonomousMode(), false, nil)
	default:
		return fmt.Errorf("unknown action %q (want: on | off | status | run)", action)
	}
}

func renderAutonomousFailure(cmd *cobra.Command, on, changed bool, passErr error) error {
	if autonomousJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"autonomous": on,
			"fix_pass": map[string]any{
				"error": passErr.Error(),
			},
		}); err != nil {
			return err
		}
		return passErr
	}
	if err := renderAutonomous(cmd, on, changed, nil); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "   Fix pass failed: %v\n", passErr)
	return passErr
}

func renderAutonomous(cmd *cobra.Command, on, changed bool, outcomes []autoheal.Outcome) error {
	if autonomousJSON {
		applied, proposed := summarizeAutoHeal(outcomes)
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"autonomous": on,
			"fix_pass": map[string]any{
				"outcomes": outcomes,
				"applied":  applied,
				"proposed": proposed,
			},
		})
	}
	w := cmd.OutOrStdout()
	state := "OFF"
	detail := "observing + proposing only — Pantheon will not change your Mac on its own"
	if on {
		state = "ON"
		detail = "self-managing — Pantheon may apply approved fixes unattended"
	}
	verb := "is"
	if changed {
		verb = "→"
	}
	fmt.Fprintf(w, "𓁟 Autonomous mode %s %s\n   %s\n", verb, state, detail)
	if !on {
		fmt.Fprintln(w, "   Turn it on with: sirsi autonomous on")
	} else {
		fmt.Fprintln(w, "   Turn it off with: sirsi autonomous off")
	}
	if outcomes != nil {
		applied, proposed := summarizeAutoHeal(outcomes)
		switch {
		case len(outcomes) == 0:
			fmt.Fprintln(w, "   Fix pass ran — no approved Warn+ remediation levers needed action.")
		default:
			fmt.Fprintf(w, "   Fix pass ran — %d applied · %d proposed/held\n", applied, proposed)
			for _, o := range outcomes {
				mark := "held"
				if o.Applied {
					mark = "applied"
				}
				fmt.Fprintf(w, "   - %s: %s (%s)\n", mark, o.Check, o.Reason)
			}
		}
	}
	return nil
}

func summarizeAutoHeal(outcomes []autoheal.Outcome) (applied, proposed int) {
	for _, o := range outcomes {
		if o.Applied {
			applied++
		} else {
			proposed++
		}
	}
	return applied, proposed
}

func init() {
	autonomousCmd.Flags().BoolVar(&autonomousJSON, "json", false, "Output the mode as JSON")
	// Wire the auto-heal pass into the supervisor's auto-heal duty (ADR-039 P3).
	// internal/autoheal imports internal/router (GateAction), so the router's
	// duty table reaches it through this injected seam rather than an import cycle.
	router.SetAutoHealFn(autoheal.Run)
	// Registry police's orchestration lives in cmd/sirsi so it can reuse the
	// CLI's native process/session discovery and guarded dispatch. The resident
	// supervisor invokes it through the router seam; no shell or python3 path
	// remains on the default duty.
	router.SetRegistryPoliceFn(runRegistryPoliceDuty)
	// Wire the self-healing gemma-liveness pass into its supervisor duty (A32,
	// owner directive 2026-07-17). Unlike auto-heal this is NOT gated on
	// autonomous mode — gemma is the Tier-0 substrate and must survive on
	// Pantheon's always-on supervisor regardless of any IDE session.
	router.SetGemmaLivenessFn(router.RunGemmaLivenessDuty)
	// Wire the dead-label kickstart pass (local sovereignty P1): plists on
	// disk absent from launchd get re-bootstrapped by the resident supervisor
	// — deterministic, no LLM, no cloud dependency.
	router.SetLaunchdKickstartFn(router.RunLaunchdKickstartDuty)
	// Wire the owner-facing run-report writer (local sovereignty P1, owner
	// directive 2026-07-23): each supervisor pass appends what it did — heals,
	// escalations, cloud reachability — to ~/.sirsi/conduit-report.json for
	// `sirsi report` and the menubar "Last check" line. Inert in library/test
	// context; only the real CLI writes the real file.
	router.SetRunReportFn(router.WriteSupervisorRun)
	// Wire the Spotlight index-marker duty (2026-08-07 reopened directive):
	// keeps the unprivileged `.metadata_never_index` exclusion current against
	// ~/Development on the resident supervisor's cadence, rather than only ever
	// running when a human remembers to type the CLI command by hand.
	router.SetSpotlightMarkersFn(func(_, _ string) error {
		return guard.RunMarkerDuty()
	})
}
