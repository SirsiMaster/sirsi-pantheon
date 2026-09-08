package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type selectionProvider struct {
	name  string
	caps  provider.Caps
	ready bool
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
	if _, err := controller.Select(RoutePolicy{Preferred: KindSNE, AllowFallback: false, RequiredCapabilities: []Capability{CapabilitySessions}}); err != nil {
		t.Fatal(err)
	}
	if got := controller.Policy().Preferred; got != KindSNE {
		t.Fatalf("preferred engine = %q, want sne", got)
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

type recordingSelectionConnector struct {
	identity Identity
	requests []GenerateRequest
}

func (c *recordingSelectionConnector) Kind() Kind { return c.identity.Engine }
func (c *recordingSelectionConnector) Capabilities() Capabilities {
	return Capabilities{Sessions: true, Receipts: true}
}
func (c *recordingSelectionConnector) OpenSession(_ context.Context, id string) (Session, error) {
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
func (c *recordingSelectionConnector) Stream(context.Context, Session, GenerateRequest) (<-chan Event, error) {
	return nil, ErrUnsupportedCapability
}

func TestSelectionControllerBindsPolicyCapabilitiesIntoPromptRequest(t *testing.T) {
	connector := &recordingSelectionConnector{identity: selectionIdentity(KindMLX)}
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
	if len(connector.requests) != 1 || len(connector.requests[0].RequiredCapabilities) != 1 || connector.requests[0].RequiredCapabilities[0] != CapabilitySessions {
		t.Fatalf("first request capabilities = %+v, want sessions", connector.requests)
	}

	if _, err := controller.Select(RoutePolicy{
		Preferred: KindMLX, RequiredCapabilities: []Capability{CapabilitySessions, CapabilityReceipts},
	}); err != nil {
		t.Fatal(err)
	}
	_, secondReceipt, err := controller.CompletePrompt(context.Background(), PromptRequest{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(connector.requests) != 2 || len(connector.requests[1].RequiredCapabilities) != 2 || connector.requests[1].RequiredCapabilities[1] != CapabilityReceipts {
		t.Fatalf("second request capabilities = %+v, want sessions+receipts", connector.requests)
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
