package engine

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const SelectionSchema = "pantheon.engine-selection/v2"

type ConnectorSummary struct {
	DisplayName            string         `json:"display_name"`
	Kind                   Kind           `json:"kind"`
	Variant                BackendVariant `json:"variant"`
	DataBoundary           string         `json:"data_boundary"`
	Capabilities           Capabilities   `json:"capabilities"`
	ContextualCapabilities []Capability   `json:"contextual_capabilities,omitempty"`
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

var promptSessionSequence atomic.Uint64

func newPromptSessionID(surface string) string {
	return fmt.Sprintf("pantheon-%s-%d-%d", surface, time.Now().UTC().UnixNano(), promptSessionSequence.Add(1))
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

// PromptStream is the admitted session and route paired with its event stream.
// Consumers can render the selected identity immediately instead of waiting
// for the terminal receipt to learn which connector served the request.
type PromptStream struct {
	Session Session
	Route   RouteDecision
	Events  <-chan Event
}

// CompletePrompt opens one session under the current explicit route policy
// and completes the request through the same Router used by all other engine
// surfaces. An empty Model means "use the selected connector's admitted
// model"; a non-empty value is checked before provider execution.
func (c *SelectionController) CompletePrompt(ctx context.Context, request PromptRequest) (Completion, Receipt, error) {
	return c.CompletePromptWithPolicy(ctx, request, c.Policy())
}

// CompletePromptWithPolicy opens one session using the caller's captured
// policy snapshot rather than rereading mutable dashboard preference. It is
// intended for requests whose route was accepted before other operator
// actions may change the current preference.
func (c *SelectionController) CompletePromptWithPolicy(ctx context.Context, request PromptRequest, policy RoutePolicy) (Completion, Receipt, error) {
	if c == nil || c.router == nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine selection: controller is required")
	}
	if ctx == nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine selection: context is required")
	}
	request = snapshotPromptRequest(request)
	if err := validatePromptRequest(request); err != nil {
		return Completion{}, Receipt{}, err
	}
	policy, err := resolveSelectionPolicy(c.router, cloneRoutePolicy(policy))
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	policy.RequiredCapabilities = promptRequiredCapabilities(policy.RequiredCapabilities, request, false)
	sessionID := newPromptSessionID("prompt")
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

// StreamPrompt opens and streams one request through the current explicit
// policy. Streaming is added to session admission requirements so an
// unsupported preferred connector can only be bypassed by policy that already
// permits fallback; it is never discovered after opening a non-streaming
// session.
func (c *SelectionController) StreamPrompt(ctx context.Context, request PromptRequest) (PromptStream, error) {
	if c == nil || c.router == nil {
		return PromptStream{}, fmt.Errorf("engine selection: controller is required")
	}
	if ctx == nil {
		return PromptStream{}, fmt.Errorf("engine selection: context is required")
	}
	request = snapshotPromptRequest(request)
	if err := validatePromptRequest(request); err != nil {
		return PromptStream{}, err
	}
	policy := c.Policy()
	policy.RequiredCapabilities = promptRequiredCapabilities(policy.RequiredCapabilities, request, true)
	sessionID := newPromptSessionID("stream")
	session, decision, err := c.router.OpenSession(ctx, sessionID, policy)
	if err != nil {
		return PromptStream{}, err
	}
	if request.Model != "" && request.Model != session.Identity.ModelID {
		return PromptStream{}, fmt.Errorf("engine selection: requested model %q does not match admitted model %q", request.Model, session.Identity.ModelID)
	}
	generation := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, System: request.System,
		Prompt: request.Prompt, MaxTokens: request.MaxTokens,
		Temperature: request.Temperature, TopP: request.TopP, Seed: request.Seed,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
		RequiredCapabilities: append([]Capability(nil), policy.RequiredCapabilities...),
	}
	events, err := c.router.Stream(ctx, session, generation, decision)
	if err != nil {
		return PromptStream{}, err
	}
	return PromptStream{Session: session, Route: decision, Events: events}, nil
}

func NewSelectionController(router *Router, policy RoutePolicy) (*SelectionController, error) {
	if router == nil {
		return nil, fmt.Errorf("engine selection: router is required")
	}
	policy, err := resolveSelectionPolicy(router, policy)
	if err != nil {
		return nil, err
	}
	return &SelectionController{router: router, policy: cloneRoutePolicy(policy)}, nil
}

