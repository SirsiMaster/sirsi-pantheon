package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type enginePromptTestProvider struct{}

func (enginePromptTestProvider) Name() string                   { return "test-model" }
func (enginePromptTestProvider) Tier() provider.Tier            { return provider.TierLocal }
func (enginePromptTestProvider) Caps() provider.Caps            { return provider.Caps{} }
func (enginePromptTestProvider) Available(context.Context) bool { return true }
func (enginePromptTestProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Model: "test-model", Text: "unused"}, nil
}
func (enginePromptTestProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.StreamChunk, error) {
	chunks := make(chan provider.StreamChunk, 2)
	chunks <- provider.StreamChunk{Model: "test-model", Text: "answer"}
	chunks <- provider.StreamChunk{Model: "test-model", FinishReason: "stop", Done: true}
	close(chunks)
	return chunks, nil
}

type enginePromptRecordingProvider struct {
	enginePromptTestProvider
	model    string
	requests []provider.Request
}

func (p *enginePromptRecordingProvider) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	p.requests = append(p.requests, request)
	return provider.Response{Model: p.model, Text: "completed: " + request.Prompt}, nil
}

type enginePromptStreamRecordingProvider struct {
	enginePromptTestProvider
	model          string
	streamRequests []provider.Request
	started        chan struct{}
}

func (p *enginePromptStreamRecordingProvider) Stream(_ context.Context, request provider.Request) (<-chan provider.StreamChunk, error) {
	p.streamRequests = append(p.streamRequests, request)
	if p.started != nil {
		close(p.started)
		return make(chan provider.StreamChunk), nil
	}
	chunks := make(chan provider.StreamChunk, 2)
	chunks <- provider.StreamChunk{Model: p.model, Text: "streamed answer"}
	chunks <- provider.StreamChunk{Model: p.model, FinishReason: "stop", Done: true}
	close(chunks)
	return chunks, nil
}

type enginePromptTestRoute struct {
	kind    engine.Kind
	variant engine.BackendVariant
}

var enginePromptTestRoutes = []enginePromptTestRoute{
	{kind: engine.KindMLX, variant: engine.VariantMLXRaw},
	{kind: engine.KindMLX, variant: engine.VariantMLXPatched},
	{kind: engine.KindOMLX, variant: engine.VariantOMLXPublic},
	{kind: engine.KindSNE, variant: engine.VariantSNEPlain},
	{kind: engine.KindSNE, variant: engine.VariantSNEMTP},
}

