package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type routerFixtureConnector struct {
	kind         Kind
	identity     Identity
	caps         Capabilities
	available    bool
	onOpen       func()
	onComplete   func()
	onStream     func()
	streamEvents <-chan Event
}

func (f routerFixtureConnector) Kind() Kind                 { return f.kind }
func (f routerFixtureConnector) Capabilities() Capabilities { return f.caps }
func (f routerFixtureConnector) OpenSession(_ context.Context, id string) (Session, error) {
	if f.onOpen != nil {
		f.onOpen()
	}
	if !f.available {
		return Session{}, errors.New("fixture unavailable")
	}
	return Session{ID: id, Identity: f.identity, CreatedAt: "2026-09-07T16:00:00Z"}, nil
}
func (f routerFixtureConnector) Complete(_ context.Context, _ Session, _ GenerateRequest) (Completion, Receipt, error) {
	if f.onComplete != nil {
		f.onComplete()
	}
	return Completion{Text: "ok", Model: f.identity.ModelID, FinishReason: "stop"}, Receipt{}, nil
}
func (f routerFixtureConnector) Stream(_ context.Context, _ Session, _ GenerateRequest) (<-chan Event, error) {
	if f.onStream != nil {
		f.onStream()
	}
	return f.streamEvents, nil
}

func identityFor(kind Kind) Identity {
	i := testIdentity()
	i.Engine = kind
	return i
}

func TestRouterRequiresExplicitFallbackAndPreservesSelectedIdentity(t *testing.T) {
	r, err := NewRouter(
		routerFixtureConnector{kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: false},
		routerFixtureConnector{kind: KindSNE, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true, MTP: true}, available: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenSession(context.Background(), "no-fallback", RoutePolicy{Preferred: KindMLX}); err == nil {
		t.Fatal("router silently fell back without explicit permission")
	}
	session, decision, err := r.OpenSession(context.Background(), "fallback", RoutePolicy{Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilityMTP}})
	if err != nil {
		t.Fatal(err)
	}
	if session.Identity.Engine != KindSNE || decision.Selected != KindSNE || !decision.Fallback {
		t.Fatalf("fallback session/decision = %+v/%+v", session, decision)
	}
	if !strings.Contains(decision.Rationale, "explicit fallback") {
		t.Fatalf("fallback rationale = %q", decision.Rationale)
	}
}

func TestRouterRejectsCapabilityGapBeforeConnectorAdmission(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{kind: KindSNE, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.OpenSession(context.Background(), "cap-gap", RoutePolicy{Preferred: KindSNE, RequiredCapabilities: []Capability{CapabilityKVState}})
	if err == nil || !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("capability gap = %v", err)
	}
}

func TestRouterRejectsCancelledSessionAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
		onOpen: cancel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenSession(ctx, "cancelled", RoutePolicy{Preferred: KindMLX}); err == nil {
		t.Fatal("cancelled session admission was reported successful")
	}
}

func TestRouterRejectsConnectorSessionIdentityDrift(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true}, available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenSession(context.Background(), "identity-drift", RoutePolicy{Preferred: KindMLX}); err == nil || !strings.Contains(err.Error(), "returned identity") {
		t.Fatalf("accepted connector identity drift: %v", err)
	}
}

func TestRouterRejectsCancellationAroundCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
		onComplete: cancel,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "completion-cancel", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, CacheNamespace: session.Identity.CacheNamespace}
	if _, _, err := r.Complete(ctx, session, request, decision); err == nil {
		t.Fatal("cancelled completion was reported successful")
	}
}

func TestRouterRejectsCancellationAroundStreamAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
		onStream: cancel,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "stream-cancel", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	if _, err := r.Stream(ctx, session, request, decision); err == nil {
		t.Fatal("cancelled stream admission was reported successful")
	}
}

func TestRouterReturnsErrorForUnconfiguredPreferredConnector(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.OpenSession(context.Background(), "missing-preferred", RoutePolicy{Preferred: KindOMLX})
	if err == nil || !strings.Contains(err.Error(), "omlx") || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured preferred connector error = %v", err)
	}
}

func TestRouterRejectsDecisionSessionEngineMismatch(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{kind: KindSNE, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	_, decision, err := r.OpenSession(context.Background(), "match", RoutePolicy{Preferred: KindSNE})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "match", Identity: identityFor(KindMLX), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, _, err := r.Complete(context.Background(), session, GenerateRequest{}, decision); err == nil {
		t.Fatal("decision/session engine mismatch was accepted")
	}
}

func TestRouterRejectsInvalidStreamReceiptAtRouterBoundary(t *testing.T) {
	session := Session{ID: "stream-receipt", Identity: identityFor(KindMLX), CreatedAt: "2026-09-07T16:00:00Z"}
	stream := make(chan Event, 1)
	stream <- Event{
		Kind:      EventCompleted,
		SessionID: session.ID,
		Sequence:  1,
		Receipt: &Receipt{
			ABIVersion:       ABIVersion,
			SessionID:        session.ID,
			Identity:         session.Identity,
			IdentityDigest:   "not-the-session-digest",
			RequestSHA256:    testSHA,
			CompletionSHA256: testSHA,
			StartedAt:        session.CreatedAt,
			FinishedAt:       "2026-09-07T16:00:01Z",
		},
	}
	close(stream)
	r, err := NewRouter(routerFixtureConnector{
		kind:         KindMLX,
		identity:     session.Identity,
		caps:         Capabilities{Sessions: true, Streaming: true},
		available:    true,
		streamEvents: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, Rationale: "preferred mlx connector admitted"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	events, err := r.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Kind != EventError || event.ErrorCode != "stream_event_invalid" || !strings.Contains(event.Error, "identity digest") {
		t.Fatalf("invalid receipt was not converted to a boundary error: %+v", event)
	}
}

func TestRouterRejectsInvalidRouteDecisionBeforeConnector(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "invalid-route", Identity: identityFor(KindMLX), CreatedAt: "2026-09-07T16:00:00Z"}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, CacheNamespace: session.Identity.CacheNamespace}
	if _, _, err := r.Complete(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("invalid route decision was accepted: %v", err)
	}
}

func TestRouterRejectsNilStreamFromConnector(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{
		kind:      KindMLX,
		identity:  identityFor(KindMLX),
		caps:      Capabilities{Sessions: true, Streaming: true},
		available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "nil-stream", Identity: identityFor(KindMLX), CreatedAt: "2026-09-07T16:00:00Z"}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, Rationale: "preferred mlx connector admitted"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	if _, err := r.Stream(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "nil stream") {
		t.Fatalf("nil connector stream was accepted: %v", err)
	}
}

func TestRouterRejectsStreamClosedBeforeTerminalEvent(t *testing.T) {
	stream := make(chan Event)
	close(stream)
	r, err := NewRouter(routerFixtureConnector{
		kind:         KindMLX,
		identity:     identityFor(KindMLX),
		caps:         Capabilities{Sessions: true, Streaming: true},
		available:    true,
		streamEvents: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "incomplete-stream", Identity: identityFor(KindMLX), CreatedAt: "2026-09-07T16:00:00Z"}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, Rationale: "preferred mlx connector admitted"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	events, err := r.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Kind != EventError || event.ErrorCode != "stream_event_invalid" || !strings.Contains(event.Error, "terminal event") {
		t.Fatalf("incomplete stream was not rejected: %+v", event)
	}
}