func (c *SelectionController) Select(policy RoutePolicy) (SelectionSnapshot, error) {
	if c == nil || c.router == nil {
		return SelectionSnapshot{}, fmt.Errorf("engine selection: controller is required")
	}
	policy, err := resolveSelectionPolicy(c.router, policy)
	if err != nil {
		return SelectionSnapshot{}, err
	}
	c.mu.Lock()
	c.policy = cloneRoutePolicy(policy)
	c.mu.Unlock()
	return c.snapshot(policy), nil
}

// SelectRoute changes only the preferred route while preserving fallback and
// required-capability policy from the same locked snapshot. Dashboard route
// changes use this instead of a client-side read/modify/write sequence.
func (c *SelectionController) SelectRoute(kind Kind, variant BackendVariant) (SelectionSnapshot, error) {
	if c == nil || c.router == nil {
		return SelectionSnapshot{}, fmt.Errorf("engine selection: controller is required")
	}
	c.mu.Lock()
	policy := cloneRoutePolicy(c.policy)
	policy.Preferred = kind
	policy.PreferredVariant = variant
	resolved, err := resolveSelectionPolicy(c.router, policy)
	if err != nil {
		c.mu.Unlock()
		return SelectionSnapshot{}, err
	}
	c.policy = cloneRoutePolicy(resolved)
	c.mu.Unlock()
	return c.snapshot(resolved), nil
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
	return c.snapshot(policy)
}

func (c *SelectionController) snapshot(policy RoutePolicy) SelectionSnapshot {
	connectors := make([]ConnectorSummary, 0, len(c.router.connectors))
	variants := []BackendVariant{VariantMLXRaw, VariantMLXPatched, VariantOMLXPublic, VariantSNEPlain, VariantSNEMTP}
	for _, kind := range []Kind{KindMLX, KindOMLX, KindSNE} {
		for _, variant := range variants {
			if connector, ok := c.router.connectors[connectorKey{kind: kind, variant: variant}]; ok {
				boundary := connectorDataBoundary(connector)
				connectors = append(connectors, ConnectorSummary{
					DisplayName: RouteDisplayName(kind, variant),
					Kind:        kind, Variant: variant, DataBoundary: boundary,
					Capabilities:           connector.Capabilities(),
					ContextualCapabilities: contextualCapabilities(connector),
				})
			}
		}
	}
	return SelectionSnapshot{Schema: SelectionSchema, Preferred: policy.Preferred, PreferredVariant: policy.PreferredVariant, AllowFallback: policy.AllowFallback, RequiredCapabilities: append([]Capability(nil), policy.RequiredCapabilities...), Connectors: connectors}
}

func contextualCapabilities(connector Connector) []Capability {
	resolver, ok := connector.(ContextualCapabilityResolver)
	if !ok {
		return nil
	}
	known := []Capability{
		CapabilityTools, CapabilityTemperature, CapabilityTopP, CapabilitySeed,
		CapabilityStreaming, CapabilityCancellation, CapabilityPrefill,
		CapabilityDecode, CapabilityMTP, CapabilityKVState, CapabilityTelemetry,
	}
	var contextual []Capability
	for _, capability := range known {
		if !connector.Capabilities().Has(capability) && resolver.CanResolveCapability(capability) {
			contextual = append(contextual, capability)
		}
	}
	return contextual
}

func resolveSelectionPolicy(router *Router, policy RoutePolicy) (RoutePolicy, error) {
	if policy.Preferred != KindMLX && policy.Preferred != KindOMLX && policy.Preferred != KindSNE {
		return RoutePolicy{}, fmt.Errorf("engine selection: preferred engine %q is required", policy.Preferred)
	}
	variant, err := router.resolveVariant(policy.Preferred, policy.PreferredVariant)
	if err != nil {
		return RoutePolicy{}, fmt.Errorf("engine selection: %w", err)
	}
	policy.PreferredVariant = variant
	if err := validateSelectionPolicy(router, policy); err != nil {
		return RoutePolicy{}, err
	}
	return policy, nil
}

