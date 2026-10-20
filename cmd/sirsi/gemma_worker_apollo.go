package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reason"
)

const gemmaWorkerApolloSystem = "You are Apollo, Pantheon's local bounded worker. Use an offered observe tool only when needed. Treat its returned evidence as factual. Return a concise final answer; do not claim any action not present in the evidence."

func gemmaWorkerApolloTurnGenerator(profile *provider.LocalProfile, registry *reason.Registry, allowed []string, maxTokens int) func(context.Context, string) (string, error) {
	backend := &provider.OpenAICompat{
		ProviderName:          apollo21ProfileID,
		Endpoint:              profile.Endpoint,
		Model:                 profile.Model,
		TierValue:             provider.TierLocal,
		SupportsTools:         true,
		SupportsStreaming:     *profile.Streaming,
		ContextTokens:         profile.ContextTokens,
		OutputTokens:          profile.OutputTokens,
		RequestTimeoutSeconds: provider.ProfileRequestTimeoutSeconds(profile),
		AccessTokenFile:       profile.AccessTokenFile,
	}
	return func(ctx context.Context, instructions string) (string, error) {
		return gemmaWorkerApolloComplete(ctx, backend, registry, allowed, instructions, maxTokens)
	}
}

func gemmaWorkerApolloComplete(ctx context.Context, backend provider.Provider, registry *reason.Registry, allowed []string, instructions string, maxTokens int) (string, error) {
	turn := reason.ToolTurn{
		Provider: backend,
		Registry: registry,
		Policy:   reason.PolicyReadOnly,
		Allowed:  allowed,
		System:   gemmaWorkerApolloSystem,
	}
	result, err := turn.Complete(ctx, strings.TrimSpace(instructions), maxTokens)
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(result.Final.Text)
	if answer == "" {
		return "", fmt.Errorf("Apollo returned no final answer")
	}
	if !result.UsedTool {
		return answer, nil
	}
	// The enclosing worker reply already binds this text to the claimed router
	// item and its one-way lease fingerprint. Include the exact factual result
	// here so a recipient can distinguish the model's explanation from the
	// observation it was allowed to use.
	evidence, err := json.Marshal(struct {
		Tool     string         `json:"tool"`
		Summary  string         `json:"summary"`
		Evidence map[string]any `json:"evidence,omitempty"`
	}{
		Tool: result.Invocation.Tool, Summary: result.Invocation.Result.Summary, Evidence: result.Invocation.Result.Evidence,
	})
	if err != nil {
		return "", fmt.Errorf("encode Apollo tool evidence: %w", err)
	}
	return answer + "\n\nREAD-ONLY TOOL EVIDENCE:\n" + string(evidence), nil
}
