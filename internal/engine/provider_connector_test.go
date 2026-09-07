package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type fakeProvider struct {
	available bool
	response  provider.Response
}

func (f fakeProvider) Name() string                   { return "fake" }
func (f fakeProvider) Tier() provider.Tier            { return provider.TierLocal }
func (f fakeProvider) Caps() provider.Caps            { return provider.Caps{Streaming: false, Offline: true} }
func (f fakeProvider) Available(context.Context) bool { return f.available }
func (f fakeProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return f.response, nil
}

func TestProviderConnectorBindsSessionAndReceipt(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 30, 0, 0, time.UTC)
	c := ProviderConnector{
		Backend: fakeProvider{available: true, response: provider.Response{Text: "hello", Model: "model-a", FinishReason: "stop", PromptTokens: 2, OutputTokens: 1}},
		Engine:  KindSNE,
		Model:   testIdentity(),
		Caps:    Capabilities{Sessions: true, Receipts: true},
		Now:     func() time.Time { return now },
	}
	session, err := c.OpenSession(context.Background(), "s-1")
	if err != nil {
		t.Fatal(err)
	}
	req := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, CacheNamespace: session.Identity.CacheNamespace}
	completion, receipt, err := c.Complete(context.Background(), session, req)
	if err != nil {
		t.Fatal(err)
	}
	if completion.Text != "hello" || completion.Model != "model-a" {
		t.Fatalf("completion = %+v", completion)
	}
	if err := receipt.Validate(session); err != nil {
		t.Fatalf("receipt does not validate: %v", err)
	}
}

func TestProviderConnectorRejectsServedModelDrift(t *testing.T) {
	c := ProviderConnector{
		Backend: fakeProvider{available: true, response: provider.Response{Text: "wrong", Model: "model-b"}},
		Engine:  KindMLX,
		Model:   func() Identity { i := testIdentity(); i.Engine = KindMLX; return i }(),
		Caps:    Capabilities{Sessions: true, Receipts: true},
	}
	session, err := c.OpenSession(context.Background(), "s-1")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Complete(context.Background(), session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, CacheNamespace: session.Identity.CacheNamespace})
	if err == nil {
		t.Fatal("served model drift was accepted")
	}
}

func TestNamedConnectorsAndStreamingFailureAreExplicit(t *testing.T) {
	identity := testIdentity()
	identity.Engine = KindMLX
	connector, err := NewMLXConnector(fakeProvider{available: true, response: provider.Response{Model: "model-a"}}, identity, Capabilities{Sessions: true, Streaming: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "s-stream")
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Stream(context.Background(), session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 2, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err == nil || !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("streaming result = %v, want explicit unsupported capability", err)
	}
	if _, err := NewSNEConnector(fakeProvider{}, func() Identity { i := testIdentity(); i.Engine = KindMLX; return i }(), Capabilities{}); err == nil {
		t.Fatal("SNE constructor accepted an MLX identity")
	}
}
