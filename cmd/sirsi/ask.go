package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reason"
	"github.com/spf13/cobra"
)

var (
	askJSON bool
	askFix  bool
)

var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask Sirsi about this machine — heuristic first, local model only when needed",
	Long: `Ask — the certainties ladder.

Sirsi answers from the cheapest rung that can actually answer:

  heuristic   deterministic checks. Zero tokens, no model loaded, always available.
  local       on-device model. Zero tokens, works offline, costs RAM.
  remote      frontier model. Costs tokens, needs the network.

The tier IS the confidence, and it is always reported: you can tell whether you
are reading a measurement, an inference, or a judgment. Escalation is explicit —
Sirsi never quietly moves you onto a paid rung.

Most questions about a machine never leave the heuristic rung. That is the point:
the machine that most needs help is the one with the least headroom to run a model.

  sirsi ask                          # what is wrong with this machine right now
  sirsi ask "why is my machine slow?"
  sirsi ask --json`,
	Args: cobra.ArbitraryArgs,
	RunE: runAsk,
}

func init() {
	askCmd.Flags().BoolVar(&askJSON, "json", false, "machine-readable answer with tier provenance")
	askCmd.Flags().BoolVar(&askFix, "fix", false, "act on what was found: run the repair tools that apply, verify each, and re-check")
	rootCmd.AddCommand(askCmd)
}

func runAsk(_ *cobra.Command, args []string) error {
	question := strings.TrimSpace(strings.Join(args, " "))
	if question == "" {
		question = "what is wrong with this machine right now?"
	}

	// ── Rung 1: heuristic ────────────────────────────────────────────────────
	// Runs first, always, with no model. On 2026-07-27 this rung alone was
	// sufficient for a 358-process fork storm and a 40.5 GB broker footprint —
	// the two things that actually took the machine down.
	report, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("read machine state: %w", err)
	}
	var alarms []guard.DiagnosticFinding
	for _, f := range report.Findings {
		if f.Severity >= guard.SeverityWarn {
			alarms = append(alarms, f)
		}
	}

	if !askJSON {
		output.Header("Sirsi")
		output.Dim("  %s", question)
		fmt.Println()
	}

	if len(alarms) > 0 {
		if askJSON {
			return emitAskJSON(question, report, alarms)
		}
		output.Info("  [heuristic] %d measured problem(s) — no model needed:", len(alarms))
		for _, f := range alarms {
			label := "⚠"
			if f.Severity >= guard.SeverityCritical {
				label = "✘"
			}
			output.Info("    %s %s", label, f.Message)
			if f.Detail != "" {
				output.Dim("        %s", f.Detail)
			}
			if cmdStr := guard.RemediationFor(f); cmdStr != "" {
				output.Dim("        fix: %s", cmdStr)
			}
		}
		fmt.Println()
		output.Dim("  Answered at the heuristic rung: zero tokens, no model loaded, works offline.")
		if askFix {
			return runAskFix(report)
		}
		if n := applicableRepairs(); n > 0 {
			output.Dim("  %d repair(s) apply to this machine — `sirsi ask --fix` runs them and verifies each.", n)
		}
		return nil
	}

	if !askJSON {
		output.Success("  [heuristic] no measured problems — %d signals checked, %s",
			len(report.Findings), report.Status)
	}

	// ── Rung 2+: escalate ────────────────────────────────────────────────────
	// Only reached when measurement found nothing to say. The escalation is
	// announced before it happens; silently moving a user onto a paid rung
	// would betray the whole model.
	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	ladder := provider.Ladder(ctx, home)
	if len(ladder) == 0 {
		if askJSON {
			return emitAskJSON(question, report, alarms)
		}
		fmt.Println()
		output.Dim("  No model configured, so no further rung to escalate to.")
		output.Dim("  Sirsi still answered — the heuristic rung needs no model at all.")
		return nil
	}

	for _, p := range ladder {
		if !p.Available(ctx) {
			if !askJSON {
				output.Dim("  [%s] unavailable — escalating", p.Tier())
			}
			continue
		}
		if !askJSON {
			fmt.Println()
			output.Dim("  [heuristic] inconclusive → escalating to %s (%s)", p.Tier(), p.Name())
		}

		resp, cerr := p.Complete(ctx, provider.Request{
			System: "You are Sirsi, answering about the user's own machine. " +
				"Be concise and concrete. If you do not have the evidence, say so plainly.",
			Prompt:    question + "\n\nMeasured state: " + summarize(report),
			MaxTokens: 400,
		})
		if cerr != nil {
			if !askJSON {
				output.Dim("  [%s] failed: %v — escalating", p.Tier(), cerr)
			}
			continue
		}
		if askJSON {
			return emitAskModelJSON(question, report, resp)
		}
		fmt.Println()
		output.Info("  %s", strings.TrimSpace(resp.Text))
		fmt.Println()
		output.Dim("  [%s] %s · %s · %d tokens · finish=%s",
			resp.Tier, resp.Provider, resp.Model, resp.OutputTokens, resp.FinishReason)
		return nil
	}

	if askJSON {
		return emitAskJSON(question, report, alarms)
	}
	fmt.Println()
	output.Dim("  Every model rung was unavailable. The heuristic answer above still stands.")
	return nil
}

