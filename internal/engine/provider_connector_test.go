package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

type cancelingProvider struct {
	fakeProvider
	cancel context.CancelFunc
}

type blockingStreamingProvider struct {
	recordingProvider
	chunks chan provider.StreamChunk
}

type mutatingProvider struct {
	fakeProvider
}

type readinessFailingProvider struct {
	fakeProvider
	err   error
	calls int
}

type contextualCapabilityProvider struct {
	fakeProvider
	resolved provider.Caps
	calls    int
}

func (p *contextualCapabilityProvider) CapabilitiesForContext(context.Context) (provider.Caps, error) {
	p.calls++
	return p.resolved, nil
}

func (p *readinessFailingProvider) Readiness(context.Context) error {
	p.calls++
	return p.err
}

func (p *blockingStreamingProvider) Stream(context.Context, provider.Request) (<-chan provider.StreamChunk, error) {
	return p.chunks, nil
}

func (p *recordingProvider) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	p.request = request
	return p.response, nil
}

func (p cancelingProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	p.cancel()
	return p.response, nil
}

func (p mutatingProvider) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	if request.Temperature != nil {
		*request.Temperature = 1
	}
	if request.TopP != nil {
		*request.TopP = 0
	}
	if request.Seed != nil {
		*request.Seed = -1
	}
	if len(request.Tools) != 0 {
		request.Tools[0].Schema["nested"].(map[string]any)["value"] = "backend mutation"
	}
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

func TestProviderConnectorDerivesOpenAIEndpointBoundaryFromEndpoint(t *testing.T) {
	for _, test := range []struct {
		name     string
		endpoint string
		tier     provider.Tier
		override string
		want     string
	}{
		{name: "public endpoint defeats local tier and override", endpoint: "https://api.example.test/v1", tier: provider.TierLocal, override: DataBoundaryOnDevice, want: DataBoundaryRemote},
		{name: "loopback endpoint defeats remote tier and override", endpoint: "http://127.0.0.1:8765/v1", tier: provider.TierRemote, override: DataBoundaryRemote, want: DataBoundaryOnDevice},
	} {
		t.Run(test.name, func(t *testing.T) {
			connector := ProviderConnector{
				Backend: &provider.OpenAICompat{Endpoint: test.endpoint, TierValue: test.tier},
				Engine:  KindMLX, Model: testIdentity(), RequestBoundary: test.override,
			}
			if got := connector.RequestDataBoundary(); got != test.want {
				t.Fatalf("RequestDataBoundary() = %q, want endpoint-derived %q", got, test.want)
			}
		})
	}
}

