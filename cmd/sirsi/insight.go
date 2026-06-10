package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/gemma"
	"github.com/SirsiMaster/sirsi-pantheon/internal/insight"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var insightNoAI bool

// insightCmd is the platform "state of the union": a deterministic, cross-deity
// recommendation that works with ZERO AI, optionally narrated by the local Gemma
// backend when one is installed (the "operate without AI, include it if present"
// contract — Pantheon is more than scan/clean/diagnose).
var insightCmd = &cobra.Command{
	Use:   "insight",
	Short: "𓂀 Cross-deity state of the union + what to do next (AI-optional)",
	Long: `Aggregates a deterministic, platform-wide picture across the Pantheon
deities — Anubis (hygiene), Horus (ops/health), Thoth (memory), the router
(collaboration), Ma'at/Ra — and prioritizes concrete next actions.

The recommendation is 100% rule-based and never requires a model. If the
optional local Gemma backend is installed, it adds a plain-language narration;
its absence changes nothing. Use --no-ai to force the deterministic-only view.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot := resolveRepoRoot()
		p := insight.Build(repoRoot)

		// OPTIONAL AI enrichment — gated by Available(); failure is non-fatal and
		// the deterministic result still stands.
		if !insightNoAI {
			cfg := gemma.Load()
			if cfg.Available() {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				if narr, err := cfg.Generate(ctx, gemmaPrompt(p)); err == nil && strings.TrimSpace(narr) != "" {
					p.Narrative = strings.TrimSpace(narr)
					p.Source = "rules+gemma"
				}
			}
		}

		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(p)
		}
		renderInsight(p)
		return nil
	},
}

func init() {
	insightCmd.Flags().BoolVar(&insightNoAI, "no-ai", false, "Deterministic only — skip the optional Gemma narration")
	rootCmd.AddCommand(insightCmd)
}

func resolveRepoRoot() string {
	if root, err := router.FindRepoRoot(); err == nil && root != "" {
		return root
	}
	wd, _ := os.Getwd()
	return wd
}

// gemmaPrompt turns the deterministic state into a tight instruction for Gemma.
func gemmaPrompt(p insight.Platform) string {
	var b strings.Builder
	b.WriteString("You are the on-device insight engine inside Sirsi Pantheon (a local Mac infrastructure platform). ")
	b.WriteString("Given this deterministic state, write 2-3 plain sentences telling the user what's going on and what to do first. No preamble, no markdown.\n\nDeity status:\n")
	for _, s := range p.Signals {
		b.WriteString(fmt.Sprintf("- %s: %s\n", s.Deity, s.Status))
	}
	if len(p.Actions) > 0 {
		b.WriteString("\nPrioritized actions:\n")
		for _, a := range p.Actions {
			b.WriteString(fmt.Sprintf("- %s (%s) → %s\n", a.Title, a.Why, a.Command))
		}
	}
	return b.String()
}

func renderInsight(p insight.Platform) {
	output.Banner()
	output.Header("Pantheon — State of the Union")

	for _, s := range p.Signals {
		dot := map[int]string{0: "🟢", 1: "🟡", 2: "🔴"}[s.Severity]
		output.Info("%s %s — %s", dot, s.Deity, s.Status)
	}

	if p.Narrative != "" {
		fmt.Println()
		output.Header("𓂀 Insight (local Gemma)")
		for _, line := range strings.Split(p.Narrative, "\n") {
			if strings.TrimSpace(line) != "" {
				output.Dim("  %s", strings.TrimSpace(line))
			}
		}
	}

	if len(p.Actions) > 0 {
		cr := &output.CommandResult{Command: "sirsi insight", Summary: "Prioritized next steps across the Pantheon"}
		for _, a := range p.Actions {
			cr.AddNextAction(a.Command, a.Title)
		}
		cr.AddEvidence("Source", p.Source)
		cr.Render()
	} else {
		output.Success("Everything healthy — nothing to do right now. (source: %s)", p.Source)
	}
}
