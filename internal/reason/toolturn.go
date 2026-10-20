package reason

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

// ToolTurn is one bounded model → allowlisted tool → model turn. It does not
// discover commands, execute shell, or grant permissions. The model may ask;
// the preselected Registry and Policy decide whether anything happens.
//
// One call is intentionally the hard ceiling. A useful second action must be
// requested in a new user turn with its own policy and evidence, rather than
// becoming an invisible model-directed loop.
type ToolTurn struct {
	Provider provider.Provider
	Registry *Registry
	Policy   Policy
	// Allowed is the exact caller-selected subset of registered tools offered to
	// the model. Empty means no tool authority, not "all registered tools".
	Allowed []string
	System  string
}

type ToolTurnResult struct {
	Initial    provider.Response
	Invocation Invocation
	Final      provider.Response
	UsedTool   bool
}

func textMessage(role, text string) provider.Message {
	return provider.Message{Role: role, Content: &text}
}

// Complete runs a single bounded turn. Tool arguments are deliberately not
// accepted yet: the existing reason.Tool ABI has no typed argument channel.
// Refusing non-empty arguments is safer than silently dropping them or
// smuggling them into a shell string.
func (t ToolTurn) Complete(ctx context.Context, prompt string, maxTokens int) (ToolTurnResult, error) {
	if t.Provider == nil || t.Registry == nil {
		return ToolTurnResult{}, fmt.Errorf("tool turn requires provider and registry")
	}
	if strings.TrimSpace(prompt) == "" {
		return ToolTurnResult{}, fmt.Errorf("tool turn requires a prompt")
	}
	if !t.Provider.Caps().Tools {
		return ToolTurnResult{}, fmt.Errorf("selected provider %q does not declare structured tool support", t.Provider.Name())
	}
	specs, allowed, err := t.specs()
	if err != nil {
		return ToolTurnResult{}, err
	}
	initial, err := t.Provider.Complete(ctx, provider.Request{
		System: t.System, Prompt: prompt, MaxTokens: maxTokens, Tools: specs, RequireTool: true,
	})
	if err != nil {
		return ToolTurnResult{}, err
	}
	result := ToolTurnResult{Initial: initial, Final: initial}
	if len(initial.ToolCalls) == 0 {
		return result, nil
	}
	if !initial.ToolsHonored {
		return result, fmt.Errorf("selected provider did not honor the offered tool contract")
	}
	if len(initial.ToolCalls) != 1 {
		return result, fmt.Errorf("tool turn refused %d requested tools; exactly one is permitted", len(initial.ToolCalls))
	}
	call := initial.ToolCalls[0]
	canonicalName, ok := allowed[call.Name]
	if !ok {
		return result, fmt.Errorf("model requested unallowlisted tool %q", call.Name)
	}
	if strings.TrimSpace(call.ID) == "" {
		return result, fmt.Errorf("model requested tool %q without a correlation id", call.Name)
	}
	if len(call.Args) != 0 {
		return result, fmt.Errorf("model supplied unsupported arguments for tool %q", call.Name)
	}
	inv := Invoke(ctx, t.Registry, canonicalName, t.Policy)
	result.Invocation, result.UsedTool = inv, true
	if !inv.Allowed {
		return result, fmt.Errorf("tool %q is not permitted: %s", call.Name, inv.Reason)
	}
	if inv.Err != nil {
		return result, fmt.Errorf("tool %q failed: %w", call.Name, inv.Err)
	}
	payload, err := toolResultPayload(inv)
	if err != nil {
		return result, err
	}
	assistant := provider.Message{Role: "assistant", ToolCalls: initial.ToolCalls}
	if strings.TrimSpace(initial.Text) != "" {
		assistant.Content = &initial.Text
	}
	final, err := t.Provider.Complete(ctx, provider.Request{
		Messages: []provider.Message{
			textMessage("system", t.System),
			textMessage("user", prompt),
			assistant,
			{Role: "tool", Content: &payload, ToolCallID: call.ID},
		},
		MaxTokens: maxTokens,
	})
	if err != nil {
		return result, err
	}
	if len(final.ToolCalls) != 0 {
		return result, fmt.Errorf("follow-up returned another tool request; one tool turn is the limit")
	}
	result.Final = final
	return result, nil
}

func (t ToolTurn) specs() ([]provider.ToolSpec, map[string]string, error) {
	if len(t.Allowed) == 0 {
		return nil, nil, fmt.Errorf("tool turn requires an explicit non-empty allowlist")
	}
	seen := make(map[string]struct{}, len(t.Allowed))
	allowed := make(map[string]string, len(t.Allowed))
	specs := make([]provider.ToolSpec, 0, len(t.Allowed))
	for _, name := range t.Allowed {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, nil, fmt.Errorf("tool allowlist contains an empty name")
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, nil, fmt.Errorf("tool allowlist contains duplicate %q", name)
		}
		tool, ok := t.Registry.Get(name)
		if !ok {
			return nil, nil, fmt.Errorf("tool allowlist names unregistered tool %q", name)
		}
		seen[name] = struct{}{}
		wireName := toolWireName(name)
		if _, collision := allowed[wireName]; collision {
			return nil, nil, fmt.Errorf("tool allowlist wire name collision for %q", name)
		}
		allowed[wireName] = name
		specs = append(specs, provider.ToolSpec{
			Name:        wireName,
			Description: tool.Does,
			Schema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		})
	}
	return specs, allowed, nil
}

// toolWireName gives a dotted canonical registry name the compact spelling
// expected by the selected local tool template. The turn-local allowlist map
// binds it back to the canonical name; any collision is refused while the
// tool offer is assembled, before the provider is contacted.
func toolWireName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

func toolResultPayload(inv Invocation) (string, error) {
	type result struct {
		Summary  string         `json:"summary"`
		Evidence map[string]any `json:"evidence,omitempty"`
		Changed  bool           `json:"changed"`
	}
	type payload struct {
		Tool     string  `json:"tool"`
		Tier     string  `json:"tier"`
		Verified *result `json:"verified,omitempty"`
		Result   result  `json:"result"`
	}
	p := payload{
		Tool: inv.Tool, Tier: inv.Tier.String(),
		Result: result{Summary: inv.Result.Summary, Evidence: inv.Result.Evidence, Changed: inv.Result.Changed},
	}
	if inv.Verified != nil {
		p.Verified = &result{Summary: inv.Verified.Summary, Evidence: inv.Verified.Evidence, Changed: inv.Verified.Changed}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encode %q tool result: %w", inv.Tool, err)
	}
	return string(b), nil
}
