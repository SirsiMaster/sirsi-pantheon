package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type selectionProvider struct {
	name  string
	caps  provider.Caps
	ready bool
}

type contextualSelectionProvider struct {
	selectionProvider
	resolved provider.Caps
}

func (p contextualSelectionProvider) CapabilitiesForContext(context.Context) (provider.Caps, error) {
	return p.resolved, nil
}

func (p selectionProvider) Name() string                   { return p.name }
func (p selectionProvider) Tier() provider.Tier            { return provider.TierLocal }
func (p selectionProvider) Caps() provider.Caps            { return p.caps }
func (p selectionProvider) Available(context.Context) bool { return p.ready }
func (p selectionProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Model: p.name}, nil
}

func selectionIdentity(kind Kind) Identity {
	return Identity{Engine: kind, EngineVersion: "1", ModelID: "model", ModelSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", TokenizerID: "tokenizer", TokenizerSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Precision: "bf16", CacheNamespace: "cache"}
}

func TestSelectionControllerPublishesConfiguredEnginePolicy(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true, caps: provider.Caps{Streaming: true}}, selectionIdentity(KindMLX), Capabilities{Sessions: true, Streaming: true})
	if err != nil {
		t.Fatal(err)
	}
	sne, err := NewSNEConnector(selectionProvider{name: "sne", ready: true}, selectionIdentity(KindSNE), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, AllowFallback: false, RequiredCapabilities: []Capability{CapabilitySessions}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.Schema != SelectionSchema || snapshot.Preferred != KindMLX || len(snapshot.Connectors) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.Connectors[0].Kind != KindMLX || snapshot.Connectors[1].Kind != KindSNE || snapshot.Connectors[0].Variant != VariantMLXRaw || snapshot.Connectors[1].Variant != VariantSNEPlain {
		t.Fatalf("connectors are not deterministic: %+v", snapshot.Connectors)
	}
	if snapshot.Connectors[0].DataBoundary != DataBoundaryNotDisclosed || snapshot.Connectors[1].DataBoundary != DataBoundaryNotDisclosed {
		t.Fatalf("endpoint boundaries were inferred without endpoint evidence: %+v", snapshot.Connectors)
	}
	if snapshot.Connectors[0].DisplayName != "MLX · Raw" || snapshot.Connectors[1].DisplayName != "Apollo (Plain)" {
		t.Fatalf("operator-facing names do not match canonical route identities: %+v", snapshot.Connectors)
	}
	if _, err := controller.Select(RoutePolicy{Preferred: KindSNE, AllowFallback: false, RequiredCapabilities: []Capability{CapabilitySessions}}); err != nil {
		t.Fatal(err)
	}
	if got := controller.Policy().Preferred; got != KindSNE {
		t.Fatalf("preferred engine = %q, want sne", got)
	}
	selected, err := controller.Select(RoutePolicy{Preferred: KindMLX, PreferredVariant: VariantMLXRaw, RequiredCapabilities: []Capability{CapabilitySessions}})
	if err != nil {
		t.Fatal(err)
	}
	if selected.PreferredVariant != VariantMLXRaw {
		t.Fatalf("preferred variant = %q", selected.PreferredVariant)
	}
}

