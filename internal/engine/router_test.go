package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

type routerFixtureConnector struct {
	kind                   Kind
	variant                BackendVariant
	identity               Identity
	caps                   Capabilities
	available              bool
	onOpen                 func()
	onComplete             func()
	onStream               func()
	streamEvents           <-chan Event
	completionRequestHash  string
	completionReceiptHash  string
	completionModel        string
	completionModelSet     bool
	completionPromptTokens int
	completionOutputTokens int
	completionTokensSet    bool
}

type contextualRouterFixture struct {
	routerFixtureConnector
	resolvedCaps Capabilities
	resolveErr   error
	resolveCalls int
}

type boundaryRouterFixture struct {
	routerFixtureConnector
	boundary string
}

func (f boundaryRouterFixture) RequestDataBoundary() string { return f.boundary }

func (f *contextualRouterFixture) CanResolveCapability(capability Capability) bool {
	return capability == CapabilityMTP
}

func (f *contextualRouterFixture) CapabilitiesForContext(context.Context) (Capabilities, error) {
	f.resolveCalls++
	return f.resolvedCaps, f.resolveErr
}

func (f routerFixtureConnector) Kind() Kind { return f.kind }
func (f routerFixtureConnector) Variant() BackendVariant {
	if f.variant != "" {
		return f.variant
	}
	return DefaultVariant(f.kind)
}
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
func (f routerFixtureConnector) Complete(_ context.Context, session Session, request GenerateRequest) (Completion, Receipt, error) {
	if f.onComplete != nil {
		f.onComplete()
	}
	requestDigest, err := GenerateRequestDigest(request)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	if f.completionRequestHash != "" {
		requestDigest = f.completionRequestHash
	}
	identityDigest, err := session.Identity.Digest()
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	completionHash := sha256.Sum256([]byte("ok"))
	completionDigest := hex.EncodeToString(completionHash[:])
	if f.completionReceiptHash != "" {
		completionDigest = f.completionReceiptHash
	}
	model := f.identity.ModelID
	if f.completionModelSet {
		model = f.completionModel
	}
	promptTokens, outputTokens := 0, 0
	if f.completionTokensSet {
		promptTokens, outputTokens = f.completionPromptTokens, f.completionOutputTokens
	}
	return Completion{Text: "ok", Model: model, FinishReason: "stop", PromptTokens: promptTokens, OutputTokens: outputTokens}, Receipt{
		ABIVersion: ABIVersion, SessionID: session.ID, Identity: session.Identity,
		IdentityDigest: identityDigest, RequestSHA256: requestDigest,
		CompletionSHA256: completionDigest,
		StartedAt:        "2026-09-07T16:00:00Z", FinishedAt: "2026-09-07T16:00:01Z",
	}, nil
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

func TestRouterBindsAdmittedConnectorBoundaryIntoReceipt(t *testing.T) {
	router, err := NewRouter(boundaryRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindMLX, identity: identityFor(KindMLX),
			caps: Capabilities{Sessions: true, Receipts: true}, available: true,
		},
		boundary: DataBoundaryRemote,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := router.OpenSession(context.Background(), "boundary-receipt", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	if decision.DataBoundary != DataBoundaryRemote {
		t.Fatalf("admitted route boundary = %q, want %q", decision.DataBoundary, DataBoundaryRemote)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "health?", MaxTokens: 8,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	_, receipt, err := router.Complete(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Route == nil || receipt.Route.DataBoundary != DataBoundaryRemote {
		t.Fatalf("route receipt boundary = %+v, want %q", receipt.Route, DataBoundaryRemote)
	}
}

func TestRouterBindsAdmittedConnectorBoundaryIntoStreamReceipt(t *testing.T) {
	started := time.Now().UTC()
	stream := make(chan Event, 1)
	router, err := NewRouter(boundaryRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindMLX, identity: identityFor(KindMLX),
			caps:      Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true},
			available: true, streamEvents: stream,
		},
		boundary: DataBoundaryRemote,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := router.OpenSession(context.Background(), "boundary-stream-receipt", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "health?", MaxTokens: 8,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	receipt, err := streamReceipt(session, request, "stream answer", started, started.Add(time.Millisecond), false)
	if err != nil {
		t.Fatal(err)
	}
	stream <- Event{
		Kind: EventCompleted, SessionID: session.ID, Sequence: 1,
		Model: session.Identity.ModelID, Text: "stream answer", Receipt: &receipt,
	}
	close(stream)
	events, err := router.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := <-events
	if !ok || event.Kind != EventCompleted || event.Receipt == nil || event.Receipt.Route == nil || event.Receipt.Route.DataBoundary != DataBoundaryRemote {
		t.Fatalf("stream terminal route receipt = %+v, open=%v; want remote boundary", event, ok)
	}
}

func TestRouterRejectsCallerForgedConnectorBoundary(t *testing.T) {
	completed := false
	router, err := NewRouter(boundaryRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindMLX, identity: identityFor(KindMLX),
			caps: Capabilities{Sessions: true, Receipts: true}, available: true,
			onComplete: func() { completed = true },
		},
		boundary: DataBoundaryRemote,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := router.OpenSession(context.Background(), "forged-boundary", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	decision.DataBoundary = DataBoundaryOnDevice
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "health?", MaxTokens: 8,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, _, err := router.Complete(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "does not match admitted connector boundary") {
		t.Fatalf("forged route boundary error = %v, want connector-bound rejection", err)
	}
	if completed {
		t.Fatal("connector ran after caller forged its admitted data boundary")
	}

	streamed := false
	streamRouter, err := NewRouter(boundaryRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindMLX, identity: identityFor(KindMLX),
			caps:      Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true},
			available: true, onStream: func() { streamed = true },
		},
		boundary: DataBoundaryRemote,
	})
	if err != nil {
		t.Fatal(err)
	}
	streamSession, streamDecision, err := streamRouter.OpenSession(context.Background(), "forged-stream-boundary", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	streamDecision.DataBoundary = DataBoundaryOnDevice
	streamRequest := GenerateRequest{
		SessionID: streamSession.ID, Identity: streamSession.Identity, Prompt: "health?", MaxTokens: 8,
		Stream: true, CacheNamespace: streamSession.Identity.CacheNamespace,
	}
	if _, err := streamRouter.Stream(context.Background(), streamSession, streamRequest, streamDecision); err == nil || !strings.Contains(err.Error(), "does not match admitted connector boundary") {
		t.Fatalf("forged stream route boundary error = %v, want connector-bound rejection", err)
	}
	if streamed {
		t.Fatal("stream connector ran after caller forged its admitted data boundary")
	}
}

func TestRouterRejectsNilContextAtEveryExecutionBoundary(t *testing.T) {
	router, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps: Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true}, available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := RoutePolicy{Preferred: KindMLX}
	if _, _, err := router.OpenSession(nil, "nil-context", policy); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("OpenSession(nil) error = %v, want explicit context error", err)
	}
	session, decision, err := router.OpenSession(context.Background(), "nil-context", policy)
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, _, err := router.Complete(nil, session, request, decision); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("Complete(nil) error = %v, want explicit context error", err)
	}
	if _, err := router.Stream(nil, session, request, decision); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("Stream(nil) error = %v, want explicit context error", err)
	}
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

