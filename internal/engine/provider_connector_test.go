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
	"strings"
	"sync/atomic"
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

func TestProviderConnectorRejectsEOFBeforeTerminalMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			// Valid model-bound delta, then EOF: neither finish_reason nor [DONE].
			_, _ = w.Write([]byte("data: {\"model\":\"model-a\",\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	identity := testIdentity()
	identity.Engine = KindMLX
	backend := &provider.OpenAICompat{SupportsStreaming: true, ProviderName: "mlx", Endpoint: srv.URL + "/v1", Model: identity.ModelID, TierValue: provider.TierLocal, HTTP: srv.Client()}
	connector, err := NewMLXConnector(backend, identity, Capabilities{Sessions: true, Streaming: true, Receipts: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(ctx, "truncated-stream-session")
	if err != nil {
		t.Fatal(err)
	}
	events, err := connector.Stream(ctx, session, GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Stream: true, CacheNamespace: session.Identity.CacheNamespace})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var previous uint64
	var last EventKind
	var errorsSeen int
	for {
		select {
		case event, ok := <-events:
			if !ok {
				if text != "partial" || errorsSeen != 1 || last != EventError {
					t.Fatalf("truncated stream text=%q errors=%d last=%q, want partial and one terminal error", text, errorsSeen, last)
				}
				return
			}
			if err := event.Validate(previous); err != nil {
				t.Error(err)
			}
			previous, last = event.Sequence, event.Kind
			if event.SessionID != session.ID {
				t.Errorf("event session=%q, want %q", event.SessionID, session.ID)
			}
			if event.Receipt != nil || event.ReceiptSHA != "" {
				t.Error("truncated stream published a receipt")
			}
			switch event.Kind {
			case EventDelta:
				text += event.Text
			case EventCompleted:
				t.Error("truncated stream emitted completion")
			case EventError:
				errorsSeen++
				if event.ErrorCode != "stream_error" || !strings.Contains(event.Error, "stream ended before a terminal marker") {
					t.Errorf("unexpected terminal error: %+v", event)
				}
			}
		case <-ctx.Done():
			t.Fatalf("truncated stream did not close: %v", ctx.Err())
		}
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
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
			return
		}
		http.Error(w, "invalid request reached HTTP", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	identity := testIdentity()
	identity.Engine = KindMLX
	// A real streaming backend prevents unsupported-transport errors from
	// satisfying this validation regression without checking the numeric value.
	backend := &provider.OpenAICompat{SupportsStreaming: true, ProviderName: "mlx", Endpoint: srv.URL + "/v1", Model: identity.ModelID, TierValue: provider.TierLocal, HTTP: srv.Client()}
	connector, err := NewMLXConnector(backend, identity, Capabilities{Sessions: true, Streaming: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.OpenSession(ctx, "stream-session")
	if err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("session probe requests=%d, want 1", got)
	}
	for _, field := range []string{"temperature", "top_p"} {
		for _, value := range []struct {
			name   string
			number float64
		}{
			{name: "NaN", number: math.NaN()},
			{name: "+Inf", number: math.Inf(1)},
			{name: "-Inf", number: math.Inf(-1)},
		} {
			t.Run(field+"/"+value.name, func(t *testing.T) {
				req := GenerateRequest{SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 4, Stream: true, CacheNamespace: session.Identity.CacheNamespace}
				number := value.number
				if field == "temperature" {
					req.Temperature = &number
				} else {
					req.TopP = &number
				}
				events, err := connector.Stream(ctx, session, req)
				if err == nil || !strings.HasPrefix(err.Error(), "engine request: "+field+" ") {
					t.Errorf("stream error=%v, want %s request validation refusal", err, field)
				}
				if events != nil {
					t.Error("invalid request returned an event stream")
				}
				// Only OpenSession's model-list probe may reach the owned server.
				if got := requests.Load(); got != 1 {
					t.Errorf("HTTP requests=%d, want only the session probe", got)
				}
			})
		}
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
