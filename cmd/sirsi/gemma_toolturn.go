package main

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reason"
	"github.com/spf13/cobra"
)

const apollo21ProfileID = "gemma-m5-apollo21"

var (
	gemmaToolTurnProfile string
	gemmaToolTurnAllowed []string
	gemmaToolTurnTokens  int
)

// gemmaToolTurnCmd is deliberately a separate, opt-in surface. The ordinary
// `sirsi gemma` command remains text-only; it must never gain tool authority by
// virtue of pointing at an OpenAI-compatible endpoint.
var gemmaToolTurnCmd = &cobra.Command{
	Use:   "tool-turn [prompt]",
	Short: "Run one bounded Apollo 2.1 tool turn through an explicit local profile",
	Long: `Offer only explicitly named, registered observe tools to a selected Apollo 2.1 profile.
The model cannot run shell commands, choose tools outside the allowlist, supply arguments,
or loop: Pantheon executes at most one permitted tool and sends its factual result back once.`,
	Args: cobra.ExactArgs(1),
	RunE: runGemmaToolTurn,
}

func init() {
	gemmaToolTurnCmd.Flags().StringVar(&gemmaToolTurnProfile, "inference-profile", "", "Explicit installed Apollo 2.1 local profile JSON")
	gemmaToolTurnCmd.Flags().StringSliceVar(&gemmaToolTurnAllowed, "allow-tool", nil, "Exact registered observe tool(s) to offer (required)")
	gemmaToolTurnCmd.Flags().IntVar(&gemmaToolTurnTokens, "max-tokens", 512, "Maximum tokens for each bounded model response")
	gemmaCmd.AddCommand(gemmaToolTurnCmd)
}

func runGemmaToolTurn(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(gemmaToolTurnProfile) == "" {
		return fmt.Errorf("tool turn requires --inference-profile for the exact %s profile", apollo21ProfileID)
	}
	if gemmaToolTurnTokens < 1 {
		return fmt.Errorf("tool turn max-tokens must be positive")
	}
	p, err := provider.ReadLocalProfile(gemmaToolTurnProfile)
	if err != nil {
		return err
	}
	if p.ID != apollo21ProfileID {
		return fmt.Errorf("tool turn requires profile id %q, got %q", apollo21ProfileID, p.ID)
	}
	if !provider.ProfileSupportsTools(p) {
		return fmt.Errorf("selected %s profile does not explicitly declare tool_calls", apollo21ProfileID)
	}
	r := reason.NewRegistry()
	if err := reason.MachineTools(r); err != nil {
		return fmt.Errorf("register bounded machine tools: %w", err)
	}
	turn := reason.ToolTurn{
		Provider: &provider.OpenAICompat{
			ProviderName:          apollo21ProfileID,
			Endpoint:              p.Endpoint,
			Model:                 p.Model,
			TierValue:             provider.TierLocal,
			SupportsTools:         true,
			SupportsStreaming:     *p.Streaming,
			ContextTokens:         p.ContextTokens,
			OutputTokens:          p.OutputTokens,
			RequestTimeoutSeconds: provider.ProfileRequestTimeoutSeconds(p),
			AccessTokenFile:       p.AccessTokenFile,
		},
		Registry: r,
		Policy:   reason.PolicyReadOnly,
		Allowed:  gemmaToolTurnAllowed,
		System:   "Use an offered tool only when needed. Treat tool output as factual evidence. Do not claim any unreturned action.",
	}
	result, err := turn.Complete(cmd.Context(), strings.TrimSpace(args[0]), gemmaToolTurnTokens)
	if err != nil {
		return err
	}
	if result.UsedTool {
		fmt.Fprintf(cmd.ErrOrStderr(), "Apollo used %s (%s); returning the bounded follow-up.\n", result.Invocation.Tool, result.Invocation.Tier)
	}
	if text := strings.TrimSpace(result.Final.Text); text != "" {
		fmt.Fprintln(cmd.OutOrStdout(), text)
		return nil
	}
	return fmt.Errorf("Apollo returned no final answer")
}