func TestSelectionControllerSelectRoutePreservesOtherPolicy(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	sne, err := NewSNEConnector(selectionProvider{name: "sne", ready: true}, selectionIdentity(KindSNE), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := controller.SelectRoute(KindSNE, VariantSNEPlain)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Preferred != KindSNE || snapshot.PreferredVariant != VariantSNEPlain || !snapshot.AllowFallback || len(snapshot.RequiredCapabilities) != 1 || snapshot.RequiredCapabilities[0] != CapabilitySessions {
		t.Fatalf("route-only snapshot lost or changed policy: %+v", snapshot)
	}
	if _, err := controller.SelectRoute(KindOMLX, VariantOMLXPublic); err == nil {
		t.Fatal("route-only selection accepted an unconfigured connector")
	}
	policy := controller.Policy()
	if policy.Preferred != KindSNE || policy.PreferredVariant != VariantSNEPlain || !policy.AllowFallback || len(policy.RequiredCapabilities) != 1 || policy.RequiredCapabilities[0] != CapabilitySessions {
		t.Fatalf("rejected route changed canonical policy: %+v", policy)
	}
}

func TestSelectionControllerConcurrentRouteChangesPreservePolicy(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	sne, err := NewSNEConnector(selectionProvider{name: "sne", ready: true}, selectionIdentity(KindSNE), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}

	var workers sync.WaitGroup
	for index := 0; index < 64; index++ {
		kind, variant := KindMLX, VariantMLXRaw
		if index%2 != 0 {
			kind, variant = KindSNE, VariantSNEPlain
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			snapshot, err := controller.SelectRoute(kind, variant)
			if err != nil {
				t.Errorf("SelectRoute(%s/%s): %v", kind, variant, err)
				return
			}
			if snapshot.Preferred != kind || snapshot.PreferredVariant != variant || !snapshot.AllowFallback || len(snapshot.RequiredCapabilities) != 1 || snapshot.RequiredCapabilities[0] != CapabilitySessions {
				t.Errorf("SelectRoute(%s/%s) returned inconsistent snapshot: %+v", kind, variant, snapshot)
			}
		}()
	}
	workers.Wait()

	policy := controller.Policy()
	validRoute := policy.Preferred == KindMLX && policy.PreferredVariant == VariantMLXRaw || policy.Preferred == KindSNE && policy.PreferredVariant == VariantSNEPlain
	if !validRoute || !policy.AllowFallback || len(policy.RequiredCapabilities) != 1 || policy.RequiredCapabilities[0] != CapabilitySessions {
		t.Fatalf("concurrent route changes corrupted canonical policy: %+v", policy)
	}
}

func TestSelectionSnapshotLabelsMTPAsContextualNotStatic(t *testing.T) {
	identity := selectionIdentity(KindSNE)
	identity.Variant = VariantSNEMTP
	identity.Assistant = &AssistantIdentity{
		ModelID: "assistant-model", Revision: "r1",
		CheckpointSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Precision:        "int8",
	}
	backend := contextualSelectionProvider{
		selectionProvider: selectionProvider{name: "sne", ready: true},
		resolved:          provider.Caps{MTP: true},
	}
	connector, err := NewSNEConnector(backend, identity, Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindSNE, PreferredVariant: VariantSNEMTP,
		RequiredCapabilities: []Capability{CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if len(snapshot.Connectors) != 1 {
		t.Fatalf("connector snapshot = %+v", snapshot.Connectors)
	}
	summary := snapshot.Connectors[0]
	if summary.Capabilities.MTP {
		t.Fatal("live MTP readiness was misrepresented as a static capability")
	}
	if len(summary.ContextualCapabilities) != 1 || summary.ContextualCapabilities[0] != CapabilityMTP {
		t.Fatalf("contextual capabilities = %v, want [mtp]", summary.ContextualCapabilities)
	}
}

func TestSelectionControllerListsAndSelectsVariantsIndependently(t *testing.T) {
	raw := &recordingSelectionConnector{identity: selectionIdentity(KindMLX)}
	raw.identity.Variant = VariantMLXRaw
	patched := &recordingSelectionConnector{identity: selectionIdentity(KindMLX)}
	patched.identity.Variant = VariantMLXPatched
	router, err := NewRouter(raw, patched)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, RequiredCapabilities: []Capability{CapabilitySessions}})
	if err != nil {
		t.Fatal(err)
	}
	if got := controller.Snapshot().Connectors; len(got) != 2 || got[0].Variant != VariantMLXRaw || got[1].Variant != VariantMLXPatched {
		t.Fatalf("variant snapshot is not complete/deterministic: %+v", got)
	}
	if _, err := controller.Select(RoutePolicy{Preferred: KindMLX, PreferredVariant: VariantMLXPatched, RequiredCapabilities: []Capability{CapabilitySessions}}); err != nil {
		t.Fatal(err)
	}
	_, receipt, err := controller.CompletePrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Route == nil || receipt.Route.SelectedVariant != VariantMLXPatched || receipt.Identity.EffectiveVariant() != VariantMLXPatched {
		t.Fatalf("selected variant was not bound into receipt: %+v", receipt)
	}
}

func TestPromptSessionIDsRemainUniqueUnderConcurrency(t *testing.T) {
	const count = 256
	ids := make(chan string, count)
	var workers sync.WaitGroup
	workers.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer workers.Done()
			ids <- newPromptSessionID("prompt")
		}()
	}
	workers.Wait()
	close(ids)
	seen := make(map[string]struct{}, count)
	for id := range ids {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate generated session ID %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("generated %d unique session IDs, want %d", len(seen), count)
	}
}

