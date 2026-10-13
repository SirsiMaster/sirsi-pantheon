package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	backend := &provider.OpenAICompat{SupportsStreaming: true, ProviderName: "mlx", Endpoint: srv.URL + "/v1", TierValue: provider.TierLocal, HTTP: srv.Client()}
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

func TestProviderConnectorRejectsCompletionWithoutObservedModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"stop\"}]}\n\n"))
		}
	}))
	defer srv.Close()

	identity := testIdentity()
	identity.Engine = KindMLX
	backend := &provider.OpenAICompat{SupportsStreaming: true, ProviderName: "mlx", Endpoint: srv.URL + "/v1", Model: identity.ModelID, TierValue: provider.TierLocal, HTTP: srv.Client()}
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
	var sawUnobserved bool
	for event := range events {
		sawUnobserved = sawUnobserved || event.ErrorCode == "model_identity_unobserved"
		if event.Kind == EventCompleted {
			t.Fatal("model-less stream emitted completion")
		}
	}
	if !sawUnobserved {
		t.Fatal("model-less stream was accepted")
	}
}

func TestProviderConnectorRejectsNonFiniteStreamingRequest(t *testing.T) {
	identity := testIdentity()
	identity.Engine = KindMLX
	connector, err := NewMLXConnector(fakeProvider{available: true}, identity, Capabilities{Sessions: true, Streaming: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(context.Background(), "stream-session")
	if err != nil {
		t.Fatal(err)
	}
	nan := math.NaN()
	_, err = connector.Stream(context.Background(), session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Temperature: &nan, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err == nil {
		t.Fatal("non-finite streaming request was accepted")
	}
}

func TestProviderConnectorSelectedProfileActualAuthBudgetReceipt(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".sirsi"), 0700)
	capability := filepath.Join(home, "capability")
	os.WriteFile(capability, []byte("fixture-capability"), 0600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-capability" {
			t.Error("engine auth missing")
		}
		if r.Method == "GET" {
			fmt.Fprint(w, `{"data":[{"id":"model-a"}]}`)
			return
		}
		var p struct {
			Model     string
			MaxTokens int `json:"max_tokens"`
			Stream    bool
		}
		json.NewDecoder(r.Body).Decode(&p)
		if p.Model != "model-a" || p.MaxTokens > 256 || p.Stream {
			t.Error("engine profile mismatch")
		}
		fmt.Fprint(w, `{"model":"model-a","choices":[{"finish_reason":"stop","message":{"content":"draft"}}]}`)
	}))
	defer srv.Close()
	no := false
	b, _ := json.Marshal(provider.LocalProfile{Endpoint: srv.URL + "/v1", Model: "model-a", ContextTokens: 1024, OutputTokens: 256, Streaming: &no, AccessTokenFile: capability})
	profile := filepath.Join(home, "profile.json")
	os.WriteFile(profile, b, 0600)
	os.WriteFile(filepath.Join(home, ".sirsi", "orchestrator.conf"), []byte("provider=sne-native-v2\nlocal_profile="+profile), 0600)
	backend := provider.Local(home, provider.LoadConf(home))
	c, e := NewSNEConnector(backend, testIdentity(), Capabilities{Sessions: true, Receipts: true})
	if e != nil {
		t.Fatal(e)
	}
	session, e := c.OpenSession(context.Background(), "selected-profile-session")
	if e != nil {
		t.Fatal(e)
	}
	_, receipt, e := c.Complete(context.Background(), session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "input", MaxTokens: 400, CacheNamespace: session.Identity.CacheNamespace})
	if e != nil {
		t.Fatal(e)
	}
	if e = receipt.Validate(session); e != nil {
		t.Fatal(e)
	}
}
