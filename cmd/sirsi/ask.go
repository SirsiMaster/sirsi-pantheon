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
	"github.com/spf13/cobra"
)

var askJSON bool

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
