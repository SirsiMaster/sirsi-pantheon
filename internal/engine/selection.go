package engine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const SelectionSchema = "pantheon.engine-selection/v1"

type ConnectorSummary struct {
	Kind         Kind           `json:"kind"`
	Variant      BackendVariant `json:"variant"`
	Capabilities Capabilities   `json:"capabilities"`
}

// SelectionSnapshot is safe to render in a UI: it describes configured
// connectors and the explicit policy, but does not claim that a backend is
// live until OpenSession performs the normal availability/readiness check.
type SelectionSnapshot struct {
	Schema               string             `json:"schema"`
	Preferred            Kind               `json:"preferred"`
	PreferredVariant     BackendVariant     `json:"preferred_variant,omitempty"`
	AllowFallback        bool               `json:"allow_fallback"`
	RequiredCapabilities []Capability       `json:"required_capabilities,omitempty"`
	Connectors           []ConnectorSummary `json:"connectors"`
}

// SelectionController is the one persistent policy owner used by UI surfaces.
// Routing still occurs through Router.OpenSession, so selecting an engine does
// not bypass capability checks or create a second execution authority.
type SelectionController struct {
	mu     sync.RWMutex
	router *Router
	policy RoutePolicy
}

// PromptRequest is the engine-neutral prompt surface used by Pantheon UI
// integrations. It deliberately contains no endpoint, process, or backend
// configuration; the controller supplies the selected connector and binds the
// resulting receipt to the admitted session identity.
type PromptRequest struct {
	Model       string
	System      string
	Prompt      string
	MaxTokens   int
	Temperature *float64
	TopP        *float64
	Seed        *int64
}

// CompletePrompt opens one session under the current explicit route policy
// and completes the request through the same Router used by all other engine
// surfaces. An empty Model means "use the selected connector's admitted
// model"; a non-empty value is checked before provider execution.
func (c *SelectionController) CompletePrompt(ctx context.Context, request PromptRequest) (Completion, Receipt, error) {
	if c == nil || c.router == nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine selection: controller is required")
	}
	if ctx == nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine selection: context is required")
	}
	policy := c.Policy()
	sessionID := fmt.Sprintf("pantheon-prompt-%d", time.Now().UTC().UnixNano())
	session, decision, err := c.router.OpenSession(ctx, sessionID, policy)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if request.Model != "" && request.Model != session.Identity.ModelID {
		return Completion{}, Receipt{}, fmt.Errorf("engine selection: requested model %q does not match admitted model %q", request.Model, session.Identity.ModelID)
	}
	generation := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, System: request.System,
		Prompt: request.Prompt, MaxTokens: request.MaxTokens,
		Temperature: request.Temperature, TopP: request.TopP, Seed: request.Seed,
		CacheNamespace:       session.Identity.CacheNamespace,
		RequiredCapabilities: append([]Capability(nil), policy.RequiredCapabilities...),
	}
	return c.router.Complete(ctx, session, generation, decision)
}

func NewSelectionController(router *Router, policy RoutePolicy) (*SelectionController, error) {
	if router == nil {
		return nil, fmt.Errorf("engine selection: router is required")
	}
	if err := validateSelectionPolicy(router, policy); err != nil {
		return nil, err
	}
	return &SelectionController{router: router, policy: cloneRoutePolicy(policy)}, nil
}

func (c *SelectionController) Select(policy RoutePolicy) (SelectionSnapshot, error) {
	if c == nil || c.router == nil {
		return SelectionSnapshot{}, fmt.Errorf("engine selection: controller is required")
	}
	if err := validateSelectionPolicy(c.router, policy); err != nil {
		return SelectionSnapshot{}, err
	}
	c.mu.Lock()
	c.policy = cloneRoutePolicy(policy)
	c.mu.Unlock()
	return c.Snapshot(), nil
}

func (c *SelectionController) Policy() RoutePolicy {
	if c == nil {
		return RoutePolicy{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneRoutePolicy(c.policy)
}

func (c *SelectionController) Snapshot() SelectionSnapshot {
	if c == nil || c.router == nil {
		return SelectionSnapshot{}
	}
	c.mu.RLock()
	policy := cloneRoutePolicy(c.policy)
	c.mu.RUnlock()
	connectors := make([]ConnectorSummary, 0, len(c.router.connectors))
	for _, kind := range []Kind{KindMLX, KindOMLX, KindSNE} {
		if connector, ok := c.router.connectors[kind]; ok {
			connectors = append(connectors, ConnectorSummary{Kind: kind, Variant: connector.Variant(), Capabilities: connector.Capabilities()})
		}
	}
	return SelectionSnapshot{Schema: SelectionSchema, Preferred: policy.Preferred, PreferredVariant: policy.PreferredVariant, AllowFallback: policy.AllowFallback, RequiredCapabilities: append([]Capability(nil), policy.RequiredCapabilities...), Connectors: connectors}
}

func validateSelectionPolicy(router *Router, policy RoutePolicy) error {
	if policy.Preferred != KindMLX && policy.Preferred != KindOMLX && policy.Preferred != KindSNE {
		return fmt.Errorf("engine selection: preferred engine %q is required", policy.Preferred)
	}
	if policy.PreferredVariant != "" {
		if err := policy.PreferredVariant.ValidateForEngine(policy.Preferred); err != nil {
			return fmt.Errorf("engine selection: preferred variant: %w", err)
		}
	}
	preferred, ok := router.connectors[policy.Preferred]
	if !ok {
		return fmt.Errorf("engine selection: preferred connector %q is not configured", policy.Preferred)
	}
	if !policy.AllowFallback {
		if policy.PreferredVariant != "" && preferred.Variant() != policy.PreferredVariant {
			return fmt.Errorf("engine selection: preferred connector variant %q does not match requested variant %q", preferred.Variant(), policy.PreferredVariant)
		}
		return requireCapabilities(preferred.Capabilities(), policy.RequiredCapabilities)
	}
	for kind, connector := range router.connectors {
		if policy.PreferredVariant != "" && connector.Variant() != policy.PreferredVariant {
			continue
		}
		if requireCapabilities(connector.Capabilities(), policy.RequiredCapabilities) == nil {
			return nil
		}
		_ = kind
	}
	return fmt.Errorf("engine selection: no configured connector satisfies required capabilities")
}

func cloneRoutePolicy(policy RoutePolicy) RoutePolicy {
	policy.RequiredCapabilities = append([]Capability(nil), policy.RequiredCapabilities...)
	return policy
}
