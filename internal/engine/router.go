package engine

import (
	"context"
	"fmt"
	"strings"
)

// RoutePolicy is the caller's explicit engine preference. Fallback is never
// implicit: a caller must opt in, and the returned decision records whether it
// happened so a surface cannot hide an engine change from the user.
type RoutePolicy struct {
	Preferred            Kind         `json:"preferred"`
	AllowFallback        bool         `json:"allow_fallback"`
	RequiredCapabilities []Capability `json:"required_capabilities,omitempty"`
}

type RouteDecision struct {
	Requested Kind   `json:"requested"`
	Selected  Kind   `json:"selected"`
	Fallback  bool   `json:"fallback"`
	Rationale string `json:"rationale"`
}

func (d RouteDecision) validate(selected Kind) error {
	if d.Requested != KindMLX && d.Requested != KindOMLX && d.Requested != KindSNE {
		return fmt.Errorf("engine route: requested engine %q is invalid", d.Requested)
	}
	if d.Selected != selected {
		return fmt.Errorf("engine route: selected engine %q does not match receipt identity %q", d.Selected, selected)
	}
	if d.Selected != KindMLX && d.Selected != KindOMLX && d.Selected != KindSNE {
		return fmt.Errorf("engine route: selected engine %q is invalid", d.Selected)
	}
	if d.Fallback != (d.Requested != d.Selected) {
		return fmt.Errorf("engine route: fallback flag does not match requested/selected engines")
	}
	if strings.TrimSpace(d.Rationale) == "" {
		return fmt.Errorf("engine route: rationale is required")
	}
	return nil
}

// Router is the one engine-neutral selection authority. Connectors are keyed
// by ABI Kind, so MLX/OMLX/SNE cannot silently overwrite one another or route
// through a backend-specific side channel.
type Router struct {
	connectors map[Kind]Connector
}

func NewRouter(connectors ...Connector) (*Router, error) {
	if len(connectors) == 0 {
		return nil, fmt.Errorf("engine router: at least one connector is required")
	}
	byKind := make(map[Kind]Connector, len(connectors))
	for _, connector := range connectors {
		if connector == nil {
			return nil, fmt.Errorf("engine router: nil connector")
		}
		kind := connector.Kind()
		if kind != KindMLX && kind != KindOMLX && kind != KindSNE {
			return nil, fmt.Errorf("engine router: unsupported connector kind %q", kind)
		}
		if _, exists := byKind[kind]; exists {
			return nil, fmt.Errorf("engine router: duplicate connector kind %q", kind)
		}
		byKind[kind] = connector
	}
	return &Router{connectors: byKind}, nil
}

func (r *Router) OpenSession(ctx context.Context, sessionID string, policy RoutePolicy) (Session, RouteDecision, error) {
	if r == nil || len(r.connectors) == 0 {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: no connectors configured")
	}
	if err := ctx.Err(); err != nil {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: session admission cancelled before routing: %w", err)
	}
	if policy.Preferred != KindMLX && policy.Preferred != KindOMLX && policy.Preferred != KindSNE {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: preferred engine %q is required", policy.Preferred)
	}
	order := r.candidateOrder(policy.Preferred)
	reasons := make([]string, 0, len(order))
	capabilityFailures := 0
	for index, kind := range order {
		if index > 0 && !policy.AllowFallback {
			break
		}
		connector, configured := r.connectors[kind]
		if !configured {
			reasons = append(reasons, fmt.Sprintf("%s: connector is not configured", kind))
			continue
		}
		if err := requireCapabilities(connector.Capabilities(), policy.RequiredCapabilities); err != nil {
			capabilityFailures++
			reasons = append(reasons, fmt.Sprintf("%s: %v", kind, err))
			continue
		}
		session, err := connector.OpenSession(ctx, sessionID)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: session admission cancelled after %s connector: %w", kind, ctxErr)
		}
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("%s: %v", kind, err))
			continue
		}
		if session.ID != sessionID {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector returned session %q for requested session %q", kind, session.ID, sessionID)
		}
		if err := session.Validate(); err != nil {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector returned invalid session: %w", kind, err)
		}
		if session.Identity.Engine != kind {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector returned identity for %s", kind, session.Identity.Engine)
		}
		decision := RouteDecision{Requested: policy.Preferred, Selected: kind, Fallback: index > 0}
		if index == 0 {
			decision.Rationale = fmt.Sprintf("preferred %s connector admitted", kind)
		} else {
			decision.Rationale = fmt.Sprintf("preferred %s unavailable; explicit fallback selected %s", policy.Preferred, kind)
		}
		return session, decision, nil
	}
	if capabilityFailures > 0 && capabilityFailures == len(reasons) {
		return Session{}, RouteDecision{}, fmt.Errorf("%w: no connector admitted for %s: %s", ErrUnsupportedCapability, policy.Preferred, strings.Join(reasons, "; "))
	}
	return Session{}, RouteDecision{}, fmt.Errorf("engine router: no connector admitted for %s: %s", policy.Preferred, strings.Join(reasons, "; "))
}