func TestSelectionControllerRejectsUnsupportedPolicy(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSelectionController(router, RoutePolicy{Preferred: KindSNE}); err == nil {
		t.Fatal("expected unconfigured preferred connector rejection")
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, RequiredCapabilities: []Capability{CapabilityStreaming}})
	if err == nil || controller != nil {
		t.Fatal("expected unsupported preferred capability rejection")
	}
	if _, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilityStreaming}}); err == nil {
		t.Fatal("expected no-capable-connector rejection")
	}
}

func TestSelectionControllerAllowsCapablePreferredConnectorWithoutFallbackPeer(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{
		Sessions: true, Receipts: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilityReceipts},
	})
	if err != nil {
		t.Fatalf("capable preferred connector was rejected without a fallback peer: %v", err)
	}
	if snapshot := controller.Snapshot(); snapshot.Preferred != KindMLX || !snapshot.AllowFallback {
		t.Fatalf("preferred-only fallback policy = %+v", snapshot)
	}
}

func TestSelectionControllerRequiresCapableFallbackWhenPreferredLacksCapability(t *testing.T) {
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, selectionIdentity(KindMLX), Capabilities{Sessions: true})
	if err != nil {
		t.Fatal(err)
	}
	sne, err := NewSNEConnector(selectionProvider{name: "sne", ready: true}, selectionIdentity(KindSNE), Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilityReceipts},
	}); err != nil {
		t.Fatalf("capable fallback was rejected: %v", err)
	}
}

func TestSelectionControllerCompletesPromptThroughSelectedEngine(t *testing.T) {
	sne, err := NewSNEConnector(selectionProvider{name: "model", ready: true}, selectionIdentity(KindSNE), Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindSNE, RequiredCapabilities: []Capability{CapabilitySessions}})
	if err != nil {
		t.Fatal(err)
	}
	completion, receipt, err := controller.CompletePrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if completion.Model != "model" {
		t.Fatalf("completion model = %q, want model", completion.Model)
	}
	if receipt.Route == nil || receipt.Route.Selected != KindSNE || receipt.Route.Requested != KindSNE {
		t.Fatalf("prompt route = %+v, want selected SNE route", receipt.Route)
	}
}