func TestProviderConnectorDoesNotInferEndpointBoundaryFromComputeTier(t *testing.T) {
	connector, err := NewSNEConnector(fakeProvider{available: true}, testIdentity(), Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := connector.RequestDataBoundary(); got != DataBoundaryNotDisclosed {
		t.Fatalf("SNE compute tier was mistaken for endpoint evidence: %q", got)
	}
	connector.RequestBoundary = "remote"
	if got := connector.RequestDataBoundary(); got != DataBoundaryRemote {
		t.Fatalf("explicit SNE endpoint classification = %q, want remote", got)
	}
}

func TestProviderConnectorPreservesContractReadinessFailure(t *testing.T) {
	backend := &readinessFailingProvider{fakeProvider: fakeProvider{available: true}, err: errors.New("readiness/status API contract mismatch")}
	connector, err := NewProviderConnector(backend, KindSNE, testIdentity(), Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.OpenSession(context.Background(), "readiness-failure"); err == nil || !strings.Contains(err.Error(), "readiness/status API contract mismatch") {
		t.Fatalf("readiness failure was hidden: %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("error-preserving readiness was called %d times, want 1", backend.calls)
	}
}

func TestProviderConnectorAdmitsMTPOnlyThroughContextualCapabilityProof(t *testing.T) {
	identity := testIdentity()
	identity.Variant = VariantSNEMTP
	identity.Assistant = &AssistantIdentity{
		ModelID: "assistant-a", Revision: "r1", CheckpointSHA256: testSHA, Precision: "int8",
	}
	backend := &contextualCapabilityProvider{
		fakeProvider: fakeProvider{available: true},
		resolved:     provider.Caps{Offline: true},
	}
	connector, err := NewSNEConnector(backend, identity, Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.OpenSession(context.Background(), "mtp-without-proof"); err == nil || !strings.Contains(err.Error(), "mtp") {
		t.Fatalf("MTP opened without readiness proof: %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("contextual capability resolver called %d times, want 1", backend.calls)
	}

	backend.resolved.MTP = true
	backend.fakeProvider.response = provider.Response{Text: "response", Model: identity.ModelID, FinishReason: "stop"}
	session, err := connector.OpenSession(context.Background(), "mtp-with-proof")
	if err != nil {
		t.Fatalf("MTP readiness proof did not admit session: %v", err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	_, receipt, err := connector.Complete(context.Background(), session, request)
	if err != nil {
		t.Fatalf("MTP completion did not use contextual capability proof: %v", err)
	}
	wantDigest, err := session.Identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Identity.Assistant == nil || *receipt.Identity.Assistant != *identity.Assistant || receipt.IdentityDigest != wantDigest {
		t.Fatalf("MTP receipt did not retain assistant identity: identity=%+v digest=%q want=%q", receipt.Identity, receipt.IdentityDigest, wantDigest)
	}
	if backend.calls != 3 {
		t.Fatalf("contextual capability resolver called %d times, want 3", backend.calls)
	}
}

func TestProviderConnectorCompleteRejectsStreamMarkedRequestBeforeTransport(t *testing.T) {
	backend := &recordingProvider{fakeProvider: fakeProvider{
		available: true, response: provider.Response{Text: "buffered", Model: "model-a"},
	}}
	connector, err := NewSNEConnector(backend, testIdentity(), Capabilities{
		Sessions: true, Receipts: true, Streaming: true, Cancellation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "stream-to-complete")
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 2,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, _, err := connector.Complete(context.Background(), session, request); err == nil || !strings.Contains(err.Error(), "stream=false") {
		t.Fatalf("connector buffered a stream-marked request: %v", err)
	}
	if backend.request.Prompt != "" {
		t.Fatalf("stream-marked request reached completion transport: %+v", backend.request)
	}
}

func TestProviderConnectorRejectsNilContextAtEveryExecutionBoundary(t *testing.T) {
	connector, err := NewSNEConnector(fakeProvider{available: true}, testIdentity(), Capabilities{
		Sessions: true, Receipts: true, Streaming: true, Cancellation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.OpenSession(nil, "nil-context"); err == nil || err.Error() != "engine connector: context is required" {
		t.Fatalf("OpenSession(nil) error = %v, want explicit context error", err)
	}
	session := testSession()
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, _, err := connector.Complete(nil, session, request); err == nil || err.Error() != "engine connector: context is required" {
		t.Fatalf("Complete(nil) error = %v, want explicit context error", err)
	}
	if _, err := connector.Stream(nil, session, request); err == nil || err.Error() != "engine connector: context is required" {
		t.Fatalf("Stream(nil) error = %v, want explicit context error", err)
	}
}

func TestProviderConnectorDoesNotPublishSuccessAfterProviderIgnoresCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backend := cancelingProvider{
		fakeProvider: fakeProvider{available: true, response: provider.Response{Text: "late", Model: "model-a"}},
		cancel:       cancel,
	}
	connector, err := NewSNEConnector(backend, testIdentity(), Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session := testSession()
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	completion, receipt, err := connector.Complete(ctx, session, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete error = %v, want context.Canceled", err)
	}
	if completion.Text != "" || receipt.RequestSHA256 != "" {
		t.Fatalf("cancelled provider result was published: completion=%+v receipt=%+v", completion, receipt)
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

func TestProviderConnectorRejectsMissingServedModel(t *testing.T) {
	connector, err := NewSNEConnector(fakeProvider{available: true, response: provider.Response{Text: "unidentified"}}, testIdentity(), Capabilities{Sessions: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session := testSession()
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	completion, receipt, err := connector.Complete(context.Background(), session, request)
	if err == nil || !strings.Contains(err.Error(), `served model "" does not match admitted model`) {
		t.Fatalf("provider completion without served model identity = %+v, %+v, %v", completion, receipt, err)
	}
	if completion.Text != "" || receipt.RequestSHA256 != "" {
		t.Fatalf("unidentified completion was published: completion=%+v receipt=%+v", completion, receipt)
	}
}

func TestProviderConnectorRejectsCompletedStreamWithoutServedModel(t *testing.T) {
	chunks := make(chan provider.StreamChunk, 1)
	chunks <- provider.StreamChunk{Done: true}
	close(chunks)
	backend := &blockingStreamingProvider{
		recordingProvider: recordingProvider{fakeProvider: fakeProvider{available: true}},
		chunks:            chunks,
	}
	connector, err := NewSNEConnector(backend, testIdentity(), Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true})
	if err != nil {
		t.Fatal(err)
	}
	session := testSession()
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	}
	events, err := connector.Stream(context.Background(), session, request)
	if err != nil {
		t.Fatal(err)
	}
	var terminal Event
	for event := range events {
		terminal = event
	}
	if terminal.Kind != EventError || terminal.ErrorCode != "model_identity_missing" {
		t.Fatalf("stream terminal = %+v, want missing-model error", terminal)
	}
}

func TestProviderConnectorEmitsTerminalErrorWhenProviderStreamClosesEarly(t *testing.T) {
	chunks := make(chan provider.StreamChunk)
	close(chunks)
	backend := &blockingStreamingProvider{
		recordingProvider: recordingProvider{fakeProvider: fakeProvider{available: true}},
		chunks:            chunks,
	}
	connector, err := NewSNEConnector(backend, testIdentity(), Capabilities{Sessions: true, Receipts: true, Streaming: true, Cancellation: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "truncated-stream")
	if err != nil {
		t.Fatal(err)
	}
	events, err := connector.Stream(context.Background(), session, GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 1,
		Stream: true, CacheNamespace: session.Identity.CacheNamespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	var terminal Event
	count := 0
	for event := range events {
		terminal = event
		count++
	}
	if count != 1 || terminal.Kind != EventError || terminal.ErrorCode != "stream_incomplete" || !strings.Contains(terminal.Error, "closed before completion") {
		t.Fatalf("truncated provider stream events = count %d, terminal %+v; want explicit terminal error", count, terminal)
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

func TestProviderConnectorDetachesMutableRequestInputs(t *testing.T) {
	backend := mutatingProvider{fakeProvider: fakeProvider{
		available: true,
		response:  provider.Response{Text: "hello", Model: "model-a", FinishReason: "stop"},
	}}
	identity := testIdentity()
	connector, err := NewSNEConnector(backend, identity, Capabilities{
		Sessions: true, Tools: true, Temperature: true, TopP: true, Seed: true, Receipts: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "s-detached")
	if err != nil {
		t.Fatal(err)
	}
	temperature, topP, seed := 0.4, 0.8, int64(17)
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 8,
		CacheNamespace: session.Identity.CacheNamespace,
		Temperature:    &temperature, TopP: &topP, Seed: &seed,
		Tools: []ToolSpec{{Name: "inspect", Schema: map[string]any{
			"nested": map[string]any{"value": "original"},
		}}},
	}
	wantDigest, err := GenerateRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := connector.Complete(context.Background(), session, request)
	if err != nil {
		t.Fatal(err)
	}
	if temperature != 0.4 || topP != 0.8 || seed != 17 {
		t.Fatalf("backend mutated caller-owned sampling values: temperature=%v top_p=%v seed=%v", temperature, topP, seed)
	}
	if got := request.Tools[0].Schema["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("backend mutated caller-owned tool schema: %v", got)
	}
	if receipt.RequestSHA256 != wantDigest {
		t.Fatalf("receipt request hash = %q, want %q", receipt.RequestSHA256, wantDigest)
	}
}

func TestNamedConnectorsAndStreamingFailureAreExplicit(t *testing.T) {
	identity := testIdentity()
	identity.Engine = KindMLX
	connector, err := NewMLXConnector(fakeProvider{available: true, response: provider.Response{Model: "model-a"}}, identity, Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true})
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

func TestProviderConnectorRejectsUnmarkedStreamBeforeTransport(t *testing.T) {
	identity := testIdentity()
	identity.Engine = KindMLX
	backend := &blockingStreamingProvider{
		recordingProvider: recordingProvider{fakeProvider: fakeProvider{available: true}},
		chunks:            make(chan provider.StreamChunk),
	}
	connector, err := NewMLXConnector(backend, identity, Capabilities{
		Sessions: true, Receipts: true, Streaming: true, Cancellation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "unmarked-stream")
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 2,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	if _, err := connector.Stream(context.Background(), session, request); err == nil || !strings.Contains(err.Error(), "stream=true") {
		t.Fatalf("connector accepted an unmarked stream request: %v", err)
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
	connector, err := NewMLXConnector(backend, identity, Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true})
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
	var terminal Event
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
			terminal = event
		}
	}
	if text != "ab" || receipt == nil {
		t.Fatalf("stream text=%q receipt=%v, want ab and receipt", text, receipt)
	}
	if terminal.Model != "model-a" || terminal.Finish != "stop" {
		t.Fatalf("stream terminal model=%q finish=%q, want model-a and stop", terminal.Model, terminal.Finish)
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

func TestProviderConnectorCancellationReplacesBufferedDeltaWithTerminalReceipt(t *testing.T) {
	chunks := make(chan provider.StreamChunk, 1)
	chunks <- provider.StreamChunk{Model: "model-a", Text: "queued"}
	backend := &blockingStreamingProvider{
		recordingProvider: recordingProvider{fakeProvider: fakeProvider{available: true, response: provider.Response{Model: "model-a"}}},
		chunks:            chunks,
	}
	identity := testIdentity()
	connector, err := NewSNEConnector(backend, identity, Capabilities{Sessions: true, Streaming: true, Cancellation: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "cancel-buffered-delta")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := connector.Stream(ctx, session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for len(events) == 0 {
		select {
		case <-deadline:
			t.Fatal("provider did not buffer its initial delta")
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
	if len(received) == 0 {
		t.Fatal("cancellation closed the stream without events")
	}
	terminal := received[len(received)-1]
	if terminal.Kind != EventError || terminal.ErrorCode != "cancelled" || terminal.Receipt == nil || !terminal.Receipt.Cancelled {
		t.Fatalf("events after cancellation with a full output buffer = %+v, want terminal cancellation receipt", received)
	}
	if terminal.Receipt.CompletionSHA256 != completionDigest(delivered.String()) {
		t.Fatalf("cancellation receipt hash does not match retained deltas %q: %+v", delivered.String(), terminal.Receipt)
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
