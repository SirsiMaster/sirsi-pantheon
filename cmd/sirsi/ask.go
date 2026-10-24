package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/modelrouter"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reason"
	"github.com/spf13/cobra"
)

var (
	askJSON      bool
	askFix       bool
	askTask      string
	askPrivacy   string
	askShowRoute bool
	askRoute     bool
	askLane      string
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
	askCmd.Flags().StringVar(&askTask, "task", "generation", "task class for model routing: generation|judgment|extraction|embedding")
	askCmd.Flags().StringVar(&askPrivacy, "privacy", "local-only", "privacy class: local-only|shareable")
	askCmd.Flags().BoolVar(&askShowRoute, "show-route", false, "print the model routing decision before the answer")
	askCmd.Flags().BoolVar(&askRoute, "route", false, "explicitly request model routing even when Doctor reports alarms")
	askCmd.Flags().StringVar(&askLane, "lane", "", "explicit model lane override: local|remote")
	rootCmd.AddCommand(askCmd)
}

type askRuntime struct {
	doctor            func() (*guard.DoctorReport, error)
	ladder            func(context.Context, string) []provider.Provider
	newRouter         func([]provider.Provider) *modelrouter.Router
	applicableRepairs func() int
	out               io.Writer
}

func runAsk(cmd *cobra.Command, args []string) error {
	return runAskWithCommand(cmd, args, askRuntime{
		doctor:            guard.Doctor,
		ladder:            provider.Ladder,
		newRouter:         func(ladder []provider.Provider) *modelrouter.Router { return modelrouter.New(ladder) },
		applicableRepairs: applicableRepairs,
	})
}

func runAskWithCommand(cmd *cobra.Command, args []string, runtime askRuntime) error {
	if cmd != nil {
		runtime.out = cmd.OutOrStdout()
	}
	return runAskWith(args, runtime)
}

func runAskWith(args []string, runtime askRuntime) error {
	if askJSON && askFix {
		return fmt.Errorf("--json and --fix cannot be combined: --fix performs interactive repairs; run without --json")
	}
	if askRoute && askFix {
		return fmt.Errorf("--route and --fix cannot be combined: run the explicit routed answer or the requested repairs separately")
	}
	question := strings.TrimSpace(strings.Join(args, " "))
	if question == "" {
		question = "what is wrong with this machine right now?"
	}
	out := runtime.out
	if out == nil {
		out = os.Stdout
	}

	// ── Rung 1: heuristic ────────────────────────────────────────────────────
	// Runs first, always, with no model. On 2026-07-27 this rung alone was
	// sufficient for a 358-process fork storm and a 40.5 GB broker footprint —
	// the two things that actually took the machine down.
	report, err := runtime.doctor()
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
		output.HeaderTo(out, "Sirsi")
		output.DimTo(out, "  %s", question)
		fmt.Fprintln(out)
	}

	if len(alarms) > 0 && !askRoute {
		if askJSON {
			return emitAskJSON(out, question, report, alarms)
		}
		printAskAlarms(out, alarms)
		output.DimTo(out, "  Answered at the heuristic rung: zero tokens, no model loaded, works offline.")
		if askFix {
			return runAskFix(out, report)
		}
		if n := runtime.applicableRepairs(); n > 0 {
			output.DimTo(out, "  %d repair(s) apply to this machine — `sirsi ask --fix` runs them and verifies each.", n)
		}
		return nil
	}
	if len(alarms) > 0 {
		if !askJSON {
			printAskAlarms(out, alarms)
			output.DimTo(out, "  [router] explicit --route requested; measured findings remain visible.")
			fmt.Fprintln(out)
		}
	}

	if !askJSON && len(alarms) == 0 {
		output.SuccessTo(out, "  [heuristic] no measured problems — %d signals checked, %s",
			len(report.Findings), report.Status)
	}

	// ── Rung 2+: escalate via model router ──────────────────────────────────
	// Reached when measurement is quiet or the user explicitly opts in with
	// --route. The router applies task/privacy policy and selects a qualified
	// lane; every actual route is shown so a remote move is never silent.
	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	ladder := runtime.ladder(ctx, home)
	rt := runtime.newRouter(ladder)
	routeReq := modelrouter.Request{
		Task:     modelrouter.TaskClass(askTask),
		Privacy:  modelrouter.PrivacyClass(askPrivacy),
		Override: modelrouter.Lane(askLane),
	}
	resp, decision, cerr := rt.Complete(ctx, routeReq, provider.Request{
		System: "You are Sirsi, answering about the user's own machine. " +
			"Be concise and concrete. If you do not have the evidence, say so plainly.",
		Prompt:    question + "\n\nMeasured state: " + summarize(report),
		MaxTokens: 400,
	})
	var recordErr *modelrouter.DecisionRecordError
	hasRecordErr := errors.As(cerr, &recordErr)
	var surfacedRecordErr error
	if hasRecordErr {
		surfacedRecordErr = recordErr
	}
	if cerr != nil {
		if !hasRecordErr {
			if !askJSON {
				fmt.Fprintln(out)
				output.DimTo(out, "  [router] %s", decisionDisplay(decision))
				output.DimTo(out, "  No model answer was produced; the heuristic findings above remain the answer.")
			} else if err := emitAskJSONWithDecision(out, question, report, alarms, decision, decisionOutcome(decision)); err != nil {
				return err
			}
			return nil
		}
		if !askJSON && resp.Text == "" {
			fmt.Fprintln(out)
			output.DimTo(out, "  [router] %s", decisionDisplay(decision))
			output.DimTo(out, "  No model answer was produced; decision evidence could not be saved.")
			return surfacedRecordErr
		} else if askJSON && resp.Text == "" {
			if err := emitAskJSONWithDecision(out, question, report, alarms, decision, decisionOutcome(decision)); err != nil {
				return err
			}
			return surfacedRecordErr
		}
	}

	if !askJSON {
		fmt.Fprintln(out)
		output.DimTo(out, "  [router] %s", decisionDisplay(decision))
	}
	if !askJSON {
		fmt.Fprintln(out)
		output.DimTo(out, "  [heuristic] inconclusive → routed to %s (%s)", decision.Lane, decisionDisplay(decision))
		if askShowRoute && decision.Rationale != "" {
			output.DimTo(out, "  [router] %s", decision.Rationale)
		}
	}
	if askJSON {
		if err := emitAskModelJSON(out, question, report, alarms, resp, decision); err != nil {
			return err
		}
		return surfacedRecordErr
	}
	fmt.Fprintln(out)
	output.InfoTo(out, "  %s", strings.TrimSpace(resp.Text))
	fmt.Fprintln(out)
	output.DimTo(out, "  [%s] %s · %s · %d tokens · finish=%s",
		resp.Tier, resp.Provider, resp.Model, resp.OutputTokens, resp.FinishReason)
	return surfacedRecordErr
}

