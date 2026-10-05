package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var errVariantSelectionRequired = errors.New("engine router: explicit variant selection is required")

const (
	DataBoundaryOnDevice     = "on-device"
	DataBoundaryRemote       = "remote"
	DataBoundaryNotDisclosed = "not-disclosed"
)

// RoutePolicy is the caller's explicit engine preference. Fallback is never
// implicit: a caller must opt in, and the returned decision records whether it
// happened so a surface cannot hide an engine change from the user.
type RoutePolicy struct {
	Preferred            Kind           `json:"preferred"`
	PreferredVariant     BackendVariant `json:"preferred_variant,omitempty"`
	AllowFallback        bool           `json:"allow_fallback"`
	RequiredCapabilities []Capability   `json:"required_capabilities,omitempty"`
}

type RouteDecision struct {
	Requested        Kind           `json:"requested"`
	RequestedVariant BackendVariant `json:"requested_variant,omitempty"`
	Selected         Kind           `json:"selected"`
	SelectedVariant  BackendVariant `json:"selected_variant,omitempty"`
	// DataBoundary is the normalized configured-endpoint classification, not
	// a runtime trace of proxies or other transport intermediaries.
	DataBoundary string `json:"data_boundary,omitempty"`
	Fallback     bool   `json:"fallback"`
	Rationale    string `json:"rationale"`
}

func connectorDataBoundary(connector Connector) string {
	reporter, ok := connector.(interface{ RequestDataBoundary() string })
	if !ok {
		return DataBoundaryNotDisclosed
	}
	switch strings.TrimSpace(reporter.RequestDataBoundary()) {
	case DataBoundaryOnDevice:
		return DataBoundaryOnDevice
	case DataBoundaryRemote:
		return DataBoundaryRemote
	default:
		return DataBoundaryNotDisclosed
	}
}

func bindConnectorDataBoundary(decision RouteDecision, connector Connector) (RouteDecision, error) {
	actual := connectorDataBoundary(connector)
	if decision.DataBoundary != "" && decision.DataBoundary != actual {
		return RouteDecision{}, fmt.Errorf("engine route: data boundary %q does not match admitted connector boundary %q", decision.DataBoundary, actual)
	}
	decision.DataBoundary = actual
	return decision, nil
}

func (d RouteDecision) validate(selected Kind, selectedVariant BackendVariant) error {
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
	if d.DataBoundary != "" && d.DataBoundary != DataBoundaryOnDevice && d.DataBoundary != DataBoundaryRemote && d.DataBoundary != DataBoundaryNotDisclosed {
		return fmt.Errorf("engine route: data boundary %q is invalid", d.DataBoundary)
	}
	if strings.TrimSpace(d.Rationale) == "" {
		return fmt.Errorf("engine route: rationale is required")
	}
	if d.RequestedVariant != "" {
		if err := d.RequestedVariant.ValidateForEngine(d.Requested); err != nil {
			return fmt.Errorf("engine route: requested variant: %w", err)
		}
	}
	if d.SelectedVariant != "" {
		if err := d.SelectedVariant.ValidateForEngine(d.Selected); err != nil {
			return fmt.Errorf("engine route: selected variant: %w", err)
		}
		if d.SelectedVariant != selectedVariant {
			return fmt.Errorf("engine route: selected variant %q does not match connector %q", d.SelectedVariant, selectedVariant)
		}
	}
	if d.Selected == d.Requested && d.RequestedVariant != "" && d.RequestedVariant != selectedVariant {
		return fmt.Errorf("engine route: selected variant %q does not match requested variant %q", selectedVariant, d.RequestedVariant)
	}
	return nil
}

// Validate binds a public route decision to the identity it claims to have
// selected. Surface adapters use this before publishing route metadata.
func (d RouteDecision) Validate(identity Identity) error {
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("engine route identity: %w", err)
	}
	return d.validate(identity.Engine, identity.EffectiveVariant())
}

// Router is the one engine-neutral selection authority. Connectors are keyed
// by ABI Kind and BackendVariant, so variants can coexist without silently
// overwriting one another or routing through a backend-specific side channel.
type Router struct {
	connectors map[connectorKey]Connector
}