func testEngineStreamingController(t *testing.T, selected, decoy enginePromptTestRoute, selectedStarted chan struct{}) (*engine.SelectionController, map[engine.BackendVariant]*enginePromptStreamRecordingProvider) {
	t.Helper()
	providers := make(map[engine.BackendVariant]*enginePromptStreamRecordingProvider, 2)
	connectors := make([]engine.Connector, 0, 2)
	for _, route := range []enginePromptTestRoute{selected, decoy} {
		if _, exists := providers[route.variant]; exists {
			continue
		}
		identity := testEngineIdentity()
		identity.Engine = route.kind
		identity.Variant = route.variant
		identity.ModelID = string(route.kind) + "-" + string(route.variant) + "-test-model"
		if route.variant == engine.VariantSNEMTP {
			identity.Assistant = &engine.AssistantIdentity{
				ModelID: "assistant-test-model", Revision: "test-revision",
				CheckpointSHA256: strings.Repeat("c", 64), Precision: "int8",
			}
		}
		var started chan struct{}
		if route.variant == selected.variant {
			started = selectedStarted
		}
		backend := &enginePromptStreamRecordingProvider{model: identity.ModelID, started: started}
		capabilities := engine.Capabilities{
			Sessions: true, Receipts: true, Streaming: true, Cancellation: true,
			MTP: route.variant == engine.VariantSNEMTP,
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
	return controller, providers
}

func testEngineIdentity() engine.Identity {
	return engine.Identity{
		Engine: engine.KindMLX, EngineVersion: "test-1", ModelID: "test-model",
		ModelSHA256: strings.Repeat("a", 64), TokenizerID: "test-tokenizer",
		TokenizerSHA256: strings.Repeat("b", 64), Precision: "bf16", CacheNamespace: "test-cache",
	}
}

func testEngineController(t *testing.T) *engine.SelectionController {
	t.Helper()
	connector, err := engine.NewMLXConnector(enginePromptTestProvider{}, testEngineIdentity(), engine.Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := engine.NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := engine.NewSelectionController(router, engine.RoutePolicy{Preferred: engine.KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	return controller
}

func testStreamIdentityAndRoute(t *testing.T) (engine.Session, engine.RouteDecision, engine.Receipt) {
	t.Helper()
	identity := testEngineIdentity()
	identityDigest, err := identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	completionSum := sha256.Sum256([]byte("answer"))
	route := engine.RouteDecision{
		Requested: engine.KindMLX, RequestedVariant: engine.VariantMLXRaw,
		Selected: engine.KindMLX, SelectedVariant: engine.VariantMLXRaw,
		DataBoundary: engine.DataBoundaryNotDisclosed, Rationale: "test route",
	}
	session := engine.Session{ID: "session-1", Identity: identity, CreatedAt: "2026-09-08T12:00:00Z"}
	receipt := engine.Receipt{
		ABIVersion: engine.ABIVersion, SessionID: session.ID, Identity: identity, IdentityDigest: identityDigest,
		RequestSHA256: strings.Repeat("c", 64), CompletionSHA256: hex.EncodeToString(completionSum[:]),
		StartedAt: session.CreatedAt, FinishedAt: "2026-09-08T12:00:01Z", Route: &route,
	}
	return session, route, receipt
}

func TestParseEnginePromptOptionsRejectsInvalidInput(t *testing.T) {
	for name, args := range map[string][]string{
		"missing prompt":      {"--max-tokens", "8"},
		"empty prompt":        {"--prompt", "   "},
		"missing engine":      {"--prompt", "hello", "--variant", "mlx-raw"},
		"missing variant":     {"--prompt", "hello", "--engine", "mlx"},
		"invalid engine":      {"--prompt", "hello", "--engine", "cuda"},
		"invalid variant":     {"--prompt", "hello", "--variant", "cuda"},
		"zero max tokens":     {"--prompt", "hello", "--max-tokens", "0"},
		"negative max tokens": {"--prompt", "hello", "--max-tokens", "-1"},
		"positional argument": {"--prompt", "hello", "unexpected"},
		"unknown flag":        {"--prompt", "hello", "--unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseEnginePromptOptions(args); err == nil {
				t.Fatal("invalid prompt arguments were accepted")
			}
		})
	}
}

func TestParseEnginePromptOptionsDefaultsToPositiveLimit(t *testing.T) {
	opts, err := parseEnginePromptOptions([]string{"--engine", "mlx", "--variant", "mlx-raw", "--prompt", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.MaxTokens <= 0 {
		t.Fatalf("default max tokens = %d, want positive", opts.MaxTokens)
	}
}

func TestEnginePromptVariantHelpExplainsStackLabNamesAndCanonicalValues(t *testing.T) {
	variantFlag := enginePromptCmd.Flags().Lookup("variant")
	if variantFlag == nil {
		t.Fatal("engine prompt --variant flag is missing")
	}
	for _, want := range []string{
		"Apollo (Plain)=sne-plain",
		"Apollo Flash (Speculative)=sne-mtp",
		"mlx-raw|mlx-patched|omlx-public|sne-plain|sne-mtp",
	} {
		if !strings.Contains(variantFlag.Usage, want) {
			t.Errorf("engine prompt variant help missing %q: %s", want, variantFlag.Usage)
		}
	}
}

func TestParseEnginePromptOptionsAcceptsEveryPublishedEngineVariantPair(t *testing.T) {
	for _, tc := range []struct {
		engine  string
		variant string
	}{
		{engine: "sne", variant: "sne-plain"},
		{engine: "sne", variant: "sne-mtp"},
		{engine: "mlx", variant: "mlx-raw"},
		{engine: "mlx", variant: "mlx-patched"},
		{engine: "omlx", variant: "omlx-public"},
	} {
		t.Run(tc.engine+"/"+tc.variant, func(t *testing.T) {
			opts, err := parseEnginePromptOptions([]string{
				"--engine", tc.engine, "--variant", tc.variant, "--prompt", "hello",
			})
			if err != nil {
				t.Fatal(err)
			}
			if opts.Engine != tc.engine || opts.Variant != tc.variant {
				t.Fatalf("parsed route = %s/%s, want %s/%s", opts.Engine, opts.Variant, tc.engine, tc.variant)
			}
		})
	}
}

func TestParseEnginePromptOptionsAcceptsStreaming(t *testing.T) {
	opts, err := parseEnginePromptOptions([]string{
		"--engine", "mlx", "--variant", "mlx-raw", "--prompt", "hello", "--stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Stream {
		t.Fatal("--stream was not retained in prompt options")
	}
}

func TestParseEnginePromptOptionsRejectsVariantForDifferentEngine(t *testing.T) {
	for _, tc := range []struct {
		engine  string
		variant string
	}{
		{engine: "mlx", variant: "omlx-public"},
		{engine: "omlx", variant: "mlx-raw"},
		{engine: "sne", variant: "mlx-patched"},
	} {
		_, err := parseEnginePromptOptions([]string{
			"--engine", tc.engine, "--variant", tc.variant, "--prompt", "hello",
		})
		if err == nil || !strings.Contains(err.Error(), "incompatible with engine") {
			t.Errorf("engine=%s variant=%s: error=%v, want compatibility rejection", tc.engine, tc.variant, err)
		}
	}
}

func TestExecuteEnginePromptRejectsIncompatibleRouteBeforeConnectorSetup(t *testing.T) {
	built := false
	err := executeEnginePrompt(context.Background(), enginePromptOptions{
		Engine: "omlx", Variant: "mlx-raw", Prompt: "hello", MaxTokens: 8,
	}, &bytes.Buffer{}, func() (*engine.SelectionController, error) {
		built = true
		return nil, nil
	}, func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
		t.Fatal("executor called for an incompatible engine/variant pair")
		return engine.Completion{}, engine.Receipt{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "incompatible with engine") {
		t.Fatalf("incompatible engine/variant error = %v", err)
	}
	if built {
		t.Fatal("connector setup ran before rejecting an incompatible engine/variant pair")
	}
}

func TestExecuteEnginePromptRejectsImplicitConfiguredRouteBeforeConnectorSetup(t *testing.T) {
	built := false
	err := executeEnginePrompt(context.Background(), enginePromptOptions{
		Prompt: "hello", MaxTokens: 8,
	}, &bytes.Buffer{}, func() (*engine.SelectionController, error) {
		built = true
		return nil, nil
	}, func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
		t.Fatal("executor called without an explicit route")
		return engine.Completion{}, engine.Receipt{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "--engine is required") {
		t.Fatalf("implicit route error = %v, want explicit engine requirement", err)
	}
	if built {
		t.Fatal("connector setup ran before rejecting an implicit configured route")
	}
}

func TestEngineCLIWorkflowIsRegisteredAndDocumented(t *testing.T) {
	if engineCmd.Parent() != rootCmd {
		t.Fatal("engine command is not registered on the root command")
	}
	if engineStatusCmd.Parent() != engineCmd || enginePromptCmd.Parent() != engineCmd {
		t.Fatal("engine status/prompt commands are not registered under engine")
	}
	for _, want := range []string{"sirsi engine status", "sirsi engine prompt", "explicit engine route"} {
		if !strings.Contains(rootCmd.Long, want) {
			t.Errorf("root help does not document %q", want)
		}
	}
}

func TestExecuteEnginePromptProjectsCompletionAndRouteReceipt(t *testing.T) {
	controller := testEngineController(t)
	opts := enginePromptOptions{Engine: "mlx", Variant: "mlx-raw", Prompt: "hello", MaxTokens: 8}
	var out bytes.Buffer
	identity := testEngineIdentity()
	identityDigest, err := identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	completion := engine.Completion{Text: "answer", Model: identity.ModelID, FinishReason: "stop"}
	completionSum := sha256.Sum256([]byte(completion.Text))
	err = executeEnginePrompt(context.Background(), opts, &out,
		func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
			return completion, engine.Receipt{
				ABIVersion: engine.ABIVersion, SessionID: "session-1", Identity: identity,
				IdentityDigest: identityDigest, RequestSHA256: strings.Repeat("c", 64), CompletionSHA256: hex.EncodeToString(completionSum[:]),
				StartedAt: "2026-09-08T12:00:00Z", FinishedAt: "2026-09-08T12:00:01Z",
				Route: &engine.RouteDecision{Requested: engine.KindMLX, RequestedVariant: engine.VariantMLXRaw, Selected: engine.KindMLX, SelectedVariant: engine.VariantMLXRaw, DataBoundary: engine.DataBoundaryNotDisclosed, Rationale: "test route"},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var got enginePromptOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Completion.Text != "answer" || got.Receipt.Route == nil || got.Receipt.Route.Selected != engine.KindMLX || got.Receipt.Route.SelectedVariant != engine.VariantMLXRaw || got.Receipt.Route.DataBoundary != engine.DataBoundaryNotDisclosed {
		t.Fatalf("unexpected prompt output: %+v", got)
	}
}

func TestExecuteEnginePromptRunsTheExplicitRouteThroughTheSelectedConnector(t *testing.T) {
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
	for routeIndex, route := range routes {
		t.Run(string(route.kind)+"/"+string(route.variant), func(t *testing.T) {
			providers := make(map[engine.BackendVariant]*enginePromptRecordingProvider, 2)
			connectors := make([]engine.Connector, 0, 2)
			for _, candidate := range []engine.BackendVariant{route.variant, routes[(routeIndex+1)%len(routes)].variant} {
				if _, exists := providers[candidate]; exists {
					continue
				}
				kind := route.kind
				if candidate != route.variant {
					kind = routes[(routeIndex+1)%len(routes)].kind
				}
				identity := testEngineIdentity()
				identity.Engine = kind
				identity.Variant = candidate
				identity.ModelID = string(kind) + "-" + string(candidate) + "-test-model"
				if candidate == engine.VariantSNEMTP {
					identity.Assistant = &engine.AssistantIdentity{
						ModelID: "assistant-test-model", Revision: "test-revision",
						CheckpointSHA256: strings.Repeat("c", 64), Precision: "int8",
					}
				}
				provider := &enginePromptRecordingProvider{model: identity.ModelID}
				capabilities := engine.Capabilities{Sessions: true, Receipts: true}
				if candidate == engine.VariantSNEMTP {
					capabilities.MTP = true
				}
				var connector engine.ProviderConnector
				var err error
				switch kind {
				case engine.KindMLX:
					connector, err = engine.NewMLXConnector(provider, identity, capabilities)
				case engine.KindOMLX:
					connector, err = engine.NewOMLXConnector(provider, identity, capabilities)
				case engine.KindSNE:
					connector, err = engine.NewSNEConnector(provider, identity, capabilities)
				default:
					t.Fatalf("unsupported test engine %q", kind)
				}
				if err != nil {
					t.Fatal(err)
				}
				providers[candidate] = provider
				connectors = append(connectors, connector)
			}

			router, err := engine.NewRouter(connectors...)
			if err != nil {
				t.Fatal(err)
			}
			configured := routes[(routeIndex+1)%len(routes)]
			controller, err := engine.NewSelectionController(router, engine.RoutePolicy{
				Preferred: configured.kind, PreferredVariant: configured.variant,
			})
			if err != nil {
				t.Fatal(err)
			}

			const prompt = "report the selected route"
			var out bytes.Buffer
			err = executeEnginePrompt(context.Background(), enginePromptOptions{
				Engine: string(route.kind), Variant: string(route.variant), Prompt: prompt, MaxTokens: 12,
			}, &out, func() (*engine.SelectionController, error) { return controller, nil }, completeEnginePrompt)
			if err != nil {
				t.Fatal(err)
			}
			for variant, provider := range providers {
				wantCalls := 0
				if variant == route.variant {
					wantCalls = 1
				}
				if len(provider.requests) != wantCalls {
					t.Errorf("%s provider calls = %d, want %d", variant, len(provider.requests), wantCalls)
				}
				if wantCalls == 1 {
					got := provider.requests[0]
					if got.Prompt != prompt || got.MaxTokens != 12 {
						t.Errorf("selected provider request = %+v, want the CLI prompt and token limit", got)
					}
				}
			}
			var output enginePromptOutput
			if err := json.Unmarshal(out.Bytes(), &output); err != nil {
				t.Fatal(err)
			}
			selected := providers[route.variant]
			if output.Completion.Text != "completed: "+prompt || output.Receipt.Identity.ModelID != selected.model {
				t.Fatalf("completion/identity = %q/%q, want selected model %q", output.Completion.Text, output.Receipt.Identity.ModelID, selected.model)
			}
			if output.Receipt.Route == nil || output.Receipt.Route.Requested != route.kind || output.Receipt.Route.RequestedVariant != route.variant || output.Receipt.Route.Selected != route.kind || output.Receipt.Route.SelectedVariant != route.variant {
				t.Fatalf("route receipt = %+v, want explicit %s/%s", output.Receipt.Route, route.kind, route.variant)
			}
			if route.variant == engine.VariantSNEMTP && output.Receipt.Identity.Assistant == nil {
				t.Fatal("Apollo Flash receipt lost its assistant identity")
			}
		})
	}
}

func TestExecuteEnginePromptRejectsCompletionReceiptDrift(t *testing.T) {
	controller := testEngineController(t)
	identity := testEngineIdentity()
	identityDigest, err := identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	completion := engine.Completion{Text: "answer", Model: identity.ModelID, FinishReason: "stop"}
	completionSum := sha256.Sum256([]byte(completion.Text))
	validReceipt := engine.Receipt{
		ABIVersion: engine.ABIVersion, SessionID: "session-1", Identity: identity,
		IdentityDigest: identityDigest, RequestSHA256: strings.Repeat("c", 64), CompletionSHA256: hex.EncodeToString(completionSum[:]),
		StartedAt: "2026-09-08T12:00:00Z", FinishedAt: "2026-09-08T12:00:01Z",
		Route: &engine.RouteDecision{Requested: engine.KindMLX, RequestedVariant: engine.VariantMLXRaw, Selected: engine.KindMLX, SelectedVariant: engine.VariantMLXRaw, DataBoundary: engine.DataBoundaryNotDisclosed, Rationale: "test route"},
	}
	for name, mutate := range map[string]func(*engine.Completion, *engine.Receipt){
		"wrong served model":      func(c *engine.Completion, _ *engine.Receipt) { c.Model = "other-model" },
		"wrong identity digest":   func(_ *engine.Completion, r *engine.Receipt) { r.IdentityDigest = strings.Repeat("0", 64) },
		"wrong completion digest": func(_ *engine.Completion, r *engine.Receipt) { r.CompletionSHA256 = strings.Repeat("0", 64) },
		"different requested route": func(_ *engine.Completion, r *engine.Receipt) {
			r.Route.Requested = engine.KindOMLX
			r.Route.RequestedVariant = engine.VariantOMLXPublic
			r.Route.Fallback = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			gotCompletion, gotReceipt := completion, validReceipt
			gotReceipt.Route = func() *engine.RouteDecision { route := *validReceipt.Route; return &route }()
			mutate(&gotCompletion, &gotReceipt)
			var out bytes.Buffer
			err := executeEnginePrompt(context.Background(), enginePromptOptions{
				Engine: "mlx", Variant: "mlx-raw", Prompt: "hello", MaxTokens: 8,
			}, &out, func() (*engine.SelectionController, error) { return controller, nil },
				func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
					return gotCompletion, gotReceipt, nil
				})
			if err == nil {
				t.Fatal("CLI accepted completion/receipt drift")
			}
			if out.Len() != 0 {
				t.Fatalf("CLI published unverified completion: %s", out.String())
			}
		})
	}
}

func TestExecuteEnginePromptRejectsFallbackNotAllowedByInvocation(t *testing.T) {
	controller := testEngineController(t)
	identity := testEngineIdentity()
	identity.Engine = engine.KindSNE
	identity.Variant = engine.VariantSNEPlain
	identityDigest, err := identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	completion := engine.Completion{Text: "answer", Model: identity.ModelID}
	completionSum := sha256.Sum256([]byte(completion.Text))
	route := engine.RouteDecision{
		Requested: engine.KindMLX, RequestedVariant: engine.VariantMLXRaw,
		Selected: engine.KindSNE, SelectedVariant: engine.VariantSNEPlain,
		DataBoundary: engine.DataBoundaryNotDisclosed, Fallback: true, Rationale: "test fallback",
	}
	receipt := engine.Receipt{
		ABIVersion: engine.ABIVersion, SessionID: "session-1", Identity: identity, IdentityDigest: identityDigest,
		RequestSHA256: strings.Repeat("c", 64), CompletionSHA256: hex.EncodeToString(completionSum[:]),
		StartedAt: "2026-09-08T12:00:00Z", FinishedAt: "2026-09-08T12:00:01Z", Route: &route,
	}
	var out bytes.Buffer
	err = executeEnginePrompt(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
			return completion, receipt, nil
		})
	if err == nil || !strings.Contains(err.Error(), "did not allow it") {
		t.Fatalf("unrequested fallback error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("CLI published a fallback result that the invocation did not allow: %s", out.String())
	}
}

func TestExecuteEnginePromptRejectsReceiptWithoutRouteBoundary(t *testing.T) {
	controller := testEngineController(t)
	var out bytes.Buffer
	err := executeEnginePrompt(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
			return engine.Completion{Text: "answer", Model: "test-model"}, engine.Receipt{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "missing route boundary provenance") {
		t.Fatalf("completion without route boundary error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("CLI published output without route provenance: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamEmitsSessionRouteAndEventJSONL(t *testing.T) {
	for routeIndex, route := range enginePromptTestRoutes {
		t.Run(string(route.kind)+"/"+string(route.variant), func(t *testing.T) {
			decoy := enginePromptTestRoutes[(routeIndex+1)%len(enginePromptTestRoutes)]
			controller, providers := testEngineStreamingController(t, route, decoy, nil)
			const prompt = "stream through the selected route"
			var out bytes.Buffer
			err := executeEnginePromptStream(context.Background(), enginePromptOptions{
				Engine: string(route.kind), Variant: string(route.variant), Stream: true, Prompt: prompt, MaxTokens: 12,
			}, &out, func() (*engine.SelectionController, error) { return controller, nil }, streamEnginePrompt)
			if err != nil {
				t.Fatal(err)
			}
			for variant, backend := range providers {
				wantCalls := 0
				if variant == route.variant {
					wantCalls = 1
				}
				if len(backend.streamRequests) != wantCalls {
					t.Errorf("%s stream calls = %d, want %d", variant, len(backend.streamRequests), wantCalls)
				}
				if wantCalls == 1 {
					request := backend.streamRequests[0]
					if request.Prompt != prompt || request.MaxTokens != 12 {
						t.Errorf("selected stream request = %+v, want CLI prompt and token limit", request)
					}
				}
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != 3 {
				t.Fatalf("stream output has %d JSON lines, want session + delta + terminal: %s", len(lines), out.String())
			}
			var header enginePromptStreamOutput
			if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
				t.Fatal(err)
			}
			if header.Type != "session" || header.Session == nil || header.Session.ID == "" || header.Route == nil || header.Route.Selected != route.kind || header.Route.SelectedVariant != route.variant || header.Route.DataBoundary != engine.DataBoundaryNotDisclosed {
				t.Fatalf("stream header = %+v, want explicit %s/%s", header, route.kind, route.variant)
			}
			if header.Session.Identity.Engine != route.kind || header.Session.Identity.EffectiveVariant() != route.variant || header.Session.Identity.ModelID != providers[route.variant].model {
				t.Fatalf("stream session identity = %+v, want selected route identity", header.Session.Identity)
			}
			var delta enginePromptStreamOutput
			if err := json.Unmarshal([]byte(lines[1]), &delta); err != nil {
				t.Fatal(err)
			}
			if delta.Type != "event" || delta.Event == nil || delta.Event.Kind != engine.EventDelta || delta.Event.Text != "streamed answer" {
				t.Fatalf("stream delta = %+v", delta)
			}
			var terminal enginePromptStreamOutput
			if err := json.Unmarshal([]byte(lines[2]), &terminal); err != nil {
				t.Fatal(err)
			}
			if terminal.Type != "event" || terminal.Event == nil || terminal.Event.Kind != engine.EventCompleted || terminal.Event.Receipt == nil || terminal.Event.Receipt.Route == nil || terminal.Event.Receipt.Route.Selected != route.kind || terminal.Event.Receipt.Route.SelectedVariant != route.variant || terminal.Event.Receipt.Route.DataBoundary != engine.DataBoundaryNotDisclosed {
				t.Fatalf("stream terminal = %+v, want receipt bound to selected route", terminal)
			}
			if terminal.Event.Receipt.Identity.Engine != route.kind || terminal.Event.Receipt.Identity.EffectiveVariant() != route.variant || terminal.Event.Receipt.Identity.ModelID != providers[route.variant].model {
				t.Fatalf("stream receipt identity = %+v, want selected route identity", terminal.Event.Receipt.Identity)
			}
			if route.variant == engine.VariantSNEMTP && (header.Session.Identity.Assistant == nil || terminal.Event.Receipt.Identity.Assistant == nil) {
				t.Fatal("Apollo Flash stream lost assistant identity from its session or receipt")
			}
		})
	}
}

func TestExecuteEnginePromptStreamRejectsSessionWithoutRouteBoundary(t *testing.T) {
	controller := testEngineController(t)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: engine.Session{ID: "session-1"}, Events: make(chan engine.Event)}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "missing route boundary provenance") {
		t.Fatalf("stream session without route boundary error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("CLI published a stream header without route provenance: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamPropagatesCancellationAndEmitsReceipt(t *testing.T) {
	for routeIndex, route := range enginePromptTestRoutes {
		t.Run(string(route.kind)+"/"+string(route.variant), func(t *testing.T) {
			started := make(chan struct{})
			decoy := enginePromptTestRoutes[(routeIndex+1)%len(enginePromptTestRoutes)]
			controller, providers := testEngineStreamingController(t, route, decoy, started)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out bytes.Buffer
			done := make(chan error, 1)
			go func() {
				done <- executeEnginePromptStream(ctx, enginePromptOptions{
					Engine: string(route.kind), Variant: string(route.variant), Stream: true, Prompt: "cancel this stream", MaxTokens: 8,
				}, &out, func() (*engine.SelectionController, error) { return controller, nil }, streamEnginePrompt)
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("stream did not reach the selected provider")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "cancelled") {
					t.Fatalf("cancelled CLI stream error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("CLI stream did not terminate after cancellation")
			}
			if len(providers[route.variant].streamRequests) != 1 {
				t.Fatalf("selected route stream calls = %d, want 1", len(providers[route.variant].streamRequests))
			}
			if len(providers[decoy.variant].streamRequests) != 0 {
				t.Fatalf("decoy route stream calls = %d, want 0", len(providers[decoy.variant].streamRequests))
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != 2 {
				t.Fatalf("cancelled stream emitted %d records, want session + terminal: %s", len(lines), out.String())
			}
			var header enginePromptStreamOutput
			if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
				t.Fatal(err)
			}
			if header.Type != "session" || header.Route == nil || header.Route.Selected != route.kind || header.Route.SelectedVariant != route.variant {
				t.Fatalf("cancelled stream header = %+v, want selected route", header)
			}
			var terminal enginePromptStreamOutput
			if err := json.Unmarshal([]byte(lines[1]), &terminal); err != nil {
				t.Fatal(err)
			}
			if terminal.Type != "event" || terminal.Event == nil || terminal.Event.Kind != engine.EventError || terminal.Event.ErrorCode != "cancelled" || terminal.Event.Receipt == nil || !terminal.Event.Receipt.Cancelled {
				t.Fatalf("cancelled stream terminal = %+v", terminal)
			}
			if terminal.Event.Receipt.Route == nil || terminal.Event.Receipt.Route.Selected != route.kind || terminal.Event.Receipt.Route.SelectedVariant != route.variant || terminal.Event.Receipt.Identity.EffectiveVariant() != route.variant {
				t.Fatalf("cancelled receipt lost route/identity: %+v", terminal.Event.Receipt)
			}
			if route.variant == engine.VariantSNEMTP && terminal.Event.Receipt.Identity.Assistant == nil {
				t.Fatal("Apollo Flash cancellation receipt lost assistant identity")
			}
		})
	}
}

func TestExecuteEnginePromptStreamRejectsNilEventChannel(t *testing.T) {
	controller := testEngineController(t)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: engine.Session{ID: "session"}}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no event channel") {
		t.Fatalf("CLI accepted a nil event channel: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("CLI wrote a misleading partial stream header: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamRejectsEventsAfterTerminal(t *testing.T) {
	controller := testEngineController(t)
	session, route, receipt := testStreamIdentityAndRoute(t)
	events := make(chan engine.Event, 2)
	events <- engine.Event{Kind: engine.EventCompleted, SessionID: session.ID, Sequence: 1, Model: session.Identity.ModelID, Text: "answer", Receipt: &receipt}
	events <- engine.Event{Kind: engine.EventDelta, SessionID: "session-1", Sequence: 2, Text: "late data"}
	close(events)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{
				Session: session,
				Route:   route,
				Events:  events,
			}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "after its terminal record") {
		t.Fatalf("CLI stream accepted post-terminal data: %v", err)
	}
	if strings.Contains(out.String(), "late data") {
		t.Fatalf("CLI emitted data after the terminal record: %s", out.String())
	}
	if strings.Contains(out.String(), `"kind":"completed"`) {
		t.Fatalf("CLI published a completed terminal before validating end-of-stream: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamDoesNotPublishErrorBeforeEndOfStream(t *testing.T) {
	controller := testEngineController(t)
	session, route, _ := testStreamIdentityAndRoute(t)
	events := make(chan engine.Event, 2)
	events <- engine.Event{Kind: engine.EventError, SessionID: session.ID, Sequence: 1, ErrorCode: "upstream", Error: "first terminal"}
	events <- engine.Event{Kind: engine.EventDelta, SessionID: session.ID, Sequence: 2, Text: "late data"}
	close(events)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: session, Route: route, Events: events}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "after its terminal record") {
		t.Fatalf("CLI stream accepted data after its error terminal: %v", err)
	}
	if strings.Contains(out.String(), `"kind":"error"`) || strings.Contains(out.String(), "first terminal") {
		t.Fatalf("CLI published an error terminal before validating end-of-stream: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamRejectsRouteIdentityMismatchBeforeHeader(t *testing.T) {
	controller := testEngineController(t)
	session, route, _ := testStreamIdentityAndRoute(t)
	route.SelectedVariant = engine.VariantMLXPatched
	events := make(chan engine.Event)
	close(events)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: session, Route: route, Events: events}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "selected variant") {
		t.Fatalf("mismatched stream route error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("CLI emitted an unbound stream header: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamRejectsRoutePolicyMismatchBeforeHeader(t *testing.T) {
	controller := testEngineController(t)
	session, route, _ := testStreamIdentityAndRoute(t)
	route.Requested = engine.KindOMLX
	route.RequestedVariant = engine.VariantOMLXPublic
	route.Fallback = true
	events := make(chan engine.Event)
	close(events)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: session, Route: route, Events: events}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "differs from selected policy") {
		t.Fatalf("mismatched stream route policy error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("CLI emitted a stream header for a different requested route: %s", out.String())
	}
}

func TestExecuteEnginePromptStreamRejectsUnboundEventSequenceAndSession(t *testing.T) {
	controller := testEngineController(t)
	for name, makeEvents := range map[string]func() <-chan engine.Event{
		"foreign session": func() <-chan engine.Event {
			events := make(chan engine.Event, 1)
			events <- engine.Event{Kind: engine.EventDelta, SessionID: "other-session", Sequence: 1, Text: "unbound"}
			close(events)
			return events
		},
		"repeated sequence": func() <-chan engine.Event {
			events := make(chan engine.Event, 2)
			events <- engine.Event{Kind: engine.EventDelta, SessionID: "session-1", Sequence: 1, Text: "first"}
			events <- engine.Event{Kind: engine.EventDelta, SessionID: "session-1", Sequence: 1, Text: "replayed"}
			close(events)
			return events
		},
	} {
		t.Run(name, func(t *testing.T) {
			session, route, _ := testStreamIdentityAndRoute(t)
			var out bytes.Buffer
			err := executeEnginePromptStream(context.Background(), enginePromptOptions{
				Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
			}, &out, func() (*engine.SelectionController, error) { return controller, nil },
				func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
					return engine.PromptStream{Session: session, Route: route, Events: makeEvents()}, nil
				})
			if err == nil {
				t.Fatal("CLI accepted an unbound event stream")
			}
		})
	}
}

func TestExecuteEnginePromptStreamRejectsCompletionDigestDrift(t *testing.T) {
	controller := testEngineController(t)
	session, route, receipt := testStreamIdentityAndRoute(t)
	receipt.CompletionSHA256 = strings.Repeat("0", 64)
	events := make(chan engine.Event, 1)
	events <- engine.Event{Kind: engine.EventCompleted, SessionID: session.ID, Sequence: 1, Model: session.Identity.ModelID, Text: "answer", Receipt: &receipt}
	close(events)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: session, Route: route, Events: events}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "text digest differs") {
		t.Fatalf("completion digest drift error = %v", err)
	}
}

func TestExecuteEnginePromptStreamRejectsCompletionWithoutMatchingDeltas(t *testing.T) {
	controller := testEngineController(t)
	session, route, receipt := testStreamIdentityAndRoute(t)
	events := make(chan engine.Event, 1)
	events <- engine.Event{
		Kind: engine.EventCompleted, SessionID: session.ID, Sequence: 1,
		Model: session.Identity.ModelID, Text: "answer", Receipt: &receipt,
	}
	close(events)
	var out bytes.Buffer
	err := executeEnginePromptStream(context.Background(), enginePromptOptions{
		Engine: "mlx", Variant: "mlx-raw", Stream: true, Prompt: "hello", MaxTokens: 8,
	}, &out, func() (*engine.SelectionController, error) { return controller, nil },
		func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error) {
			return engine.PromptStream{Session: session, Route: route, Events: events}, nil
		})
	if err == nil || !strings.Contains(err.Error(), "completed stream text differs from forwarded deltas") {
		t.Fatalf("completion without matching deltas error = %v", err)
	}
	if strings.Contains(out.String(), `"kind":"completed"`) {
		t.Fatalf("CLI published an unbound completed result: %s", out.String())
	}
}
