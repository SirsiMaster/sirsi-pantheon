package engine

import (
	"fmt"
	"sync"
)

const SelectionSchema = "pantheon.engine-selection/v1"

type ConnectorSummary struct {
	Kind         Kind         `json:"kind"`
	Capabilities Capabilities `json:"capabilities"`
}

// SelectionSnapshot is safe to render in a UI: it describes configured
// connectors and the explicit policy, but does not claim that a backend is
// live until OpenSession performs the normal availability/readiness check.
type SelectionSnapshot struct {
	Schema               string             `json:"schema"`
	Preferred            Kind               `json:"preferred"`
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
			connectors = append(connectors, ConnectorSummary{Kind: kind, Capabilities: connector.Capabilities()})
		}
	}
	return SelectionSnapshot{Schema: SelectionSchema, Preferred: policy.Preferred, AllowFallback: policy.AllowFallback, RequiredCapabilities: append([]Capability(nil), policy.RequiredCapabilities...), Connectors: connectors}
}

func validateSelectionPolicy(router *Router, policy RoutePolicy) error {
	if policy.Preferred != KindMLX && policy.Preferred != KindOMLX && policy.Preferred != KindSNE {
		return fmt.Errorf("engine selection: preferred engine %q is required", policy.Preferred)
	}
	preferred, ok := router.connectors[policy.Preferred]
	if !ok {
		return fmt.Errorf("engine selection: preferred connector %q is not configured", policy.Preferred)
	}
	if !policy.AllowFallback {
		return requireCapabilities(preferred.Capabilities(), policy.RequiredCapabilities)
	}
	for kind, connector := range router.connectors {
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