func TestRouterCanExplicitlyFallbackWhenPreferredEngineIsUnconfigured(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "unconfigured-fallback", RoutePolicy{Preferred: KindSNE, AllowFallback: true})
	if err != nil {
		t.Fatal(err)
	}
	if session.Identity.Engine != KindMLX || decision.Requested != KindSNE || decision.Selected != KindMLX || !decision.Fallback {
		t.Fatalf("explicit unconfigured-engine fallback = session %+v, decision %+v", session.Identity, decision)
	}
}

func TestRouterDoesNotFallbackWhenNonDefaultPreferredVariantIsUnspecified(t *testing.T) {
	mlxOpened, sneOpened := 0, 0
	mlx := routerFixtureConnector{kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true, onOpen: func() { mlxOpened++ }}
	sneIdentity := identityFor(KindSNE)
	sneIdentity.Variant = VariantSNEMTP
	sneIdentity.Assistant = testAssistantIdentity()
	sne := routerFixtureConnector{kind: KindSNE, variant: VariantSNEMTP, identity: sneIdentity, caps: Capabilities{Sessions: true, MTP: true}, available: true, onOpen: func() { sneOpened++ }}
	r, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenSession(context.Background(), "variant-required", RoutePolicy{Preferred: KindSNE, AllowFallback: true}); err == nil || !strings.Contains(err.Error(), "explicit variant selection is required") {
		t.Fatalf("implicit non-default variant or fallback was accepted: %v", err)
	}
	if mlxOpened != 0 || sneOpened != 0 {
		t.Fatalf("connectors opened before explicit variant selection: MLX=%d SNE=%d", mlxOpened, sneOpened)
	}
	session, decision, err := r.OpenSession(context.Background(), "variant-explicit", RoutePolicy{Preferred: KindSNE, PreferredVariant: VariantSNEMTP})
	if err != nil {
		t.Fatal(err)
	}
	if session.Identity.EffectiveVariant() != VariantSNEMTP || decision.SelectedVariant != VariantSNEMTP || decision.Fallback {
		t.Fatalf("explicit MTP route = identity %+v, decision %+v", session.Identity, decision)
	}
}

