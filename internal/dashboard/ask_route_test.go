package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type failingRequestBody struct{}

func (failingRequestBody) Read([]byte) (int, error) { return 0, errors.New("injected read failure") }
func (failingRequestBody) Close() error             { return nil }

type askPromptExecutor struct {
	completion engine.Completion
	receipt    engine.Receipt
	request    *engine.PromptRequest
}

type askRouteProvider struct {
	model    string
	requests []provider.Request
}

func (p *askRouteProvider) Name() string                   { return p.model }
func (p *askRouteProvider) Tier() provider.Tier            { return provider.TierLocal }
func (p *askRouteProvider) Caps() provider.Caps            { return provider.Caps{} }
func (p *askRouteProvider) Available(context.Context) bool { return true }
func (p *askRouteProvider) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, request)
	return provider.Response{Model: p.model, Text: `{"findings":[0],"summary":"The selected route responded."}`, FinishReason: "stop"}, nil
}

type askPolicySwitchController struct {
	*engine.SelectionController
	switchTo engine.RoutePolicy
	switched bool
}

func (c *askPolicySwitchController) Policy() engine.RoutePolicy {
	accepted := c.SelectionController.Policy()
	if !c.switched {
		c.switched = true
		if _, err := c.SelectionController.Select(c.switchTo); err != nil {
			panic(err)
		}
	}
	return accepted
}

func (f askPromptExecutor) CompletePrompt(_ context.Context, request engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	if f.request != nil {
		*f.request = request
	}
	return f.completion, f.receipt, nil
}

type askControllerWithoutReceipt struct{}

func (askControllerWithoutReceipt) Snapshot() engine.SelectionSnapshot {
	return engine.SelectionSnapshot{}
}
func (askControllerWithoutReceipt) Select(engine.RoutePolicy) (engine.SelectionSnapshot, error) {
	return engine.SelectionSnapshot{}, nil
}
func (askControllerWithoutReceipt) CompletePrompt(context.Context, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	return engine.Completion{Model: "local", Text: `{"findings":[],"summary":"local answer"}`}, engine.Receipt{}, nil
}

type askInvocationTracker struct {
	askControllerWithoutReceipt
	called bool
}

func (c *askInvocationTracker) CompletePrompt(context.Context, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	c.called = true
	return engine.Completion{}, engine.Receipt{}, errors.New("prompt should not be invoked")
}

type askCancellableController struct {
	started chan struct{}
	once    sync.Once
}

func (*askCancellableController) Snapshot() engine.SelectionSnapshot {
	return engine.SelectionSnapshot{}
}
func (*askCancellableController) Select(engine.RoutePolicy) (engine.SelectionSnapshot, error) {
	return engine.SelectionSnapshot{}, nil
}
func (c *askCancellableController) CompletePrompt(ctx context.Context, _ engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	c.once.Do(func() { close(c.started) })
	<-ctx.Done()
	return engine.Completion{}, engine.Receipt{}, ctx.Err()
}

func TestAPIAskFailsClosedWithoutCanonicalPromptController(t *testing.T) {
	server := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"question":"what should I address?"}`))
	resp := httptest.NewRecorder()
	server.apiAsk(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusServiceUnavailable, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "canonical engine selection and receipt controller is not configured") {
		t.Fatalf("missing-controller response = %s", resp.Body.String())
	}
}

func TestRegisteredAskRequiresLocalCapabilityBeforePrompting(t *testing.T) {
	const capability = "pantheon-dashboard-local-capability-test"
	controller := &askInvocationTracker{}
	server := New(Config{
		SNELocalAccessToken: capability,
		EngineSelection:     controller,
	})
	ts := testServer(t, server.cfg)
	defer ts.Close()

	response, err := http.Post(ts.URL+"/api/ask", "application/json", strings.NewReader(`{"question":"inspect this machine"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized diagnostics prompt status=%d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
	if controller.called {
		t.Fatal("unauthorized request reached the selected engine")
	}
}