func TestSelectionControllerCompletesPromptUsingCapturedPolicy(t *testing.T) {
	mlxIdentity := selectionIdentity(KindMLX)
	mlxIdentity.ModelID = "mlx"
	mlx, err := NewMLXConnector(selectionProvider{name: "mlx", ready: true}, mlxIdentity, Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	sneIdentity := selectionIdentity(KindSNE)
	sneIdentity.ModelID = "sne"
	sne, err := NewSNEConnector(selectionProvider{name: "sne", ready: true}, sneIdentity, Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(mlx, sne)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindSNE, RequiredCapabilities: []Capability{CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	acceptedPolicy := controller.Policy()
	if _, err := controller.SelectRoute(KindMLX, VariantMLXRaw); err != nil {
		t.Fatal(err)
	}
	_, receipt, err := controller.CompletePromptWithPolicy(context.Background(), PromptRequest{Prompt: "accepted before route change", MaxTokens: 8}, acceptedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Route == nil || receipt.Route.Requested != KindSNE || receipt.Route.Selected != KindSNE || receipt.Route.RequestedVariant != VariantSNEPlain {
		t.Fatalf("prompt did not honor captured SNE policy: %+v", receipt.Route)
	}
	if got := controller.Policy().Preferred; got != KindMLX {
		t.Fatalf("test did not retain concurrent route change, current policy=%q", got)
	}
}

func TestSelectionControllerSnapshotsSamplingInputsBeforeSessionAdmission(t *testing.T) {
	temperature, topP, seed := 0.4, 0.8, int64(17)
	connector := &recordingSelectionConnector{
		identity: selectionIdentity(KindSNE),
		capabilities: Capabilities{
			Sessions: true, Receipts: true, Temperature: true, TopP: true, Seed: true,
		},
		onOpen: func() {
			temperature, topP, seed = 1.9, 0.1, 99
		},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindSNE})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = controller.CompletePrompt(context.Background(), PromptRequest{
		Prompt: "hello", MaxTokens: 8, Temperature: &temperature, TopP: &topP, Seed: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(connector.requests) != 1 {
		t.Fatalf("connector received %d requests, want 1", len(connector.requests))
	}
	got := connector.requests[0]
	if got.Temperature == nil || *got.Temperature != 0.4 || got.TopP == nil || *got.TopP != 0.8 || got.Seed == nil || *got.Seed != 17 {
		t.Fatalf("session admission changed the admitted sampling inputs: %+v", got)
	}
}

func TestSelectionControllerChecksSamplingCapabilitiesBeforeOpeningSession(t *testing.T) {
	temperature := 0.5
	topP := 0.8
	seed := int64(5)
	cases := []struct {
		name       string
		request    PromptRequest
		capability Capability
	}{
		{name: "temperature", request: PromptRequest{Prompt: "hello", MaxTokens: 8, Temperature: &temperature}, capability: CapabilityTemperature},
		{name: "top_p", request: PromptRequest{Prompt: "hello", MaxTokens: 8, TopP: &topP}, capability: CapabilityTopP},
		{name: "seed", request: PromptRequest{Prompt: "hello", MaxTokens: 8, Seed: &seed}, capability: CapabilitySeed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector := &recordingSelectionConnector{
				identity:     selectionIdentity(KindSNE),
				capabilities: Capabilities{Sessions: true, Receipts: true},
			}
			router, err := NewRouter(connector)
			if err != nil {
				t.Fatal(err)
			}
			controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindSNE})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := controller.CompletePrompt(context.Background(), tc.request); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), string(tc.capability)) {
				t.Fatalf("unsupported sampling capability error = %v", err)
			}
			if connector.openCalls != 0 || len(connector.requests) != 0 {
				t.Fatalf("unsupported sampling request reached connector: opens=%d requests=%d", connector.openCalls, len(connector.requests))
			}
		})
	}
}

