package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

type fakeProvider struct {
	available bool
	response  provider.Response
}

type recordingProvider struct {
	fakeProvider
	request provider.Request
}

type blockingStreamingProvider struct {
	recordingProvider
	chunks chan provider.StreamChunk
}

func (p *blockingStreamingProvider) Stream(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
	return p.chunks, nil
}

func (p *recordingProvider) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	p.request = request
	return p.response, nil
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

func TestProviderConnectorPreservesSystemAndToolInputs(t *testing.T) {
	backend := &recordingProvider{fakeProvider: fakeProvider{
		available: true,
		response:  provider.Response{Text: "hello", Model: "model-a", FinishReason: "stop"},
	}}
	identity := testIdentity()
	connector, err := NewSNEConnector(backend, identity, Capabilities{Sessions: true, Tools: true, Temperature: true, TopP: true, Seed: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "s-tools")
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.7
	topP := 0.8
	seed := int64(42)
	req := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, System: "be concise", Prompt: "hello", MaxTokens: 4,
		CacheNamespace: session.Identity.CacheNamespace,
		Temperature:    &temperature, TopP: &topP, Seed: &seed,
		Tools: []ToolSpec{{Name: "inspect", Description: "inspect state", Schema: map[string]any{"type": "object"}}},
	}
	if _, _, err := connector.Complete(context.Background(), session, req); err != nil {
		t.Fatal(err)
	}
	if backend.request.System != "be concise" || backend.request.Prompt != "hello" || len(backend.request.Tools) != 1 || backend.request.Tools[0].Name != "inspect" {
		t.Fatalf("provider request lost ABI inputs: %+v", backend.request)
	}
	if backend.request.Temperature == nil || *backend.request.Temperature != temperature || backend.request.TopP == nil || *backend.request.TopP != topP || backend.request.Seed == nil || *backend.request.Seed != seed {
		t.Fatalf("provider request lost sampling controls: %+v", backend.request)
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

func TestProviderConnectorNormalizesLoopbackStreamAndReceipt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"model-a\",\"choices\":[{\"delta\":{\"content\":\"a\"},\"finish_reason\":\"\"}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"model\":\"model-a\",\"choices\":[{\"delta\":{\"content\":\"b\"},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	identity := testIdentity()
	identity.Engine = KindMLX
	backend := &provider.OpenAICompat{ProviderName: "mlx", Endpoint: srv.URL + "/v1", TierValue: provider.TierLocal, HTTP: srv.Client()}
	connector, err := NewMLXConnector(backend, identity, Capabilities{Sessions: true, Streaming: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "stream-session")
	if err != nil {
		t.Fatal(err)
	}
	events, err := connector.Stream(context.Background(), session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var receipt *Receipt
	var previous uint64
	for event := range events {
		if err := event.Validate(previous); err != nil {
			t.Fatal(err)
		}
		previous = event.Sequence
		if event.Kind == EventDelta {
			text += event.Text
		}
		if event.Kind == EventCompleted {
			receipt = event.Receipt
		}
	}
	if text != "ab" || receipt == nil {
		t.Fatalf("stream text=%q receipt=%v, want ab and receipt", text, receipt)
	}
	if err := receipt.Validate(session); err != nil {
		t.Fatalf("stream receipt invalid: %v", err)
	}
}

func TestProviderConnectorEmitsCancelledReceiptWhenStreamStalls(t *testing.T) {
	backend := &blockingStreamingProvider{
		recordingProvider: recordingProvider{fakeProvider: fakeProvider{
			available: true,
			response:  provider.Response{Model: "model-a"},
		}},
		chunks: make(chan provider.StreamChunk),
	}
	identity := testIdentity()
	connector, err := NewSNEConnector(backend, identity, Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "cancel-session")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := connector.Stream(ctx, session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	var got *Event
	for event := range events {
		copy := event
		got = &copy
	}
	if got == nil || got.Kind != EventError || got.ErrorCode != "cancelled" || got.Receipt == nil || !got.Receipt.Cancelled {
		t.Fatalf("cancellation event = %+v, want cancelled error with receipt", got)
	}
	if err := got.Receipt.Validate(session); err != nil {
		t.Fatalf("cancelled receipt invalid: %v", err)
	}
}

func TestProviderConnectorCancelsAfterPartialStream(t *testing.T) {
	chunks := make(chan provider.StreamChunk, 1)
	chunks <- provider.StreamChunk{Model: "model-a", Text: "partial"}
	backend := &blockingStreamingProvider{
		recordingProvider: recordingProvider{fakeProvider: fakeProvider{available: true, response: provider.Response{Model: "model-a"}}},
		chunks:            chunks,
	}
	identity := testIdentity()
	connector, err := NewSNEConnector(backend, identity, Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "cancel-after-partial")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := connector.Stream(ctx, session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err != nil {
		t.Fatal(err)
	}
	first := <-events
	if first.Kind != EventDelta || first.Text != "partial" {
		t.Fatalf("first event = %+v, want partial delta", first)
	}
	cancel()
	var got *Event
	for event := range events {
		copy := event
		got = &copy
	}
	if got == nil || got.ErrorCode != "cancelled" || got.Receipt == nil || !got.Receipt.Cancelled {
		t.Fatalf("post-partial cancellation = %+v, want cancelled receipt", got)
	}
}