func TestAPIAskRejectsAnswerWithoutVerifiableRouteReceipt(t *testing.T) {
	server := New(Config{EngineSelection: askControllerWithoutReceipt{}})
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"question":"what should I address?"}`))
	resp := httptest.NewRecorder()
	server.apiAsk(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusServiceUnavailable, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "selected engine returned no verifiable route receipt") {
		t.Fatalf("missing-receipt response = %s", resp.Body.String())
	}
}

func TestAPIAskRejectsAmbiguousOrUnexpectedRequestJSON(t *testing.T) {
	bodies := [][]byte{
		[]byte(`{"question":"first","question":"second"}`),
		[]byte(`{"question":"first","Question":"second"}`),
		[]byte(`{"question":"what?","unexpected":true}`),
		[]byte(`{"question":"what?"}{"question":"again"}`),
		append([]byte(`{"question":"`), append([]byte{0xff}, []byte(`"}`)...)...),
	}
	for _, body := range bodies {
		server := New(Config{})
		req := httptest.NewRequest(http.MethodPost, "/api/ask", bytes.NewReader(body))
		resp := httptest.NewRecorder()
		server.apiAsk(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("body %q status = %d, want %d: %s", body, resp.Code, http.StatusBadRequest, resp.Body.String())
		}
	}
}

func TestAPIAskBoundsRequestBodyBeforeDiagnosticsOrRouting(t *testing.T) {
	server := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"question":"`+strings.Repeat("x", maxAskRequestBody)+`"}`))
	resp := httptest.NewRecorder()
	server.apiAsk(resp, req)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized prompt status = %d, want %d: %s", resp.Code, http.StatusRequestEntityTooLarge, resp.Body.String())
	}
}