type connectorKey struct {
	kind    Kind
	variant BackendVariant
}

func NewRouter(connectors ...Connector) (*Router, error) {
	if len(connectors) == 0 {
		return nil, fmt.Errorf("engine router: at least one connector is required")
	}
	byVariant := make(map[connectorKey]Connector, len(connectors))
	for _, connector := range connectors {
		if connector == nil {
			return nil, fmt.Errorf("engine router: nil connector")
		}
		kind := connector.Kind()
		if kind != KindMLX && kind != KindOMLX && kind != KindSNE {
			return nil, fmt.Errorf("engine router: unsupported connector kind %q", kind)
		}
		if err := connector.Variant().ValidateForEngine(kind); err != nil {
			return nil, fmt.Errorf("engine router: %s connector has invalid variant: %w", kind, err)
		}
		key := connectorKey{kind: kind, variant: connector.Variant()}
		if _, exists := byVariant[key]; exists {
			return nil, fmt.Errorf("engine router: duplicate connector %q variant %q", kind, key.variant)
		}
		byVariant[key] = connector
	}
	return &Router{connectors: byVariant}, nil
}

func (r *Router) OpenSession(ctx context.Context, sessionID string, policy RoutePolicy) (Session, RouteDecision, error) {
	if r == nil || len(r.connectors) == 0 {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: no connectors configured")
	}
	if ctx == nil {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: session admission cancelled before routing: %w", err)
	}
	if policy.Preferred != KindMLX && policy.Preferred != KindOMLX && policy.Preferred != KindSNE {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: preferred engine %q is required", policy.Preferred)
	}
	if policy.PreferredVariant != "" {
		if err := policy.PreferredVariant.ValidateForEngine(policy.Preferred); err != nil {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: preferred variant: %w", err)
		}
	}
	preferredVariant := policy.PreferredVariant
	if preferredVariant == "" {
		var err error
		preferredVariant, err = r.resolveVariant(policy.Preferred, "")
		if err != nil {
			if !policy.AllowFallback || errors.Is(err, errVariantSelectionRequired) {
				return Session{}, RouteDecision{}, err
			}
		}
	}
	if preferredVariant == VariantSNEMTP && policy.AllowFallback {
		return Session{}, RouteDecision{}, fmt.Errorf("engine router: explicit sne-mtp routes cannot fall back or downgrade execution mode")
	}
	order := r.candidateOrder(policy.Preferred, preferredVariant)
	reasons := make([]string, 0, len(order))
	capabilityFailures := 0
	requiredCapabilities := sessionRequiredCapabilities(policy.RequiredCapabilities)
	for index, kind := range order {
		if index > 0 && !policy.AllowFallback {
			break
		}
		candidateVariant := variantForCandidate(kind, policy.Preferred, preferredVariant)
		connector, configured := r.connectors[connectorKey{kind: kind, variant: candidateVariant}]
		if !configured && kind != policy.Preferred {
			candidateVariant, _ = r.resolveVariant(kind, "")
			connector, configured = r.connectors[connectorKey{kind: kind, variant: candidateVariant}]
		}
		if !configured {
			reasons = append(reasons, fmt.Sprintf("%s variant %s: connector is not configured", kind, candidateVariant))
			continue
		}
		candidateRequirements := append([]Capability(nil), requiredCapabilities...)
		for _, capability := range variantRequiredCapabilities(candidateVariant) {
			candidateRequirements = includeCapability(candidateRequirements, capability)
		}
		candidateCapabilities := connector.Capabilities()
		capabilityErr := requireCapabilities(candidateCapabilities, candidateRequirements)
		if capabilityErr != nil {
			resolver, canResolve := connector.(ContextualCapabilityResolver)
			if canResolve && capabilityRequirementsResolvable(candidateCapabilities, candidateRequirements, resolver) {
				resolved, resolveErr := resolver.CapabilitiesForContext(ctx)
				if ctxErr := ctx.Err(); ctxErr != nil {
					return Session{}, RouteDecision{}, fmt.Errorf("engine router: contextual capability admission cancelled: %w", ctxErr)
				}
				if resolveErr != nil {
					reasons = append(reasons, fmt.Sprintf("%s readiness: %v", kind, resolveErr))
					continue
				}
				candidateCapabilities = resolved
				capabilityErr = requireCapabilities(candidateCapabilities, candidateRequirements)
			}
		}
		if capabilityErr != nil {
			capabilityFailures++
			reasons = append(reasons, fmt.Sprintf("%s: %v", kind, capabilityErr))
			continue
		}
		variant := connector.Variant()
		if err := variant.ValidateForEngine(kind); err != nil {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector has invalid variant: %w", kind, err)
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
		session.Identity.Variant = session.Identity.EffectiveVariant()
		if err := session.Validate(); err != nil {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector returned invalid session: %w", kind, err)
		}
		if session.Identity.Engine != kind {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector returned identity for %s", kind, session.Identity.Engine)
		}
		if session.Identity.EffectiveVariant() != variant {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: %s connector returned variant %q, want %q", kind, session.Identity.EffectiveVariant(), variant)
		}
		decision := RouteDecision{Requested: policy.Preferred, RequestedVariant: policy.PreferredVariant, Selected: kind, SelectedVariant: variant, DataBoundary: connectorDataBoundary(connector), Fallback: kind != policy.Preferred}
		if kind == policy.Preferred {
			decision.Rationale = fmt.Sprintf("preferred %s connector admitted", kind)
		} else {
			decision.Rationale = fmt.Sprintf("preferred %s unavailable; explicit fallback selected %s", policy.Preferred, kind)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Session{}, RouteDecision{}, fmt.Errorf("engine router: session admission cancelled before returning %s connector: %w", kind, ctxErr)
		}
		return session, decision, nil
	}
	if capabilityFailures > 0 && capabilityFailures == len(reasons) {
		return Session{}, RouteDecision{}, fmt.Errorf("%w: no connector admitted for %s: %s", ErrUnsupportedCapability, policy.Preferred, strings.Join(reasons, "; "))
	}
	return Session{}, RouteDecision{}, fmt.Errorf("engine router: no connector admitted for %s: %s", policy.Preferred, strings.Join(reasons, "; "))
}

func (r *Router) Complete(ctx context.Context, session Session, request GenerateRequest, decision RouteDecision) (Completion, Receipt, error) {
	if ctx == nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: context is required")
	}
	if request.Stream {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: Complete requires request stream=false")
	}
	snapshot, expectedRequestDigest, err := SnapshotGenerateRequest(request)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	request = snapshot
	connector, err := r.connectorForDecision(session, decision)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	decision, err = bindConnectorDataBoundary(decision, connector)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if err := validateRoutedRequest(ctx, connector, session, request); err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: request rejected before connector: %w", err)
	}
	connectorRequest, _, err := SnapshotGenerateRequest(request)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion cancelled before connector: %w", err)
	}
	completion, receipt, err := connector.Complete(ctx, session, connectorRequest)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion cancelled after connector: %w", ctxErr)
	}
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if completion.Model != session.Identity.ModelID {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: served model %q does not match admitted model %q", completion.Model, session.Identity.ModelID)
	}
	if completion.PromptTokens < 0 || completion.OutputTokens < 0 {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion token counts must be non-negative (prompt=%d output=%d)", completion.PromptTokens, completion.OutputTokens)
	}
	if receipt.RequestSHA256 != expectedRequestDigest {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion receipt request hash %q does not match admitted request %q", receipt.RequestSHA256, expectedRequestDigest)
	}
	if want := completionDigest(completion.Text); receipt.CompletionSHA256 != want {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion receipt text hash %q does not match returned completion %q", receipt.CompletionSHA256, want)
	}
	route := decision
	receipt.Route = &route
	if err := receipt.Validate(session); err != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion receipt: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Completion{}, Receipt{}, fmt.Errorf("engine router: completion cancelled before accepting verified result: %w", ctxErr)
	}
	return completion, receipt, err
}