func decisionOutcome(d modelrouter.Decision) string {
	if d.Provider != nil || d.Lane != "" || d.FailureCategory == "canceled" {
		return "failed"
	}
	return "refused"
}

func printAskAlarms(out io.Writer, alarms []guard.DiagnosticFinding) {
	output.InfoTo(out, "  [heuristic] %d measured problem(s):", len(alarms))
	for _, f := range alarms {
		label := "⚠"
		if f.Severity >= guard.SeverityCritical {
			label = "✘"
		}
		output.InfoTo(out, "    %s %s", label, f.Message)
		if f.Detail != "" {
			output.DimTo(out, "        %s", f.Detail)
		}
		if cmdStr := guard.RemediationFor(f); cmdStr != "" {
			output.DimTo(out, "        fix: %s", cmdStr)
		}
	}
}

func decisionDisplay(d modelrouter.Decision) string {
	if d.Provider == nil {
		if d.FailureCategory != "" {
			return "refused (" + d.FailureCategory + ")"
		}
		return "refused"
	}
	providerName := d.ProviderName
	if providerName == "" {
		providerName = d.Provider.Name()
	}
	model := d.Model
	if model != "" {
		providerName += "/" + model
	}
	text := string(d.Lane) + "/" + providerName
	if d.FallbackReason != "" {
		text += " (fallback: " + d.FallbackReason + ")"
	}
	return text
}