func (r *Router) Complete(ctx context.Context, session Session, request GenerateRequest, decision RouteDecision) (Completion, Receipt, error) {
	connector, err := r.connectorForDecision(session, decision)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion cancelled before connector: %w", err)
	}
	completion, receipt, err := connector.Complete(ctx, session, request)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion cancelled after connector: %w", ctxErr)
	}
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	route := decision
	receipt.Route = &route
	if err := receipt.Validate(session); err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion receipt: %w", err)
	}
	return completion, receipt, err
}

func (r *Router) Stream(ctx context.Context, session Session, request GenerateRequest, decision RouteDecision) (<-chan Event, error) {
	connector, err := r.connectorForDecision(session, decision)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("engine router: stream cancelled before connector: %w", err)
	}
	events, err := connector.Stream(ctx, session, request)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("engine router: stream cancelled after connector: %w", ctxErr)
	}
	if err != nil {
		return nil, err
	}
	if events == nil {
		return nil, fmt.Errorf("engine router: connector returned a nil stream")
	}
	routed := make(chan Event, 1)
	go func() {
		defer close(routed)
		var previous uint64
		for event := range events {
			if event.Receipt != nil {
				route := decision
				event.Receipt.Route = &route
				if err := event.Receipt.Validate(session); err != nil {
					routerEmitError(ctx, routed, previous, session.ID, err)
					return
				}
			}
			if err := event.Validate(previous); err != nil {
				routerEmitError(ctx, routed, previous, session.ID, err)
				return
			}
			previous = event.Sequence
			select {
			case routed <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return routed, nil
}

func (r *Router) connectorForDecision(session Session, decision RouteDecision) (Connector, error) {
	if r == nil {
		return nil, fmt.Errorf("engine router: nil router")
	}
	if err := decision.validate(session.Identity.Engine); err != nil {
		return nil, fmt.Errorf("engine router: invalid route decision: %w", err)
	}
	if decision.Selected != session.Identity.Engine {
		return nil, fmt.Errorf("engine router: decision %q does not match session engine %q", decision.Selected, session.Identity.Engine)
	}
	connector, ok := r.connectors[decision.Selected]
	if !ok {
		return nil, fmt.Errorf("engine router: selected connector %q is not configured", decision.Selected)
	}
	return connector, nil
}

func routerEmitError(ctx context.Context, events chan<- Event, previous uint64, sessionID string, err error) {
	event := Event{
		Kind:      EventError,
		SessionID: sessionID,
		Sequence:  previous + 1,
		ErrorCode: "stream_event_invalid",
		Error:     err.Error(),
	}
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func (r *Router) candidateOrder(preferred Kind) []Kind {
	order := []Kind{preferred}
	for _, kind := range []Kind{KindMLX, KindOMLX, KindSNE} {
		if kind != preferred {
			if _, ok := r.connectors[kind]; ok {
				order = append(order, kind)
			}
		}
	}
	return order
}

func requireCapabilities(actual Capabilities, required []Capability) error {
	seen := make(map[Capability]struct{}, len(required))
	for _, capability := range required {
		if _, ok := seen[capability]; ok {
			return fmt.Errorf("duplicate required capability %q", capability)
		}
		seen[capability] = struct{}{}
		if !actual.Has(capability) {
			return fmt.Errorf("%w: %s", ErrUnsupportedCapability, capability)
		}
	}
	return nil
}