func (r *Router) Stream(ctx context.Context, session Session, request GenerateRequest, decision RouteDecision) (<-chan Event, error) {
	if ctx == nil {
		return nil, fmt.Errorf("engine router: context is required")
	}
	if !request.Stream {
		return nil, fmt.Errorf("engine router: Stream requires request stream=true")
	}
	snapshot, expectedRequestDigest, err := SnapshotGenerateRequest(request)
	if err != nil {
		return nil, err
	}
	request = snapshot
	connector, err := r.connectorForDecision(session, decision)
	if err != nil {
		return nil, err
	}
	decision, err = bindConnectorDataBoundary(decision, connector)
	if err != nil {
		return nil, err
	}
	if err := validateRoutedRequest(ctx, connector, session, request); err != nil {
		return nil, fmt.Errorf("engine router: request rejected before connector: %w", err)
	}
	connectorRequest, _, err := SnapshotGenerateRequest(request)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("engine router: stream cancelled before connector: %w", err)
	}
	streamStartedAt := time.Now().UTC()
	events, err := connector.Stream(ctx, session, connectorRequest)
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
		var forwarded strings.Builder
		sawDelta := false
		terminal := false
		for {
			var event Event
			var ok bool
			select {
			case <-ctx.Done():
				routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
				return
			case event, ok = <-events:
				if !ok {
					if !terminal {
						if ctx.Err() != nil {
							routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
						} else {
							routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("stream ended before a terminal event"))
						}
					}
					return
				}
			}
			if event.SessionID != session.ID {
				if ctx.Err() != nil {
					routerEmitCancellation(routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
				} else {
					routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("stream event session %q does not match admitted session %q", event.SessionID, session.ID))
				}
				return
			}
			if event.Receipt != nil {
				if event.Receipt.RequestSHA256 != expectedRequestDigest {
					if ctx.Err() != nil {
						routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
					} else {
						routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("stream receipt request hash %q does not match admitted request %q", event.Receipt.RequestSHA256, expectedRequestDigest))
					}
					return
				}
				if event.Kind == EventCompleted {
					if sawDelta && event.Text != forwarded.String() {
						if ctx.Err() != nil {
							routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
						} else {
							routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("completed stream text does not match forwarded deltas"))
						}
						return
					}
					if want := completionDigest(event.Text); event.Receipt.CompletionSHA256 != want {
						if ctx.Err() != nil {
							routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
						} else {
							routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("stream receipt text hash %q does not match completed text %q", event.Receipt.CompletionSHA256, want))
						}
						return
					}
				}
				if event.Kind == EventError {
					if want := completionDigest(forwarded.String()); event.Receipt.CompletionSHA256 != want {
						if ctx.Err() != nil {
							routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
						} else {
							routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("error stream receipt text hash %q does not match forwarded text %q", event.Receipt.CompletionSHA256, want))
						}
						return
					}
				}
				route := decision
				event.Receipt.Route = &route
				if err := event.Receipt.Validate(session); err != nil {
					if ctx.Err() != nil {
						routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
					} else {
						routerEmitError(ctx, routed, previous, session.ID, err)
					}
					return
				}
			}
			if event.Kind == EventCompleted && event.Model != session.Identity.ModelID {
				if ctx.Err() != nil {
					routerEmitCancellation(routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
				} else {
					routerEmitError(ctx, routed, previous, session.ID, fmt.Errorf("stream served model %q does not match admitted model %q", event.Model, session.Identity.ModelID))
				}
				return
			}
			if err := event.Validate(previous); err != nil {
				if ctx.Err() != nil {
					routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
				} else {
					routerEmitError(ctx, routed, previous, session.ID, err)
				}
				return
			}
			if event.Kind == EventCompleted || event.Kind == EventError {
				terminal = true
			}
			if ctx.Err() != nil {
				if routerEventHasCancellationReceipt(event) {
					routerPublishTerminal(routed, event, func(pending Event) Event {
						return routerRebaseCancellationForDroppedDelta(event, pending, session, request, decision, forwarded.String(), streamStartedAt)
					})
				} else {
					routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
				}
				return
			}
			select {
			case routed <- event:
			case <-ctx.Done():
				routerEmitConnectorCancellation(events, routed, previous, session, request, decision, forwarded.String(), streamStartedAt, ctx.Err())
				return
			}
			previous = event.Sequence
			if event.Kind == EventDelta {
				sawDelta = true
				forwarded.WriteString(event.Text)
			}
			if terminal {
				return
			}
		}
	}()
	return routed, nil
}

func completionDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func routerEmitCancellation(events chan Event, previous uint64, session Session, request GenerateRequest, decision RouteDecision, forwarded string, started time.Time, cause error) {
	message := "stream cancelled"
	if cause != nil {
		message = cause.Error()
	}
	event := Event{Kind: EventError, SessionID: session.ID, Sequence: previous + 1, ErrorCode: "cancelled", Error: message}
	finished := time.Now().UTC()
	if finished.Before(started) {
		finished = started
	}
	receipt, err := streamReceipt(session, request, forwarded, started, finished, true)
	if err == nil {
		route := decision
		receipt.Route = &route
		if receipt.Validate(session) == nil {
			event.Receipt = &receipt
		} else {
			event.ErrorCode = "cancellation_receipt_invalid"
			event.Error = "router could not validate its cancellation receipt"
		}
	} else {
		event.ErrorCode = "cancellation_receipt_invalid"
		event.Error = fmt.Sprintf("router could not create cancellation receipt: %v", err)
	}
	routerPublishTerminal(events, event, func(pending Event) Event {
		return routerRebaseCancellationForDroppedDelta(event, pending, session, request, decision, forwarded, started)
	})
}

func routerEmitConnectorCancellation(source <-chan Event, destination chan Event, previous uint64, session Session, request GenerateRequest, decision RouteDecision, forwarded string, started time.Time, cause error) {
	select {
	case event, ok := <-source:
		expectedRequestDigest, digestErr := GenerateRequestDigest(request)
		if ok && digestErr == nil && event.SessionID == session.ID && routerEventHasCancellationReceipt(event) && event.Receipt.RequestSHA256 == expectedRequestDigest && event.Receipt.CompletionSHA256 == completionDigest(forwarded) {
			if event.Receipt != nil {
				route := decision
				event.Receipt.Route = &route
			}
			if event.Validate(previous) == nil && (event.Receipt == nil || event.Receipt.Validate(session) == nil) {
				routerPublishTerminal(destination, event, func(pending Event) Event {
					return routerRebaseCancellationForDroppedDelta(event, pending, session, request, decision, forwarded, started)
				})
				return
			}
		}
	default:
	}
	routerEmitCancellation(destination, previous, session, request, decision, forwarded, started, cause)
}