func TestSelectionControllerStreamsThroughSelectedEngineWithRouteBoundTerminal(t *testing.T) {
	identity := selectionIdentity(KindMLX)
	identity.Variant = VariantMLXPatched
	connector := &recordingSelectionConnector{
		identity:     identity,
		capabilities: Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindMLX, PreferredVariant: VariantMLXPatched,
		RequiredCapabilities: []Capability{CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := controller.StreamPrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if stream.Session.Identity.EffectiveVariant() != VariantMLXPatched || stream.Route.SelectedVariant != VariantMLXPatched {
		t.Fatalf("stream admission lost selected variant: session=%+v route=%+v", stream.Session.Identity, stream.Route)
	}
	event, ok := <-stream.Events
	if !ok || event.Kind != EventCompleted || event.Receipt == nil {
		t.Fatalf("stream terminal = %+v, open=%v", event, ok)
	}
	if event.Receipt.Route == nil || event.Receipt.Route.SelectedVariant != VariantMLXPatched {
		t.Fatalf("stream receipt lost selected route: %+v", event.Receipt.Route)
	}
	if len(connector.requests) != 1 || !connector.requests[0].Stream || !containsCapability(connector.requests[0].RequiredCapabilities, CapabilityStreaming) || !containsCapability(connector.requests[0].RequiredCapabilities, CapabilityCancellation) {
		t.Fatalf("stream request was not capability-bound: %+v", connector.requests)
	}
}

func TestSelectionControllerRejectsNonCancellableStreamBeforeOpeningSession(t *testing.T) {
	connector := &recordingSelectionConnector{
		identity:     selectionIdentity(KindMLX),
		capabilities: Capabilities{Sessions: true, Streaming: true, Receipts: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.StreamPrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8}); err == nil || !strings.Contains(err.Error(), "cancellation") {
		t.Fatalf("non-cancellable stream was admitted: %v", err)
	}
	if connector.openCalls != 0 || len(connector.requests) != 0 {
		t.Fatalf("non-cancellable stream reached connector: opens=%d requests=%d", connector.openCalls, len(connector.requests))
	}
}

func TestSelectionControllerRejectsMissingReceiptCapabilityBeforeOpeningSession(t *testing.T) {
	connector := &recordingSelectionConnector{
		identity:     selectionIdentity(KindMLX),
		capabilities: Capabilities{Sessions: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CompletePrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8}); err == nil || !strings.Contains(err.Error(), "receipts") {
		t.Fatalf("connector without receipt capability was admitted: %v", err)
	}
	if connector.openCalls != 0 || len(connector.requests) != 0 {
		t.Fatalf("non-receipting connector reached: opens=%d requests=%d", connector.openCalls, len(connector.requests))
	}
}

func TestSelectionControllerRejectsInvalidPromptBeforeOpeningSession(t *testing.T) {
	cases := map[string]func(*PromptRequest){
		"empty prompt":           func(request *PromptRequest) { request.Prompt = " \t" },
		"nonpositive max tokens": func(request *PromptRequest) { request.MaxTokens = 0 },
		"temperature NaN":        func(request *PromptRequest) { value := math.NaN(); request.Temperature = &value },
		"top_p infinity":         func(request *PromptRequest) { value := math.Inf(1); request.TopP = &value },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			for _, stream := range []bool{false, true} {
				connector := &recordingSelectionConnector{
					identity:     selectionIdentity(KindMLX),
					capabilities: Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true},
				}
				router, err := NewRouter(connector)
				if err != nil {
					t.Fatal(err)
				}
				controller, err := NewSelectionController(router, RoutePolicy{Preferred: KindMLX})
				if err != nil {
					t.Fatal(err)
				}
				request := PromptRequest{Prompt: "hello", MaxTokens: 8}
				mutate(&request)
				if stream {
					_, err = controller.StreamPrompt(context.Background(), request)
				} else {
					_, _, err = controller.CompletePrompt(context.Background(), request)
				}
				if err == nil {
					t.Fatal("invalid prompt was accepted")
				}
				if connector.openCalls != 0 {
					t.Fatalf("invalid prompt opened %d sessions (stream=%v)", connector.openCalls, stream)
				}
			}
		})
	}
}

type recordingSelectionConnector struct {
	identity     Identity
	requests     []GenerateRequest
	capabilities Capabilities
	openCalls    int
	onOpen       func()
}