func TestRouterRejectsMTPVariantWithoutCapabilityBeforeSessionProbe(t *testing.T) {
	openCalls := 0
	identity := identityFor(KindSNE)
	identity.Variant = VariantSNEMTP
	identity.Assistant = testAssistantIdentity()
	router, err := NewRouter(routerFixtureConnector{
		kind: KindSNE, variant: VariantSNEMTP, identity: identity,
		caps: Capabilities{Sessions: true, Receipts: true}, available: true,
		onOpen: func() { openCalls++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := router.OpenSession(context.Background(), "mtp-without-capability", RoutePolicy{
		Preferred: KindSNE, PreferredVariant: VariantSNEMTP,
	}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "mtp") {
		t.Fatalf("MTP route without capability error = %v", err)
	}
	if openCalls != 0 {
		t.Fatalf("MTP capability gap probed the connector %d times", openCalls)
	}
}

func TestRouterResolvesMTPOnlyFromContextualReadiness(t *testing.T) {
	openCalls := 0
	identity := identityFor(KindSNE)
	identity.Variant = VariantSNEMTP
	identity.Assistant = testAssistantIdentity()
	connector := &contextualRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindSNE, variant: VariantSNEMTP, identity: identity,
			caps: Capabilities{Sessions: true, Receipts: true}, available: true,
			onOpen: func() { openCalls++ },
		},
		resolvedCaps: Capabilities{Sessions: true, Receipts: true, MTP: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	policy := RoutePolicy{Preferred: KindSNE, PreferredVariant: VariantSNEMTP}
	selection, err := NewSelectionController(router, policy)
	if err != nil {
		t.Fatalf("configured but not yet probed MTP route could not be selected: %v", err)
	}
	if connector.resolveCalls != 0 || selection.Snapshot().Connectors[0].Capabilities.MTP {
		t.Fatal("selection snapshot claimed live MTP readiness before session admission")
	}
	session, decision, err := router.OpenSession(context.Background(), "mtp-readiness", RoutePolicy{
		Preferred: KindSNE, PreferredVariant: VariantSNEMTP,
	})
	if err != nil {
		t.Fatal(err)
	}
	if connector.resolveCalls != 1 || openCalls != 1 || session.Identity.Assistant == nil || decision.SelectedVariant != VariantSNEMTP {
		t.Fatalf("contextual MTP admission: resolve=%d open=%d session=%+v decision=%+v", connector.resolveCalls, openCalls, session, decision)
	}

	connector.resolveErr = errors.New("readiness does not advertise mtp")
	connector.resolveCalls = 0
	openCalls = 0
	if _, _, err := router.OpenSession(context.Background(), "mtp-not-ready", RoutePolicy{
		Preferred: KindSNE, PreferredVariant: VariantSNEMTP,
	}); err == nil || !strings.Contains(err.Error(), "does not advertise mtp") {
		t.Fatalf("unready contextual MTP route error = %v", err)
	}
	if connector.resolveCalls != 1 || openCalls != 0 {
		t.Fatalf("unready MTP reached session open: resolve=%d open=%d", connector.resolveCalls, openCalls)
	}
}

func TestRouterCompletionRevalidatesContextualMTPCapability(t *testing.T) {
	identity := identityFor(KindSNE)
	identity.Variant = VariantSNEMTP
	identity.Assistant = testAssistantIdentity()
	connector := &contextualRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindSNE, variant: VariantSNEMTP, identity: identity,
			caps: Capabilities{Sessions: true, Receipts: true}, available: true,
		},
		resolvedCaps: Capabilities{Sessions: true, Receipts: true, MTP: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	policy := RoutePolicy{Preferred: KindSNE, PreferredVariant: VariantSNEMTP}
	session, decision, err := router.OpenSession(context.Background(), "mtp-completion", policy)
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity,
		Prompt: "hello", MaxTokens: 8, CacheNamespace: session.Identity.CacheNamespace,
	}
	completion, receipt, err := router.Complete(context.Background(), session, request, decision)
	if err != nil {
		t.Fatalf("contextually admitted MTP completion failed: %v", err)
	}
	if completion.Model != identity.ModelID || receipt.Identity.Variant != VariantSNEMTP {
		t.Fatalf("MTP completion lost its admitted identity: completion=%+v receipt=%+v", completion, receipt)
	}
	if connector.resolveCalls != 2 {
		t.Fatalf("contextual readiness was resolved %d times, want session and completion checks", connector.resolveCalls)
	}
}

func TestRouterStreamChecksContextualMTPBeforeReportingUnsupportedStreaming(t *testing.T) {
	identity := identityFor(KindSNE)
	identity.Variant = VariantSNEMTP
	identity.Assistant = testAssistantIdentity()
	streamCalls := 0
	connector := &contextualRouterFixture{
		routerFixtureConnector: routerFixtureConnector{
			kind: KindSNE, variant: VariantSNEMTP, identity: identity,
			caps: Capabilities{Sessions: true, Receipts: true}, available: true,
			onStream: func() { streamCalls++ },
		},
		resolvedCaps: Capabilities{Sessions: true, Receipts: true, MTP: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	policy := RoutePolicy{Preferred: KindSNE, PreferredVariant: VariantSNEMTP}
	session, decision, err := router.OpenSession(context.Background(), "mtp-stream", policy)
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity,
		Prompt: "hello", MaxTokens: 8, Stream: true,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, err := router.Stream(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "streaming is unsupported") {
		t.Fatalf("streaming limitation was not reported after contextual MTP validation: %v", err)
	}
	if connector.resolveCalls != 2 || streamCalls != 0 {
		t.Fatalf("contextual checks=%d stream calls=%d, want two checks and no transport", connector.resolveCalls, streamCalls)
	}
}

func TestRouterNeverFallsBackFromExplicitMTP(t *testing.T) {
	opened := 0
	identity := identityFor(KindSNE)
	identity.Variant = VariantSNEMTP
	identity.Assistant = testAssistantIdentity()
	router, err := NewRouter(routerFixtureConnector{
		kind: KindSNE, variant: VariantSNEMTP, identity: identity,
		caps: Capabilities{Sessions: true, Receipts: true, MTP: true}, available: false,
		onOpen: func() { opened++ },
	}, routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
		onOpen: func() { opened++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := router.OpenSession(context.Background(), "mtp-no-downgrade", RoutePolicy{
		Preferred: KindSNE, PreferredVariant: VariantSNEMTP, AllowFallback: true,
	}); err == nil || !strings.Contains(err.Error(), "cannot fall back") {
		t.Fatalf("explicit MTP fallback policy error = %v", err)
	}
	if opened != 0 {
		t.Fatalf("explicit MTP fallback policy opened %d connectors", opened)
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

func TestRouterRequiresSessionCapabilityBeforeOpeningConnector(t *testing.T) {
	opened := 0
	connector := routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{}, available: true,
		onOpen: func() { opened++ },
	}
	r, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.OpenSession(context.Background(), "missing-session-capability", RoutePolicy{Preferred: KindMLX})
	if err == nil || !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("session capability error = %v, want ErrUnsupportedCapability", err)
	}
	if opened != 0 {
		t.Fatalf("connector without session capability was called %d times", opened)
	}
}

func TestRouterEnforcesRequestCapabilitiesBeforeCallingConnectors(t *testing.T) {
	completeCalls, streamCalls := 0, 0
	connector := routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
		onComplete: func() { completeCalls++ },
		onStream:   func() { streamCalls++ },
	}
	r, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "request-capability-fence", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 8,
		CacheNamespace: session.Identity.CacheNamespace, Tools: []ToolSpec{{Name: "lookup"}},
	}
	if _, _, err := r.Complete(context.Background(), session, request, decision); err == nil || !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("unsupported tool request error = %v, want ErrUnsupportedCapability", err)
	}
	request.Tools = nil
	request.Stream = true
	if _, err := r.Stream(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "streaming is unsupported") {
		t.Fatalf("unsupported stream request error = %v", err)
	}
	if completeCalls != 0 || streamCalls != 0 {
		t.Fatalf("unsupported requests reached connector: complete=%d stream=%d", completeCalls, streamCalls)
	}
}

func TestRouterRejectsStreamMarkedCompletionBeforeCallingConnector(t *testing.T) {
	completeCalls := 0
	connector := routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps:      Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true},
		available: true, onComplete: func() { completeCalls++ },
	}
	r, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "stream-to-complete", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 2,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, _, err := r.Complete(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "stream=false") {
		t.Fatalf("router buffered a stream-marked request: %v", err)
	}
	if completeCalls != 0 {
		t.Fatalf("stream-marked request reached connector %d times", completeCalls)
	}
}

func TestRouterRejectsUnmarkedStreamBeforeCallingConnector(t *testing.T) {
	streamCalls := 0
	connector := routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps:      Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true},
		available: true, onStream: func() { streamCalls++ },
	}
	r, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "unmarked-stream", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 2,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, err := r.Stream(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "stream=true") {
		t.Fatalf("router accepted an unmarked stream request: %v", err)
	}
	if streamCalls != 0 {
		t.Fatalf("unmarked stream reached connector %d times", streamCalls)
	}
}