func routerEventHasCancellationReceipt(event Event) bool {
	return event.Kind == EventError && event.ErrorCode == "cancelled" && event.Receipt != nil && event.Receipt.Cancelled
}

func routerRebaseCancellationForDroppedDelta(event, pending Event, session Session, request GenerateRequest, decision RouteDecision, forwarded string, started time.Time) Event {
	if !routerEventHasCancellationReceipt(event) || pending.Kind != EventDelta || pending.Text == "" {
		return event
	}
	if !strings.HasSuffix(forwarded, pending.Text) {
		event.Receipt = nil
		event.ErrorCode = "cancellation_receipt_invalid"
		event.Error = "router could not reconcile cancellation receipt with buffered delta"
		return event
	}
	startedAt, err := time.Parse(time.RFC3339Nano, event.Receipt.StartedAt)
	if err != nil {
		startedAt = started
	}
	delivered := strings.TrimSuffix(forwarded, pending.Text)
	finishedAt := time.Now().UTC()
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	receipt, err := streamReceipt(session, request, delivered, startedAt, finishedAt, true)
	if err != nil {
		event.Receipt = nil
		event.ErrorCode = "cancellation_receipt_invalid"
		event.Error = fmt.Sprintf("router could not create reconciled cancellation receipt: %v", err)
		return event
	}
	route := decision
	receipt.Route = &route
	event.Receipt = &receipt
	return event
}

func routerPublishTerminal(events chan Event, event Event, rebase func(Event) Event) {
	select {
	case events <- event:
		return
	default:
	}
	// Cancellation is terminal. If an unread delta occupies the one-slot
	// forwarding buffer, replace it so consumers still observe termination.
	select {
	case pending := <-events:
		if rebase != nil {
			event = rebase(pending)
		}
	default:
	}
	select {
	case events <- event:
	default:
	}
}

