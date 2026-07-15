// braincmd.go — `sirsi brain`, the Orchestration Brain CONTROL PLANE surface
// (PANTHEON_RULES A29, ADR-034). It is the operator's window into the tiered,
// pluggable brain: which provider drives each role, what LLM Level that is, and
// whether the Tier-0 substrate (the EXISTING router registry + wake system) is
// healthy.
//
// It REUSES, never rebuilds, the wake subsystem: `brain status`/`doctor` read
// internal/router's ProbeWakeReadiness/CollectNodeStatus (surfaced via
// internal/brain.Dispatcher). Waking + repair stay with `sirsi router doctor
// --fix` (which runs WakePass); the brain SURFACES + ENFORCES the invariant, the
// router ACTS on it.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/spf13/cobra"
)

var (
	brainStatusJSON bool
	brainDoctorJSON bool
)

var brainCmd = &cobra.Command{
	Use:   "brain",
	Short: "The Orchestration Brain control plane — tiered, pluggable, user-navigable (A29)",
	Long: `The Orchestration Brain is a config you own: a deterministic Tier-0 dispatch
floor (ships ON, needs no AI) that you can dial UP one tier at a time.

  Level 0  Deterministic       rules only — works out of the box, zero AI
  Level 1  + Local triage      a local model screens ambiguous items (zero-token)
  Level 2  + Agentic execution a local agent handles escalated work
  Level 3  + Hosted            opt-in API keys per role (the only per-token path)

The eternal loop is Tier-0 (the router), never a model. Every LLM choice is
visible, swappable, and troubleshootable from here.`,
}

var brainStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the active tier/level, provider per role, registry + wake health, RAM headroom",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := brain.LoadConfig()
		if err != nil {
			return err
		}
		var d brain.Dispatcher
		if rd, derr := brain.NewRouterDispatcher(); derr != nil {
			// Degrade, don't die: no router fabric on this machine (fresh
			// install) still renders config + level, with the registry/wake
			// diagnosis explaining what's unreachable and how to fix it.
			d = brain.UnreachableDispatcher{Err: derr}
		} else {
			d = rd
		}
		st := brain.Collect(cfg, d)
		if brainStatusJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(st)
		}
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "𓁟 Orchestration Brain — Level %d\n", st.Level)
		if st.Autonomous {
			fmt.Fprintf(w, "Autonomous mode: ON — Pantheon may apply fixes unattended (`sirsi autonomous off` to stop)\n\n")
		} else {
			fmt.Fprintf(w, "Autonomous mode: OFF — Pantheon observes + proposes only (`sirsi autonomous on` to let it act)\n\n")
		}
		fmt.Fprintln(w, "Role        Tier     Provider")
		for _, r := range st.Roles {
			fmt.Fprintf(w, "%-11s %-8s %s\n", r.Role, r.Tier, r.Provider.String())
		}
		fmt.Fprintf(w, "\nTier-0 substrate (router registry + wake):\n")
		fmt.Fprintf(w, "  %d agent(s) registered · %d wakeable · %d stranded inbox(es)\n",
			st.Registry.Registered, st.Registry.Wakeable, st.Registry.Stranded)
		if len(st.Registry.Unwakeable) > 0 {
			fmt.Fprintf(w, "  ⚠ %d registered agent(s) with NO live wake-channel: %v\n", len(st.Registry.Unwakeable), st.Registry.Unwakeable)
			fmt.Fprintln(w, "    → run `sirsi brain doctor` for the fix.")
		}
		if st.Node.TotalRAM > 0 {
			fmt.Fprintf(w, "  RAM: %.1f GB free / %.1f GB total · pressure %s\n",
				float64(st.Node.FreeRAM)/(1<<30), float64(st.Node.TotalRAM)/(1<<30), st.Node.Pressure)
		}
		return nil
	},
}

var brainLevelsCmd = &cobra.Command{
	Use:   "levels",
	Short: "Show the LLM spectrum (Level 0–3): what each unlocks and what it costs",
	RunE: func(cmd *cobra.Command, args []string) error {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "𓁟 LLM spectrum — the slider you navigate:\n\n")
		for _, l := range brain.Spectrum() {
			fmt.Fprintf(w, "  Level %d — %s  [%s]\n      %s\n", l.Level, l.Name, l.Cost, l.Unlocks)
		}
		fmt.Fprintln(w, "\nSwap a role with `sirsi brain use <role> <provider>`; `sirsi brain use <role> none` reverts to deterministic.")
		return nil
	},
}