func TestRouterRejectsCompletionReceiptForDifferentRequest(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps: Capabilities{Sessions: true, Receipts: true}, available: true,
		completionRequestHash: testSHA,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "receipt-request-mismatch", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "the admitted request", MaxTokens: 8, CacheNamespace: session.Identity.CacheNamespace}
	if _, _, err := r.Complete(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "does not match admitted request") {
		t.Fatalf("completion receipt for a different request was accepted: %v", err)
	}
}

func TestRouterRejectsCompletionReceiptForDifferentText(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps: Capabilities{Sessions: true, Receipts: true}, available: true,
		completionReceiptHash: testSHA,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "completion-text-mismatch", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, CacheNamespace: session.Identity.CacheNamespace}
	if _, _, err := r.Complete(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "does not match returned completion") {
		t.Fatalf("completion with a receipt for different text was accepted: %v", err)
	}
}

func TestRouterRejectsNegativeCompletionTokenCounts(t *testing.T) {
	for _, counts := range []struct {
		name   string
		prompt int
		output int
	}{
		{name: "prompt", prompt: -1, output: 1},
		{name: "output", prompt: 1, output: -1},
	} {
		t.Run(counts.name, func(t *testing.T) {
			r, err := NewRouter(routerFixtureConnector{
				kind: KindMLX, identity: identityFor(KindMLX),
				caps: Capabilities{Sessions: true, Receipts: true}, available: true,
				completionPromptTokens: counts.prompt, completionOutputTokens: counts.output, completionTokensSet: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			session, decision, err := r.OpenSession(context.Background(), "negative-token-counts", RoutePolicy{Preferred: KindMLX})
			if err != nil {
				t.Fatal(err)
			}
			request := GenerateRequest{
				SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
				CacheNamespace: session.Identity.CacheNamespace,
			}
			if completion, _, err := r.Complete(context.Background(), session, request, decision); err == nil || !strings.Contains(err.Error(), "token counts must be non-negative") {
				t.Fatalf("completion with invalid token counts accepted: %+v error=%v", completion, err)
			}
		})
	}
}

func TestRouterRejectsCancellationDuringCompletionValidation(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnContextErrCall{Context: base, cancel: cancel}
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps: Capabilities{Sessions: true, Receipts: true}, available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "cancel-during-completion-validation", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	if completion, _, err := r.Complete(ctx, session, request, decision); err == nil || !strings.Contains(err.Error(), "cancelled before accepting verified result") {
		t.Fatalf("completion was accepted after cancellation: completion=%+v error=%v", completion, err)
	}
}

func TestRouterRejectsMissingOrDriftedCompletionModelIdentity(t *testing.T) {
	for _, model := range []struct {
		name  string
		value string
	}{
		{name: "missing"},
		{name: "different", value: "unexpected-model"},
	} {
		t.Run(model.name, func(t *testing.T) {
			identity := identityFor(KindMLX)
			router, err := NewRouter(routerFixtureConnector{
				kind: KindMLX, identity: identity, caps: Capabilities{Sessions: true, Receipts: true},
				available: true, completionModel: model.value, completionModelSet: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			session, decision, err := router.OpenSession(context.Background(), "completion-model-"+model.name, RoutePolicy{Preferred: KindMLX})
			if err != nil {
				t.Fatal(err)
			}
			request := GenerateRequest{
				SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
				CacheNamespace: session.Identity.CacheNamespace,
			}
			completion, receipt, err := router.Complete(context.Background(), session, request, decision)
			if err == nil || !strings.Contains(err.Error(), "served model") {
				t.Fatalf("completion identity = %+v receipt=%+v err=%v, want fail-closed model error", completion, receipt, err)
			}
			if completion.Text != "" || receipt.RequestSHA256 != "" {
				t.Fatalf("invalid model completion escaped router: completion=%+v receipt=%+v", completion, receipt)
			}
		})
	}
}

func TestRouterRejectsVariantMismatchBeforeConnectorAdmission(t *testing.T) {
	opened := 0
	connector := routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
		onOpen: func() { opened++ },
	}
	r, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.OpenSession(context.Background(), "variant-mismatch", RoutePolicy{Preferred: KindMLX, PreferredVariant: VariantMLXPatched})
	if err == nil || !strings.Contains(err.Error(), "variant") {
		t.Fatalf("variant mismatch was accepted: %v", err)
	}
	if opened != 0 {
		t.Fatalf("connector was opened before variant admission failed: %d", opened)
	}
}

func TestRouterRejectsReturnedVariantDrift(t *testing.T) {
	identity := identityFor(KindMLX)
	identity.Variant = VariantMLXPatched
	r, err := NewRouter(routerFixtureConnector{kind: KindMLX, identity: identity, caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.OpenSession(context.Background(), "returned-variant-drift", RoutePolicy{Preferred: KindMLX})
	if err == nil || !strings.Contains(err.Error(), "returned variant") {
		t.Fatalf("returned variant drift was accepted: %v", err)
	}
}

func TestRouterDecisionCarriesSelectedVariant(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	_, decision, err := r.OpenSession(context.Background(), "variant-route", RoutePolicy{Preferred: KindMLX, PreferredVariant: VariantMLXRaw})
	if err != nil {
		t.Fatal(err)
	}
	if decision.RequestedVariant != VariantMLXRaw || decision.SelectedVariant != VariantMLXRaw {
		t.Fatalf("variant route decision = %+v", decision)
	}
}

func TestRouterSupportsCoConfiguredVariantsAndRoutesExactSelection(t *testing.T) {
	rawIdentity := identityFor(KindMLX)
	rawIdentity.Variant = VariantMLXRaw
	patchedIdentity := identityFor(KindMLX)
	patchedIdentity.Variant = VariantMLXPatched
	rawOpened, patchedOpened := 0, 0
	r, err := NewRouter(
		routerFixtureConnector{kind: KindMLX, variant: VariantMLXRaw, identity: rawIdentity, caps: Capabilities{Sessions: true}, available: true, onOpen: func() { rawOpened++ }},
		routerFixtureConnector{kind: KindMLX, variant: VariantMLXPatched, identity: patchedIdentity, caps: Capabilities{Sessions: true}, available: true, onOpen: func() { patchedOpened++ }},
	)
	if err != nil {
		t.Fatalf("co-configured variants rejected: %v", err)
	}
	session, decision, err := r.OpenSession(context.Background(), "patched-route", RoutePolicy{Preferred: KindMLX, PreferredVariant: VariantMLXPatched})
	if err != nil {
		t.Fatal(err)
	}
	if session.Identity.EffectiveVariant() != VariantMLXPatched || decision.SelectedVariant != VariantMLXPatched || rawOpened != 0 || patchedOpened != 1 {
		t.Fatalf("wrong variant admitted: identity=%+v decision=%+v opened raw/patched=%d/%d", session.Identity, decision, rawOpened, patchedOpened)
	}
	if _, err := NewRouter(
		routerFixtureConnector{kind: KindMLX, variant: VariantMLXRaw, identity: rawIdentity, caps: Capabilities{Sessions: true}, available: true},
		routerFixtureConnector{kind: KindMLX, variant: VariantMLXRaw, identity: rawIdentity, caps: Capabilities{Sessions: true}, available: true},
	); err == nil {
		t.Fatal("duplicate kind+variant connector was accepted")
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

type cancelOnContextErrCall struct {
	context.Context
	cancel context.CancelFunc
	call   int
}

func (c *cancelOnContextErrCall) Err() error {
	c.call++
	if c.call == 3 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestRouterRejectsCancellationDuringSessionValidation(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnContextErrCall{Context: base, cancel: cancel}
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session, _, err := r.OpenSession(ctx, "cancel-during-validation", RoutePolicy{Preferred: KindMLX}); err == nil || !strings.Contains(err.Error(), "cancelled before returning mlx connector") {
		t.Fatalf("session admission after cancellation returned session %+v, error %v", session, err)
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
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true, Receipts: true}, available: true,
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
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true}, available: true,
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

func TestRouterTerminatesForwarderWhenConnectorLeavesStreamOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true}, available: true,
		streamEvents: make(chan Event),
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), "cancel-open-stream", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	events, err := r.Stream(ctx, session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case event, ok := <-events:
		if !ok || event.Kind != EventError || event.ErrorCode != "cancelled" || event.Receipt == nil || !event.Receipt.Cancelled {
			t.Fatalf("cancellation event = %+v, open=%v", event, ok)
		}
		if err := event.Receipt.Validate(session); err != nil {
			t.Fatalf("router cancellation receipt does not validate: %v", err)
		}
		if event.Receipt.Route == nil || event.Receipt.Route.SelectedVariant != decision.SelectedVariant {
			t.Fatalf("router cancellation receipt lost route provenance: %+v", event.Receipt.Route)
		}
	case <-time.After(time.Second):
		t.Fatal("router did not terminate the stream after cancellation")
	}
	if _, ok := <-events; ok {
		t.Fatal("router stream remained open after its cancellation terminal")
	}
}

func TestRouterCancellationReceiptBindsOnlyForwardedPartialText(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const sessionID = "cancel-forwarded-partial"
	source := make(chan Event, 1)
	source <- Event{Kind: EventDelta, SessionID: sessionID, Sequence: 1, Text: "partial"}
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available: true, streamEvents: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), sessionID, RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	events, err := r.Stream(ctx, session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event, ok := <-events:
		if !ok || event.Kind != EventDelta || event.Text != "partial" {
			t.Fatalf("first routed event = %+v, open=%v", event, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("router did not forward the provider delta")
	}
	cancel()
	select {
	case event, ok := <-events:
		if !ok || event.Kind != EventError || event.ErrorCode != "cancelled" || event.Receipt == nil || !event.Receipt.Cancelled {
			t.Fatalf("cancellation terminal = %+v, open=%v", event, ok)
		}
		if err := event.Receipt.Validate(session); err != nil {
			t.Fatalf("cancellation receipt is invalid: %v", err)
		}
		want := sha256.Sum256([]byte("partial"))
		if event.Receipt.CompletionSHA256 != hex.EncodeToString(want[:]) {
			t.Fatalf("cancellation receipt includes unforwarded content: hash=%q", event.Receipt.CompletionSHA256)
		}
		if event.Receipt.Route == nil || event.Receipt.Route.SelectedVariant != decision.SelectedVariant {
			t.Fatalf("cancellation receipt lost route provenance: %+v", event.Receipt.Route)
		}
	case <-time.After(time.Second):
		t.Fatal("router did not emit a receipt-bearing cancellation terminal")
	}
}

func TestRouterCancellationReceiptExcludesDeltaEvictedFromBufferedOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const sessionID = "cancel-buffered-delta"
	source := make(chan Event, 1)
	source <- Event{Kind: EventDelta, SessionID: sessionID, Sequence: 1, Text: "queued"}
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available: true, streamEvents: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), sessionID, RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	events, err := r.Stream(ctx, session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for len(events) == 0 {
		select {
		case <-deadline:
			t.Fatal("router did not buffer its initial delta")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	var received []Event
	var delivered strings.Builder
	for event := range events {
		received = append(received, event)
		if event.Kind == EventDelta {
			delivered.WriteString(event.Text)
		}
	}
	if len(received) != 1 {
		t.Fatalf("events after cancellation with a full output buffer = %+v, want only a terminal event", received)
	}
	terminal := received[0]
	if terminal.Kind != EventError || terminal.ErrorCode != "cancelled" || terminal.Receipt == nil || !terminal.Receipt.Cancelled {
		t.Fatalf("buffered cancellation terminal = %+v, want receipt-bearing cancellation", terminal)
	}
	if err := terminal.Receipt.Validate(session); err != nil {
		t.Fatalf("buffered cancellation receipt is invalid: %v", err)
	}
	if terminal.Receipt.CompletionSHA256 != completionDigest(delivered.String()) {
		t.Fatalf("cancellation receipt hashes dropped delta: delivered=%q receipt=%q", delivered.String(), terminal.Receipt.CompletionSHA256)
	}
	if terminal.Receipt.Route == nil || terminal.Receipt.Route.SelectedVariant != decision.SelectedVariant {
		t.Fatalf("buffered cancellation receipt lost route provenance: %+v", terminal.Receipt.Route)
	}
}

func TestRouterRebasesConnectorCancellationReceiptWhenDroppingBufferedDelta(t *testing.T) {
	session := Session{ID: "connector-cancel-buffered-delta", Identity: identityFor(KindMLX), CreatedAt: "2026-09-23T12:00:00Z"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, SelectedVariant: VariantMLXRaw, DataBoundary: DataBoundaryOnDevice, Rationale: "preferred mlx connector admitted"}
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "queued", started, started.Add(time.Millisecond), true)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Route = &decision
	terminal := Event{Kind: EventError, SessionID: session.ID, Sequence: 2, ErrorCode: "cancelled", Error: "stream cancelled", Receipt: &receipt}
	output := make(chan Event, 1)
	output <- Event{Kind: EventDelta, SessionID: session.ID, Sequence: 1, Text: "queued"}
	routerPublishTerminal(output, terminal, func(pending Event) Event {
		return routerRebaseCancellationForDroppedDelta(terminal, pending, session, request, decision, "queued", started)
	})
	got := <-output
	if err := got.Validate(1); err != nil {
		t.Fatalf("rebased connector cancellation event is invalid: %v", err)
	}
	if got.Receipt == nil || !got.Receipt.Cancelled {
		t.Fatalf("rebased connector cancellation has no cancellation receipt: %+v", got)
	}
	if err := got.Receipt.Validate(session); err != nil {
		t.Fatalf("rebased connector cancellation receipt is invalid: %v", err)
	}
	if got.Receipt.CompletionSHA256 != completionDigest("") {
		t.Fatalf("connector cancellation receipt hashes dropped delta: %q", got.Receipt.CompletionSHA256)
	}
	if got.Receipt.Route == nil || got.Receipt.Route.SelectedVariant != decision.SelectedVariant {
		t.Fatalf("rebased connector cancellation lost route provenance: %+v", got.Receipt.Route)
	}
}

func TestRouterInvalidatesCancellationReceiptWhenBufferedDeltaCannotBeReconciled(t *testing.T) {
	session := Session{ID: "connector-cancel-unreconciled-delta", Identity: identityFor(KindMLX), CreatedAt: "2026-09-23T12:00:00Z"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, SelectedVariant: VariantMLXRaw, DataBoundary: DataBoundaryOnDevice, Rationale: "preferred mlx connector admitted"}
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "partial", started, started.Add(time.Millisecond), true)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Route = &decision
	terminal := Event{Kind: EventError, SessionID: session.ID, Sequence: 2, ErrorCode: "cancelled", Error: "stream cancelled", Receipt: &receipt}
	got := routerRebaseCancellationForDroppedDelta(
		terminal,
		Event{Kind: EventDelta, SessionID: session.ID, Sequence: 1, Text: "not-the-suffix"},
		session,
		request,
		decision,
		"partial",
		started,
	)
	if got.ErrorCode != "cancellation_receipt_invalid" || got.Receipt != nil {
		t.Fatalf("unreconcilable cancellation retained authoritative receipt: %+v", got)
	}
	if err := got.Validate(1); err != nil {
		t.Fatalf("fail-closed cancellation error is invalid: %v", err)
	}
}

func TestRouterReplacesConnectorCancellationReceiptForUnforwardedText(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := Session{ID: "cancel-receipt", Identity: identityFor(KindMLX), CreatedAt: "2026-09-07T16:00:00Z"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "partial", started, started.Add(time.Millisecond), true)
	if err != nil {
		t.Fatal(err)
	}
	source := make(chan Event)
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: session.Identity, caps: Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true}, available: true,
		streamEvents: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, SelectedVariant: VariantMLXRaw, Rationale: "preferred mlx connector admitted"}
	events, err := r.Stream(ctx, session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	source <- Event{Kind: EventError, SessionID: session.ID, Sequence: 1, ErrorCode: "cancelled", Error: "stream cancelled", Receipt: &receipt}
	close(source)
	select {
	case event, ok := <-events:
		if !ok || event.Kind != EventError || event.ErrorCode != "cancelled" || event.Receipt == nil || !event.Receipt.Cancelled {
			t.Fatalf("cancellation event lost receipt: %+v, open=%v", event, ok)
		}
		if event.Receipt.Route == nil || event.Receipt.Route.SelectedVariant != VariantMLXRaw {
			t.Fatalf("cancellation receipt lost selected route: %+v", event.Receipt.Route)
		}
		want := sha256.Sum256(nil)
		if event.Receipt.CompletionSHA256 != hex.EncodeToString(want[:]) {
			t.Fatalf("cancellation receipt attests to text that was never forwarded: hash=%q", event.Receipt.CompletionSHA256)
		}
	case <-time.After(time.Second):
		t.Fatal("router did not deliver cancellation receipt")
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
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	requestDigest, err := GenerateRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	stream := make(chan Event, 1)
	stream <- Event{
		Kind:      EventCompleted,
		SessionID: session.ID,
		Sequence:  1,
		Model:     session.Identity.ModelID,
		Receipt: &Receipt{
			ABIVersion:       ABIVersion,
			SessionID:        session.ID,
			Identity:         session.Identity,
			IdentityDigest:   "not-the-session-digest",
			RequestSHA256:    requestDigest,
			CompletionSHA256: completionDigest(""),
			StartedAt:        session.CreatedAt,
			FinishedAt:       "2026-09-07T16:00:01Z",
		},
	}
	close(stream)
	r, err := NewRouter(routerFixtureConnector{
		kind:         KindMLX,
		identity:     session.Identity,
		caps:         Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available:    true,
		streamEvents: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, Rationale: "preferred mlx connector admitted"}
	events, err := r.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Kind != EventError || event.ErrorCode != "stream_event_invalid" || !strings.Contains(event.Error, "identity digest") {
		t.Fatalf("invalid receipt was not converted to a boundary error: %+v", event)
	}
}

func TestRouterBindsErrorStreamReceiptToForwardedPartialText(t *testing.T) {
	started := time.Now().UTC()
	for _, tc := range []struct {
		name        string
		forgeDigest bool
		wantReject  bool
	}{
		{name: "matching partial text receipt"},
		{name: "forged partial text receipt", forgeDigest: true, wantReject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := make(chan Event, 2)
			r, err := NewRouter(routerFixtureConnector{
				kind: KindMLX, identity: identityFor(KindMLX),
				caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
				available: true, streamEvents: source,
			})
			if err != nil {
				t.Fatal(err)
			}
			session, decision, err := r.OpenSession(context.Background(), "error-stream-receipt", RoutePolicy{Preferred: KindMLX})
			if err != nil {
				t.Fatal(err)
			}
			request := GenerateRequest{
				SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4,
				Stream: true, CacheNamespace: session.Identity.CacheNamespace,
			}
			receipt, err := streamReceipt(session, request, "partial output", started, started.Add(time.Millisecond), false)
			if err != nil {
				t.Fatal(err)
			}
			if tc.forgeDigest {
				receipt.CompletionSHA256 = completionDigest("different output")
			}
			source <- Event{Kind: EventDelta, SessionID: session.ID, Sequence: 1, Text: "partial output"}
			source <- Event{Kind: EventError, SessionID: session.ID, Sequence: 2, ErrorCode: "provider_error", Error: "provider stopped", Receipt: &receipt}
			close(source)

			events, err := r.Stream(context.Background(), session, request, decision)
			if err != nil {
				t.Fatal(err)
			}
			first, ok := <-events
			if !ok || first.Kind != EventDelta || first.Text != "partial output" {
				t.Fatalf("first stream event = %+v, open=%v; want forwarded partial delta", first, ok)
			}
			terminal, ok := <-events
			if !ok || terminal.Kind != EventError {
				t.Fatalf("terminal stream event = %+v, open=%v; want error", terminal, ok)
			}
			if tc.wantReject {
				if terminal.ErrorCode != "stream_event_invalid" || !strings.Contains(terminal.Error, "error stream receipt text hash") {
					t.Fatalf("forged error receipt outcome = %+v; want fail-closed receipt error", terminal)
				}
				return
			}
			if terminal.ErrorCode != "provider_error" || terminal.Receipt == nil || terminal.Receipt.Route == nil {
				t.Fatalf("valid error receipt outcome = %+v; want provider error with route-bound receipt", terminal)
			}
			if terminal.Receipt.CompletionSHA256 != completionDigest("partial output") {
				t.Fatalf("error receipt digest = %q; want digest of forwarded partial output", terminal.Receipt.CompletionSHA256)
			}
		})
	}
}

func TestRouterRejectsEventsFromAnotherSession(t *testing.T) {
	session := Session{ID: "admitted-session", Identity: identityFor(KindMLX), CreatedAt: "2026-09-07T16:00:00Z"}
	stream := make(chan Event, 1)
	stream <- Event{Kind: EventDelta, SessionID: "other-session", Sequence: 1, Text: "must not cross sessions"}
	close(stream)
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: session.Identity, caps: Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true}, available: true,
		streamEvents: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision := RouteDecision{Requested: KindMLX, Selected: KindMLX, SelectedVariant: VariantMLXRaw, Rationale: "preferred mlx connector admitted"}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	events, err := r.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := <-events
	if !ok || event.Kind != EventError || event.ErrorCode != "stream_event_invalid" || event.SessionID != session.ID || !strings.Contains(event.Error, "does not match admitted session") {
		t.Fatalf("cross-session event was not rejected at the router boundary: %+v, open=%v", event, ok)
	}
}

func TestRouterRejectsStreamReceiptForDifferentRequest(t *testing.T) {
	const sessionID = "stream-receipt-request-mismatch"
	source := make(chan Event, 1)
	r, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identityFor(KindMLX),
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available: true, streamEvents: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := r.OpenSession(context.Background(), sessionID, RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "admitted prompt", MaxTokens: 8, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
	otherRequest := request
	otherRequest.Prompt = "different prompt"
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, otherRequest, "answer", started, started.Add(time.Millisecond), false)
	if err != nil {
		t.Fatal(err)
	}
	source <- Event{Kind: EventCompleted, SessionID: session.ID, Sequence: 1, Model: session.Identity.ModelID, Text: "answer", Receipt: &receipt}
	events, err := r.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := <-events
	if !ok || event.Kind != EventError || event.ErrorCode != "stream_event_invalid" || !strings.Contains(event.Error, "does not match admitted request") {
		t.Fatalf("stream receipt for a different request was accepted: %+v, open=%v", event, ok)
	}
}

func TestRouterRejectsCompletedStreamWithModelDrift(t *testing.T) {
	identity := identityFor(KindMLX)
	stream := make(chan Event, 1)
	connector := routerFixtureConnector{
		kind: KindMLX, identity: identity,
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available: true, streamEvents: stream,
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := router.OpenSession(context.Background(), "stream-model-drift", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "answer", started, started.Add(time.Millisecond), false)
	if err != nil {
		t.Fatal(err)
	}
	stream <- Event{Kind: EventCompleted, SessionID: session.ID, Sequence: 1, Model: "unexpected-model", Text: "answer", Receipt: &receipt}
	close(stream)
	events, err := router.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := <-events
	if !ok || event.Kind != EventError || !strings.Contains(event.Error, "does not match admitted model") {
		t.Fatalf("completed stream with model drift was accepted: %+v, open=%v", event, ok)
	}
}

func TestRouterRejectsCompletedStreamReceiptForDifferentText(t *testing.T) {
	identity := identityFor(KindMLX)
	stream := make(chan Event, 1)
	router, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identity,
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available: true, streamEvents: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := router.OpenSession(context.Background(), "stream-text-mismatch", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "different text", started, started.Add(time.Millisecond), false)
	if err != nil {
		t.Fatal(err)
	}
	stream <- Event{Kind: EventCompleted, SessionID: session.ID, Sequence: 1, Model: session.Identity.ModelID, Text: "answer", Receipt: &receipt}
	close(stream)
	events, err := router.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := <-events
	if !ok || event.Kind != EventError || event.ErrorCode != "stream_event_invalid" || !strings.Contains(event.Error, "does not match completed text") {
		t.Fatalf("completed stream with a receipt for different text was accepted: %+v, open=%v", event, ok)
	}
}

func TestRouterRejectsCompletedStreamTextThatDiffersFromForwardedDeltas(t *testing.T) {
	identity := identityFor(KindMLX)
	stream := make(chan Event, 2)
	router, err := NewRouter(routerFixtureConnector{
		kind: KindMLX, identity: identity,
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
		available: true, streamEvents: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, decision, err := router.OpenSession(context.Background(), "stream-terminal-text-mismatch", RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "answer", started, started.Add(time.Millisecond), false)
	if err != nil {
		t.Fatal(err)
	}
	stream <- Event{Kind: EventDelta, SessionID: session.ID, Sequence: 1, Text: "different "}
	stream <- Event{Kind: EventCompleted, SessionID: session.ID, Sequence: 2, Model: session.Identity.ModelID, Text: "answer", Receipt: &receipt}
	close(stream)
	events, err := router.Stream(context.Background(), session, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	first, ok := <-events
	if !ok || first.Kind != EventDelta || first.Text != "different " {
		t.Fatalf("first event = %+v, open=%v; want original delta", first, ok)
	}
	terminal, ok := <-events
	if !ok || terminal.Kind != EventError || terminal.ErrorCode != "stream_event_invalid" || !strings.Contains(terminal.Error, "does not match forwarded deltas") {
		t.Fatalf("terminal event = %+v, open=%v; want mismatch error", terminal, ok)
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
		caps:      Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
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
		caps:         Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
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