func emitAskModelJSON(out io.Writer, question string, r *guard.DoctorReport, alarms []guard.DiagnosticFinding, resp provider.Response, decision modelrouter.Decision) error {
	payload := struct {
		Question     string           `json:"question"`
		Tier         string           `json:"tier"`
		Status       string           `json:"status"`
		Signals      int              `json:"signals"`
		Answer       string           `json:"answer"`
		Provider     string           `json:"provider"`
		Model        string           `json:"model"`
		OutputTokens int              `json:"output_tokens"`
		FinishReason string           `json:"finish_reason"`
		Problems     []askProblemJSON `json:"problems"`
		Routing      askRouteJSON     `json:"routing"`
	}{
		Question: question, Tier: resp.Tier.String(), Status: string(r.Status),
		Signals: len(r.Findings), Answer: strings.TrimSpace(resp.Text),
		Provider: resp.Provider, Model: resp.Model, OutputTokens: resp.OutputTokens,
		FinishReason: resp.FinishReason, Routing: askRouteProjection(decision, "completed"),
	}
	for _, f := range alarms {
		payload.Problems = append(payload.Problems, askProblemJSON{Check: f.Check, Severity: f.Severity, Message: f.Message})
	}
	if payload.Problems == nil {
		payload.Problems = []askProblemJSON{}
	}
	return json.NewEncoder(out).Encode(payload)
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

func emitAskJSON(out io.Writer, question string, r *guard.DoctorReport, alarms []guard.DiagnosticFinding) error {
	return emitAskJSONWithDecision(out, question, r, alarms, modelrouter.Decision{}, "")
}

type askRouteJSON struct {
	RequestedOverride string `json:"requested_override,omitempty"`
	InitialLane       string `json:"initial_lane,omitempty"`
	FinalLane         string `json:"final_lane,omitempty"`
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	Reason            string `json:"reason,omitempty"`
	FallbackReason    string `json:"fallback_reason,omitempty"`
	Outcome           string `json:"outcome,omitempty"`
	FailureCategory   string `json:"failure_category,omitempty"`
	OverrideApplied   bool   `json:"override_applied,omitempty"`
}

func askRouteProjection(d modelrouter.Decision, outcome string) askRouteJSON {
	projection := askRouteJSON{RequestedOverride: string(d.RequestedOverride), InitialLane: string(d.InitialLane), FinalLane: string(d.Lane),
		Reason: d.Rationale, FallbackReason: d.FallbackReason, Outcome: outcome, FailureCategory: d.FailureCategory, OverrideApplied: d.OverrideApplied}
	projection.Provider, projection.Model = d.ProviderName, d.Model
	if d.Provider != nil && projection.Provider == "" {
		projection.Provider = d.Provider.Name()
	}
	return projection
}

func emitAskJSONWithDecision(out io.Writer, question string, r *guard.DoctorReport, alarms []guard.DiagnosticFinding, decision modelrouter.Decision, outcome string) error {
	tier := "heuristic"
	payload := struct {
		Question string           `json:"question"`
		Tier     string           `json:"tier"`
		Status   string           `json:"status"`
		Signals  int              `json:"signals"`
		Problems []askProblemJSON `json:"problems"`
		Routing  *askRouteJSON    `json:"routing,omitempty"`
	}{Question: question, Tier: tier, Status: string(r.Status), Signals: len(r.Findings), Problems: []askProblemJSON{}}
	for _, f := range alarms {
		payload.Problems = append(payload.Problems, askProblemJSON{Check: f.Check, Severity: f.Severity, Message: f.Message})
	}
	if outcome != "" {
		routing := askRouteProjection(decision, outcome)
		payload.Routing = &routing
	}
	return json.NewEncoder(out).Encode(payload)
}

type askProblemJSON struct {
	Check    string                   `json:"check"`
	Severity guard.DiagnosticSeverity `json:"severity"`
	Message  string                   `json:"message"`
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
func runAskFix(out io.Writer, before *guard.DoctorReport) error {
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
	fmt.Fprintln(out)
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
			output.DimTo(out, "  ○ %s — skipped: %s", name, why)
			continue
		}
		output.InfoTo(out, "  ▸ %s — %s", name, why)
		inv := reason.Invoke(ctx, r, name, reason.PolicyAutoRepair)
		done = append(done, ran{name, inv})

		if inv.Err != nil {
			output.WarnTo(out, "    ✘ %s", inv.Err)
			continue
		}
		if inv.Result.Summary != "" {
			output.DimTo(out, "    ran:      %s", inv.Result.Summary)
		}
		if inv.Verified != nil {
			output.SuccessTo(out, "    verified: %s", inv.Verified.Summary)
		}
	}

	if len(done) == 0 {
		fmt.Fprintln(out)
		output.DimTo(out, "  Nothing to repair — no tool declared itself applicable. The findings above")
		output.DimTo(out, "  need a lever Sirsi does not have yet, which is a gap worth reporting.")
		return nil
	}

	// Re-read the world. This is the whole point of --fix over printing a command.
	after, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("re-check machine state: %w", err)
	}
	fmt.Fprintln(out)
	output.InfoTo(out, "  after re-checking:")
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
			output.SuccessTo(out, "    %s %s %s %s  (%s)", f.Check, sevName(was), arrow, sevName(f.Severity), f.Message)
		} else {
			output.WarnTo(out, "    %s %s %s %s  (%s)", f.Check, sevName(was), arrow, sevName(f.Severity), f.Message)
		}
	}
	if changed == 0 {
		output.DimTo(out, "    no severity changed — the repairs ran and verified, but the machine reads the same.")
		output.DimTo(out, "    That is a real answer, not a failure: the cause is elsewhere.")
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