func emitAskModelJSON(question string, r *guard.DoctorReport, resp provider.Response) error {
	payload := struct {
		Question     string `json:"question"`
		Tier         string `json:"tier"`
		Status       string `json:"status"`
		Signals      int    `json:"signals"`
		Answer       string `json:"answer"`
		Provider     string `json:"provider"`
		Model        string `json:"model"`
		OutputTokens int    `json:"output_tokens"`
		FinishReason string `json:"finish_reason"`
	}{
		Question: question, Tier: resp.Tier.String(), Status: string(r.Status),
		Signals: len(r.Findings), Answer: strings.TrimSpace(resp.Text),
		Provider: resp.Provider, Model: resp.Model, OutputTokens: resp.OutputTokens,
		FinishReason: resp.FinishReason,
	}
	return json.NewEncoder(os.Stdout).Encode(payload)
}

// summarize gives the model the measured facts. The model interprets evidence;
// it is never the source of it — a model asked to guess at machine state will
// produce a fluent, confident, unfalsifiable answer.
func summarize(r *guard.DoctorReport) string {
	var b strings.Builder
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "%s: %s. ", f.Check, f.Message)
	}
	return b.String()
}

func emitAskJSON(question string, r *guard.DoctorReport, alarms []guard.DiagnosticFinding) error {
	tier := "heuristic"
	fmt.Printf("{\"question\":%q,\"tier\":%q,\"status\":%q,\"signals\":%d,\"problems\":[",
		question, tier, r.Status, len(r.Findings))
	for i, f := range alarms {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf("{\"check\":%q,\"severity\":%d,\"message\":%q}", f.Check, f.Severity, f.Message)
	}
	fmt.Println("]}")
	return nil
}

// applicableRepairs counts the repair tools that say they apply to this machine
// right now. Cheap enough to run on the read-only path so the offer is only made
// when it is real — "run --fix" printed under a finding no tool addresses is a
// dead lever, and a dead lever teaches an operator to ignore live ones.
func applicableRepairs() int {
	r := reason.NewRegistry()
	if err := reason.MachineTools(r); err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	for _, name := range r.Names() {
		t, ok := r.Get(name)
		if !ok || t.Tier < reason.TierRepair || t.Applies == nil {
			continue
		}
		if ok, _ := t.Applies(ctx); ok {
			n++
		}
	}
	return n
}

// runAskFix is the act half of the loop. It runs every repair tool that declares
// itself applicable, then RE-READS the machine and reports the severity delta per
// check — because "the repair succeeded" and "the problem is gone" are different
// claims, and only the second one is what the user asked for.
func runAskFix(before *guard.DoctorReport) error {
	r := reason.NewRegistry()
	if err := reason.MachineTools(r); err != nil {
		return fmt.Errorf("register repair tools: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	type ran struct {
		name string
		inv  reason.Invocation
	}
	var done []ran
	fmt.Println()
	for _, name := range r.Names() {
		t, ok := r.Get(name)
		if !ok || t.Tier < reason.TierRepair {
			continue
		}
		if t.Applies == nil {
			continue // never fire a repair that cannot say why it is relevant
		}
		applies, why := t.Applies(ctx)
		if !applies {
			output.Dim("  ○ %s — skipped: %s", name, why)
			continue
		}
		output.Info("  ▸ %s — %s", name, why)
		inv := reason.Invoke(ctx, r, name, reason.PolicyAutoRepair)
		done = append(done, ran{name, inv})

		if inv.Err != nil {
			output.Warn("    ✘ %s", inv.Err)
			continue
		}
		if inv.Result.Summary != "" {
			output.Dim("    ran:      %s", inv.Result.Summary)
		}
		if inv.Verified != nil {
			output.Success("    verified: %s", inv.Verified.Summary)
		}
	}

	if len(done) == 0 {
		fmt.Println()
		output.Dim("  Nothing to repair — no tool declared itself applicable. The findings above")
		output.Dim("  need a lever Sirsi does not have yet, which is a gap worth reporting.")
		return nil
	}

	// Re-read the world. This is the whole point of --fix over printing a command.
	after, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("re-check machine state: %w", err)
	}
	fmt.Println()
	output.Info("  after re-checking:")
	prev := map[string]guard.DiagnosticSeverity{}
	for _, f := range before.Findings {
		prev[f.Check] = f.Severity
	}
	changed := 0
	for _, f := range after.Findings {
		was, seen := prev[f.Check]
		if !seen || was == f.Severity {
			continue
		}
		changed++
		arrow := "→"
		if f.Severity < was {
			output.Success("    %s %s %s %s  (%s)", f.Check, sevName(was), arrow, sevName(f.Severity), f.Message)
		} else {
			output.Warn("    %s %s %s %s  (%s)", f.Check, sevName(was), arrow, sevName(f.Severity), f.Message)
		}
	}
	if changed == 0 {
		output.Dim("    no severity changed — the repairs ran and verified, but the machine reads the same.")
		output.Dim("    That is a real answer, not a failure: the cause is elsewhere.")
	}
	return nil
}

func sevName(s guard.DiagnosticSeverity) string {
	switch {
	case s >= guard.SeverityCritical:
		return "CRITICAL"
	case s >= guard.SeverityWarn:
		return "WARN"
	default:
		return "OK"
	}
}