func validateSelectionPolicy(router *Router, policy RoutePolicy) error {
	preferred, ok := router.connectors[connectorKey{kind: policy.Preferred, variant: policy.PreferredVariant}]
	if !ok {
		return fmt.Errorf("engine selection: preferred connector %q variant %q is not configured", policy.Preferred, policy.PreferredVariant)
	}
	if preferred.Variant() == VariantSNEMTP && policy.AllowFallback {
		return fmt.Errorf("engine selection: explicit sne-mtp routes cannot fall back or downgrade execution mode")
	}
	requiredCapabilities := sessionRequiredCapabilities(policy.RequiredCapabilities)
	for _, capability := range variantRequiredCapabilities(preferred.Variant()) {
		requiredCapabilities = includeCapability(requiredCapabilities, capability)
	}
	preferredErr := requireCapabilities(preferred.Capabilities(), requiredCapabilities)
	if preferredErr == nil {
		return nil
	}
	if resolver, ok := preferred.(ContextualCapabilityResolver); ok && capabilityRequirementsResolvable(preferred.Capabilities(), requiredCapabilities, resolver) {
		return nil
	}
	if !policy.AllowFallback {
		return preferredErr
	}
	for _, kind := range []Kind{KindMLX, KindOMLX, KindSNE} {
		if kind == policy.Preferred {
			continue
		}
		variant, err := router.resolveVariant(kind, "")
		if err != nil {
			continue
		}
		connector, configured := router.connectors[connectorKey{kind: kind, variant: variant}]
		if !configured {
			continue
		}
		candidateRequirements := append([]Capability(nil), sessionRequiredCapabilities(policy.RequiredCapabilities)...)
		for _, capability := range variantRequiredCapabilities(connector.Variant()) {
			candidateRequirements = includeCapability(candidateRequirements, capability)
		}
		capabilities := connector.Capabilities()
		if requireCapabilities(capabilities, candidateRequirements) == nil {
			return nil
		}
		if resolver, ok := connector.(ContextualCapabilityResolver); ok && capabilityRequirementsResolvable(capabilities, candidateRequirements, resolver) {
			return nil
		}
	}
	return fmt.Errorf("engine selection: preferred connector cannot satisfy required capabilities (%v), and no configured fallback satisfies them", preferredErr)
}

func cloneRoutePolicy(policy RoutePolicy) RoutePolicy {
	policy.RequiredCapabilities = append([]Capability(nil), policy.RequiredCapabilities...)
	return policy
}

func includeCapability(required []Capability, capability Capability) []Capability {
	out := append([]Capability(nil), required...)
	for _, existing := range out {
		if existing == capability {
			return out
		}
	}
	return append(out, capability)
}

func promptRequiredCapabilities(required []Capability, request PromptRequest, streaming bool) []Capability {
	required = includeCapability(required, CapabilityReceipts)
	if request.Temperature != nil {
		required = includeCapability(required, CapabilityTemperature)
	}
	if request.TopP != nil {
		required = includeCapability(required, CapabilityTopP)
	}
	if request.Seed != nil {
		required = includeCapability(required, CapabilitySeed)
	}
	if streaming {
		required = includeCapability(required, CapabilityStreaming)
		required = includeCapability(required, CapabilityCancellation)
	}
	return required
}

func snapshotPromptRequest(request PromptRequest) PromptRequest {
	request.Temperature = copyFloat64(request.Temperature)
	request.TopP = copyFloat64(request.TopP)
	request.Seed = copyInt64(request.Seed)
	return request
}

func validatePromptRequest(request PromptRequest) error {
	if strings.TrimSpace(request.Prompt) == "" {
		return fmt.Errorf("engine selection: prompt is required")
	}
	if request.MaxTokens <= 0 {
		return fmt.Errorf("engine selection: max tokens must be positive")
	}
	if request.Temperature != nil && (math.IsNaN(*request.Temperature) || math.IsInf(*request.Temperature, 0) || *request.Temperature < 0 || *request.Temperature > 2) {
		return fmt.Errorf("engine selection: temperature must be finite and within [0,2]")
	}
	if request.TopP != nil && (math.IsNaN(*request.TopP) || math.IsInf(*request.TopP, 0) || *request.TopP <= 0 || *request.TopP > 1) {
		return fmt.Errorf("engine selection: top_p must be finite and within (0,1]")
	}
	return nil
}