func (r *Router) connectorForDecision(session Session, decision RouteDecision) (Connector, error) {
	if r == nil {
		return nil, fmt.Errorf("engine router: nil router")
	}
	if decision.Selected != session.Identity.Engine {
		return nil, fmt.Errorf("engine router: decision %q does not match session engine %q", decision.Selected, session.Identity.Engine)
	}
	selectedVariant := decision.SelectedVariant
	if selectedVariant == "" {
		selectedVariant = DefaultVariant(decision.Selected)
	}
	connector, ok := r.connectors[connectorKey{kind: decision.Selected, variant: selectedVariant}]
	if !ok {
		return nil, fmt.Errorf("engine router: selected connector %q is not configured", decision.Selected)
	}
	if err := decision.validate(session.Identity.Engine, connector.Variant()); err != nil {
		return nil, fmt.Errorf("engine router: invalid route decision: %w", err)
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

func (r *Router) candidateOrder(preferred Kind, preferredVariant BackendVariant) []Kind {
	order := []Kind{preferred}
	for _, kind := range []Kind{KindMLX, KindOMLX, KindSNE} {
		if kind != preferred {
			configured := false
			for key := range r.connectors {
				if key.kind == kind {
					configured = true
					break
				}
			}
			if configured {
				order = append(order, kind)
			}
		}
	}
	return order
}

func variantForCandidate(kind, preferred Kind, preferredVariant BackendVariant) BackendVariant {
	if kind == preferred {
		return preferredVariant
	}
	return DefaultVariant(kind)
}

func (r *Router) resolveVariant(kind Kind, requested BackendVariant) (BackendVariant, error) {
	if requested != "" {
		if _, ok := r.connectors[connectorKey{kind: kind, variant: requested}]; !ok {
			return "", fmt.Errorf("engine router: connector %q variant %q is not configured", kind, requested)
		}
		return requested, nil
	}
	defaultVariant := DefaultVariant(kind)
	if _, ok := r.connectors[connectorKey{kind: kind, variant: defaultVariant}]; ok {
		return defaultVariant, nil
	}
	configured := false
	for key := range r.connectors {
		if key.kind == kind {
			configured = true
			break
		}
	}
	if !configured {
		return "", fmt.Errorf("engine router: connector %q is not configured", kind)
	}
	return "", fmt.Errorf("%w for engine %q without its default variant %q", errVariantSelectionRequired, kind, defaultVariant)
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

func capabilityRequirementsResolvable(actual Capabilities, required []Capability, resolver ContextualCapabilityResolver) bool {
	if resolver == nil {
		return false
	}
	for _, capability := range required {
		if !actual.Has(capability) && !resolver.CanResolveCapability(capability) {
			return false
		}
	}
	return true
}

func validateRoutedRequest(ctx context.Context, connector Connector, session Session, request GenerateRequest) error {
	capabilities := connector.Capabilities()
	if err := request.Validate(session, capabilities); err == nil {
		return nil
	} else {
		variantRequirements := variantRequiredCapabilities(session.Identity.EffectiveVariant())
		if len(variantRequirements) == 0 || requireCapabilities(capabilities, variantRequirements) == nil {
			return err
		}
		resolver, ok := connector.(ContextualCapabilityResolver)
		if !ok || !capabilityRequirementsResolvable(capabilities, variantRequirements, resolver) {
			return err
		}
		resolved, resolveErr := resolver.CapabilitiesForContext(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("contextual capability admission cancelled: %w", ctxErr)
		}
		if resolveErr != nil {
			return fmt.Errorf("contextual capability admission: %w", resolveErr)
		}
		return request.Validate(session, resolved)
	}
}

// sessionRequiredCapabilities adds the session capability that is intrinsic
// to Router.OpenSession without changing the caller's explicit request list.
// It avoids manufacturing a duplicate when callers also declare sessions as
// a required capability.
func sessionRequiredCapabilities(required []Capability) []Capability {
	for _, capability := range required {
		if capability == CapabilitySessions {
			return required
		}
	}
	withSessions := make([]Capability, 0, len(required)+1)
	withSessions = append(withSessions, CapabilitySessions)
	return append(withSessions, required...)
}