func TestAPIAskDistinguishesBodyReadFailureFromSizeLimit(t *testing.T) {
	server := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/api/ask", nil)
	req.Body = failingRequestBody{}
	resp := httptest.NewRecorder()
	server.apiAsk(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("body read failure status = %d, want 400: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "could not read request body") {
		t.Fatalf("body read failure response = %s", resp.Body.String())
	}
}

func TestAPIAskPropagatesBrowserCancellationToEngineContext(t *testing.T) {
	controller := &askCancellableController{started: make(chan struct{})}
	server := New(Config{EngineSelection: controller})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"question":"cancel this"}`)).WithContext(ctx)
	resp := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.apiAsk(resp, req)
		close(done)
	}()

	select {
	case <-controller.started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach the engine executor")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("engine executor did not return after request cancellation")
	}
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("cancelled request status = %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
}

func TestAskSelectedEngineReturnsRouteBoundReceipt(t *testing.T) {
	var captured engine.PromptRequest
	const completionText = `{"findings":[0],"summary":"the selected engine answered"}`
	identity := engine.Identity{
		Engine: engine.KindSNE, Variant: engine.VariantSNEPlain,
		EngineVersion: "sne-v3", ModelID: "sne-model", ModelSHA256: strings.Repeat("a", 64),
		TokenizerID: "sne-tokenizer", TokenizerSHA256: strings.Repeat("b", 64),
		Precision: "bf16", CacheNamespace: "sne-cache",
	}
	identityDigest, err := identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	completionSum := sha256.Sum256([]byte(completionText))
	executor := askPromptExecutor{
		completion: engine.Completion{Model: "sne-model", Text: completionText},
		receipt: engine.Receipt{
			ABIVersion: engine.ABIVersion, SessionID: "ask-session", Identity: identity,
			IdentityDigest: identityDigest, RequestSHA256: strings.Repeat("c", 64),
			CompletionSHA256: hex.EncodeToString(completionSum[:]),
			StartedAt:        "2026-09-07T16:00:00Z", FinishedAt: "2026-09-07T16:00:01Z",
			Route: &engine.RouteDecision{Requested: engine.KindSNE, RequestedVariant: engine.VariantSNEPlain, Selected: engine.KindSNE, SelectedVariant: engine.VariantSNEPlain, DataBoundary: engine.DataBoundaryNotDisclosed, Rationale: "preferred sne connector admitted"},
		},
		request: &captured,
	}
	selection, model, receipt, err := askSelectedEngine(context.Background(), executor, "what is healthy?", "[0] OK health")
	if err != nil {
		t.Fatal(err)
	}
	if model != "sne-model" || len(selection.Findings) != 1 || receipt == nil {
		t.Fatalf("selection result = %+v, model=%q, receipt=%+v", selection, model, receipt)
	}
	if receipt.Route == nil || receipt.Route.Selected != engine.KindSNE || receipt.RequestSHA256 == "" {
		t.Fatalf("receipt lost route/request identity: %+v", receipt)
	}
	if !strings.HasPrefix(captured.System, "You are Sirsi Pantheon's local machine-diagnostics assistant.") || strings.Contains(captured.System, "You are Horus") {
		t.Fatalf("engine received stale product identity in system prompt: %q", captured.System)
	}
	if captured.Prompt != "what is healthy?" || !strings.Contains(captured.System, "[0] OK health") {
		t.Fatalf("selected-engine prompt lost question or diagnostic grounding: %+v", captured)
	}
	digestDrift := executor
	digestDrift.receipt.CompletionSHA256 = strings.Repeat("d", 64)
	if _, _, _, err := askSelectedEngine(context.Background(), digestDrift, "what is healthy?", "[0] OK health"); err == nil || !strings.Contains(err.Error(), "does not match its route receipt digest") {
		t.Fatalf("answer with completion/receipt digest drift was accepted: %v", err)
	}
	invalid := executor
	invalidReceipt := invalid.receipt
	invalidRoute := *invalidReceipt.Route
	invalidRoute.DataBoundary = ""
	invalidReceipt.Route = &invalidRoute
	invalid.receipt = invalidReceipt
	if _, _, _, err := askSelectedEngine(context.Background(), invalid, "what is healthy?", "[0] OK health"); err == nil || !strings.Contains(err.Error(), "route data boundary is required") {
		t.Fatalf("answer with incomplete route provenance was accepted: %v", err)
	}
}

func TestAskSelectedEngineUsesExactConfiguredConnectorAcrossAllVariants(t *testing.T) {
	routes := []struct {
		kind    engine.Kind
		variant engine.BackendVariant
	}{
		{kind: engine.KindMLX, variant: engine.VariantMLXRaw},
		{kind: engine.KindMLX, variant: engine.VariantMLXPatched},
		{kind: engine.KindOMLX, variant: engine.VariantOMLXPublic},
		{kind: engine.KindSNE, variant: engine.VariantSNEPlain},
		{kind: engine.KindSNE, variant: engine.VariantSNEMTP},
	}
	for routeIndex, selected := range routes {
		t.Run(string(selected.kind)+"/"+string(selected.variant), func(t *testing.T) {
			decoy := routes[(routeIndex+1)%len(routes)]
			providers := make(map[engine.BackendVariant]*askRouteProvider, 2)
			connectors := make([]engine.Connector, 0, 2)
			for _, route := range []struct {
				kind    engine.Kind
				variant engine.BackendVariant
			}{selected, decoy} {
				if _, exists := providers[route.variant]; exists {
					continue
				}
				identity := engine.Identity{
					Engine: route.kind, Variant: route.variant, EngineVersion: "test-1",
					ModelID:     string(route.kind) + "-" + string(route.variant) + "-test-model",
					ModelSHA256: strings.Repeat("a", 64), TokenizerID: "test-tokenizer",
					TokenizerSHA256: strings.Repeat("b", 64), Precision: "bf16", CacheNamespace: "dashboard-route-test",
				}
				if route.variant == engine.VariantSNEMTP {
					identity.Assistant = &engine.AssistantIdentity{
						ModelID: "assistant-test-model", Revision: "test-revision",
						CheckpointSHA256: strings.Repeat("c", 64), Precision: "int8",
					}
				}
				backend := &askRouteProvider{model: identity.ModelID}
				capabilities := engine.Capabilities{
					Sessions: true, Receipts: true, MTP: route.variant == engine.VariantSNEMTP,
				}
				var connector engine.ProviderConnector
				var err error
				switch route.kind {
				case engine.KindMLX:
					connector, err = engine.NewMLXConnector(backend, identity, capabilities)
				case engine.KindOMLX:
					connector, err = engine.NewOMLXConnector(backend, identity, capabilities)
				case engine.KindSNE:
					connector, err = engine.NewSNEConnector(backend, identity, capabilities)
				default:
					t.Fatalf("unsupported test engine %q", route.kind)
				}
				if err != nil {
					t.Fatal(err)
				}
				providers[route.variant] = backend
				connectors = append(connectors, connector)
			}
			router, err := engine.NewRouter(connectors...)
			if err != nil {
				t.Fatal(err)
			}
			controller, err := engine.NewSelectionController(router, engine.RoutePolicy{
				Preferred: decoy.kind, PreferredVariant: decoy.variant,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.Select(engine.RoutePolicy{
				Preferred: selected.kind, PreferredVariant: selected.variant,
			}); err != nil {
				t.Fatal(err)
			}

			const question = "which configured connector answered?"
			selection, model, receipt, err := askSelectedEngine(context.Background(), controller, question, "[0] [OK] verified fixture")
			if err != nil {
				t.Fatal(err)
			}
			if len(selection.Findings) != 1 || selection.Findings[0] != 0 || model != providers[selected.variant].model {
				t.Fatalf("dashboard selection/model = %+v/%q", selection, model)
			}
			for variant, backend := range providers {
				wantCalls := 0
				if variant == selected.variant {
					wantCalls = 1
				}
				if len(backend.requests) != wantCalls {
					t.Errorf("%s completion calls = %d, want %d", variant, len(backend.requests), wantCalls)
				}
				if wantCalls == 1 && (backend.requests[0].Prompt != question || !strings.Contains(backend.requests[0].System, "[0] [OK] verified fixture")) {
					t.Errorf("selected connector request lost dashboard question or diagnostic grounding: %+v", backend.requests[0])
				}
			}
			if receipt == nil || receipt.Route == nil || receipt.Route.Requested != selected.kind || receipt.Route.RequestedVariant != selected.variant || receipt.Route.Selected != selected.kind || receipt.Route.SelectedVariant != selected.variant || receipt.Identity.ModelID != providers[selected.variant].model {
				t.Fatalf("dashboard receipt = %+v, want exact %s/%s route and identity", receipt, selected.kind, selected.variant)
			}
			if selected.variant == engine.VariantSNEMTP && receipt.Identity.Assistant == nil {
				t.Fatal("Apollo Flash dashboard receipt lost assistant identity")
			}
		})
	}
}

func TestAPIAskUsesRoutePolicyCapturedBeforeDiagnostics(t *testing.T) {
	identityFor := func(kind engine.Kind, variant engine.BackendVariant, model string) engine.Identity {
		return engine.Identity{
			Engine: kind, Variant: variant, EngineVersion: "test-1", ModelID: model,
			ModelSHA256: strings.Repeat("a", 64), TokenizerID: "test-tokenizer",
			TokenizerSHA256: strings.Repeat("b", 64), Precision: "bf16", CacheNamespace: "ask-policy-snapshot-test",
		}
	}
	mlxProvider := &askRouteProvider{model: "mlx-test-model"}
	mlx, err := engine.NewMLXConnector(mlxProvider, identityFor(engine.KindMLX, engine.VariantMLXRaw, mlxProvider.model), engine.Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	sneProvider := &askRouteProvider{model: "sne-test-model"}
	sne, err := engine.NewSNEConnector(sneProvider, identityFor(engine.KindSNE, engine.VariantSNEPlain, sneProvider.model), engine.Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := engine.NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := engine.NewSelectionController(router, engine.RoutePolicy{
		Preferred: engine.KindSNE, PreferredVariant: engine.VariantSNEPlain,
		RequiredCapabilities: []engine.Capability{engine.CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		EngineSelection: &askPolicySwitchController{
			SelectionController: controller,
			switchTo:            engine.RoutePolicy{Preferred: engine.KindMLX, PreferredVariant: engine.VariantMLXRaw},
		},
		SNELocalAccessToken: "pantheon-dashboard-route-snapshot-capability",
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/ask", strings.NewReader(`{"question":"which route was accepted?"}`))
	request.Host = "127.0.0.1"
	request.Header.Set("Authorization", "Bearer pantheon-dashboard-route-snapshot-capability")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/ask status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response askResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(sneProvider.requests) != 1 || len(mlxProvider.requests) != 0 {
		t.Fatalf("requests after concurrent route change: SNE=%d MLX=%d", len(sneProvider.requests), len(mlxProvider.requests))
	}
	if response.Receipt == nil || response.Receipt.Route == nil || response.Receipt.Route.Requested != engine.KindSNE || response.Receipt.Route.Selected != engine.KindSNE || response.Receipt.Route.RequestedVariant != engine.VariantSNEPlain {
		t.Fatalf("response receipt does not preserve accepted SNE route: %+v", response.Receipt)
	}
	if got := controller.Policy().Preferred; got != engine.KindMLX {
		t.Fatalf("test did not change current policy during request, got %q", got)
	}
}