var brainUseCmd = &cobra.Command{
	Use:   "use <role> <provider>",
	Short: "Plug a provider into a role (e.g. `use triage local:qwen2.5-7b-instruct`; `use triage none`)",
	Long: `Swap the provider for an orchestration role. Takes effect on next read — no
restart (A29 §swap-without-restart).

  roles:      dispatch (Tier-0, must stay 'none') · triage (Tier-1) · execution (Tier-2)
  providers:  none · local:<model-id> · hosted:<provider-id>

Examples:
  sirsi brain use triage local:qwen2.5-7b-instruct
  sirsi brain use execution hosted:claude-3-5-haiku
  sirsi brain use triage none`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		role := brain.Role(args[0])
		if err := brain.Set(role, args[1]); err != nil {
			return err
		}
		cfg, err := brain.LoadConfig()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✔ %s → %s (Level %d). No restart needed.\n", role, cfg.Provider(role).String(), cfg.Level())
		return nil
	},
}

var brainDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Troubleshoot the brain in plain English: config, registry/wake invariant, stranded inboxes, RAM",
	Long: `Diagnose the Orchestration Brain and its Tier-0 substrate. It SURFACES and
ENFORCES the Registry/Wake invariant (A29) by reading the existing router wake
system — it never wakes or mutates anything. Repairs stay with
'sirsi router doctor --fix' (which runs the wake pass).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := brain.LoadConfig()
		if err != nil {
			return err
		}
		var d brain.Dispatcher
		if rd, derr := brain.NewRouterDispatcher(); derr != nil {
			// Degrade, don't die: no router fabric on this machine (fresh
			// install) still renders config + level, with the registry/wake
			// diagnosis explaining what's unreachable and how to fix it.
			d = brain.UnreachableDispatcher{Err: derr}
		} else {
			d = rd
		}
		diags := brain.Doctor(cfg, d)
		if brainDoctorJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(diags)
		}
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "𓁟 Brain Doctor\n\n")
		problems := 0
		for _, dg := range diags {
			if dg.Ok {
				fmt.Fprintf(w, "  ✓ %s — %s\n", dg.Check, dg.Detail)
				continue
			}
			problems++
			fmt.Fprintf(w, "  ⚠ %s — %s\n", dg.Check, dg.Detail)
			if dg.Fix != "" {
				fmt.Fprintf(w, "      fix: %s\n", dg.Fix)
			}
		}
		fmt.Fprintln(w)
		if problems == 0 {
			fmt.Fprintln(w, "✅ Brain healthy — deterministic floor intact, substrate wakeable, RAM ok.")
		} else {
			fmt.Fprintf(w, "Found %d issue(s) above; each lists its plain-English fix.\n", problems)
		}
		return nil
	},
}

var brainTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Dry-run the current brain config: derive the level + tiers, no side effects",
	Long: `Show what the current config resolves to — the level, the tier each role
drives, and any config problems — without touching the router or loading a
model. A safe way to preview a ` + "`brain use`" + ` change (A29 §test/no-side-effects).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := brain.LoadConfig()
		if err != nil {
			return err
		}
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "𓁟 Brain (dry-run) — Level %d\n\n", cfg.Level())
		for _, r := range brain.Roles() {
			p := cfg.Provider(r)
			fmt.Fprintf(w, "  %-11s → %s\n", r, p.String())
		}
		if verrs := cfg.Validate(); len(verrs) > 0 {
			fmt.Fprintln(w, "\nConfig problems (would fail doctor):")
			for _, e := range verrs {
				fmt.Fprintf(w, "  ⚠ %s\n", e)
			}
			return nil
		}
		fmt.Fprintln(w, "\nConfig valid. Tier-0 dispatch stays deterministic. No side effects performed.")
		return nil
	},
}

func init() {
	brainStatusCmd.Flags().BoolVar(&brainStatusJSON, "json", false, "emit the status read-model as JSON (for menubar/Nexus surfaces)")
	brainDoctorCmd.Flags().BoolVar(&brainDoctorJSON, "json", false, "emit the diagnoses as JSON")
	brainCmd.AddCommand(brainStatusCmd, brainLevelsCmd, brainUseCmd, brainDoctorCmd, brainTestCmd)
}