func (c *recordingSelectionConnector) Kind() Kind              { return c.identity.Engine }
func (c *recordingSelectionConnector) Variant() BackendVariant { return c.identity.EffectiveVariant() }
func (c *recordingSelectionConnector) Capabilities() Capabilities {
	if c.capabilities != (Capabilities{}) {
		return c.capabilities
	}
	return Capabilities{Sessions: true, Receipts: true}
}
func (c *recordingSelectionConnector) OpenSession(_ context.Context, id string) (Session, error) {
	c.openCalls++
	if c.onOpen != nil {
		c.onOpen()
	}
	return Session{ID: id, Identity: c.identity, CreatedAt: "2026-09-08T12:00:00Z"}, nil
}
func (c *recordingSelectionConnector) Complete(_ context.Context, session Session, request GenerateRequest) (Completion, Receipt, error) {
	if err := request.Validate(session, c.Capabilities()); err != nil {
		return Completion{}, Receipt{}, err
	}
	c.requests = append(c.requests, request)
	requestDigest, err := generateRequestDigest(request)
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	identityDigest, err := session.Identity.Digest()
	if err != nil {
		return Completion{}, Receipt{}, err
	}
	completionSum := sha256.Sum256([]byte("answer"))
	return Completion{Text: "answer", Model: session.Identity.ModelID, FinishReason: "stop"}, Receipt{
		ABIVersion:       ABIVersion,
		SessionID:        session.ID,
		Identity:         session.Identity,
		IdentityDigest:   identityDigest,
		RequestSHA256:    requestDigest,
		CompletionSHA256: hex.EncodeToString(completionSum[:]),
		StartedAt:        "2026-09-08T12:00:00Z",
		FinishedAt:       "2026-09-08T12:00:01Z",
	}, nil
}
func (c *recordingSelectionConnector) Stream(_ context.Context, session Session, request GenerateRequest) (<-chan Event, error) {
	if err := request.Validate(session, c.Capabilities()); err != nil {
		return nil, err
	}
	c.requests = append(c.requests, request)
	started := time.Now().UTC()
	receipt, err := streamReceipt(session, request, "stream answer", started, started.Add(time.Millisecond), false)
	if err != nil {
		return nil, err
	}
	events := make(chan Event, 1)
	events <- Event{Kind: EventCompleted, SessionID: session.ID, Sequence: 1, Model: session.Identity.ModelID, Text: "stream answer", Receipt: &receipt}
	close(events)
	return events, nil
}

func containsCapability(capabilities []Capability, want Capability) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func TestSelectionControllerBindsPolicyCapabilitiesIntoPromptRequest(t *testing.T) {
	connector := &recordingSelectionConnector{
		identity:     selectionIdentity(KindMLX),
		capabilities: Capabilities{Sessions: true, Receipts: true, Temperature: true},
	}
	router, err := NewRouter(connector)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewSelectionController(router, RoutePolicy{
		Preferred: KindMLX, RequiredCapabilities: []Capability{CapabilitySessions},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, firstReceipt, err := controller.CompletePrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(connector.requests) != 1 || len(connector.requests[0].RequiredCapabilities) != 2 || !containsCapability(connector.requests[0].RequiredCapabilities, CapabilitySessions) || !containsCapability(connector.requests[0].RequiredCapabilities, CapabilityReceipts) {
		t.Fatalf("first request capabilities = %+v, want sessions+receipts", connector.requests)
	}

	if _, err := controller.Select(RoutePolicy{
		Preferred: KindMLX, RequiredCapabilities: []Capability{CapabilitySessions, CapabilityReceipts, CapabilityTemperature},
	}); err != nil {
		t.Fatal(err)
	}
	_, secondReceipt, err := controller.CompletePrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(connector.requests) != 2 || len(connector.requests[1].RequiredCapabilities) != 3 || !containsCapability(connector.requests[1].RequiredCapabilities, CapabilityReceipts) || !containsCapability(connector.requests[1].RequiredCapabilities, CapabilityTemperature) {
		t.Fatalf("second request capabilities = %+v, want sessions+receipts+temperature", connector.requests)
	}
	firstDigest, err := generateRequestDigest(connector.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := generateRequestDigest(connector.requests[1])
	if err != nil {
		t.Fatal(err)
	}
	if firstReceipt.RequestSHA256 != firstDigest || secondReceipt.RequestSHA256 != secondDigest {
		t.Fatalf("receipt request digest is not bound to the observed request: first=%q/%q second=%q/%q", firstReceipt.RequestSHA256, firstDigest, secondReceipt.RequestSHA256, secondDigest)
	}
	firstRequest := connector.requests[0]
	secondRequest := connector.requests[1]
	firstRequest.SessionID = "same-session"
	secondRequest.SessionID = "same-session"
	firstNormalizedDigest, err := generateRequestDigest(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondNormalizedDigest, err := generateRequestDigest(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if firstNormalizedDigest == secondNormalizedDigest {
		t.Fatalf("request digest did not change with policy capabilities: %q", firstNormalizedDigest)
	}
}

func generateRequestDigest(request GenerateRequest) (string, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	requestSum := sha256.Sum256(requestBytes)
	return hex.EncodeToString(requestSum[:]), nil
}
