package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOpenAICompatDefaultClientDoesNotFollowRedirects(t *testing.T) {
	var destinationHit atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationHit.Store(true)
		_, _ = w.Write([]byte(`{"data":[{"id":"unexpected"}]}`))
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer origin.Close()

	provider := &OpenAICompat{ProviderName: "test", Endpoint: origin.URL, Model: "admitted-model"}
	if _, err := provider.ProbeServedModel(context.Background()); err == nil || !strings.Contains(err.Error(), "http 302") {
		t.Fatalf("redirect response error = %v, want configured endpoint HTTP 302", err)
	}
	if destinationHit.Load() {
		t.Fatal("OpenAI-compatible provider followed redirect to a different authority")
	}
}

func TestOpenAICompatInjectedClientDoesNotFollowRedirects(t *testing.T) {
	var destinationHit atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationHit.Store(true)
		_, _ = w.Write([]byte(`{"data":[{"id":"unexpected"}]}`))
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	provider := &OpenAICompat{
		ProviderName: "test",
		Endpoint:     origin.URL,
		Model:        "admitted-model",
		HTTP:         origin.Client(),
	}
	if _, err := provider.ProbeServedModel(context.Background()); err == nil || !strings.Contains(err.Error(), "http 307") {
		t.Fatalf("injected-client redirect error = %v, want configured endpoint HTTP 307", err)
	}
	if destinationHit.Load() {
		t.Fatal("OpenAI-compatible provider followed an injected client's redirect to a different authority")
	}
}

func TestOpenAICompatInjectedClientDoesNotReplayPromptOnPostRedirect(t *testing.T) {
	var destinationHit atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationHit.Store(true)
		_, _ = w.Write([]byte(`{"model":"admitted-model","choices":[{"finish_reason":"stop","message":{"content":"unexpected"}}]}`))
	}))
	defer destination.Close()
	var originHit atomic.Bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHit.Store(true)
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	provider := &OpenAICompat{
		ProviderName: "test",
		Endpoint:     origin.URL + "/v1",
		Model:        "admitted-model",
		HTTP:         origin.Client(),
	}
	if _, err := provider.Complete(context.Background(), Request{Prompt: "private prompt text"}); err == nil || !strings.Contains(err.Error(), "http 307") {
		t.Fatalf("injected-client completion redirect error = %v, want configured endpoint HTTP 307", err)
	}
	if !originHit.Load() {
		t.Fatal("configured completion endpoint did not receive the request")
	}
	if destinationHit.Load() {
		t.Fatal("OpenAI-compatible provider replayed a prompt to a redirected authority")
	}
}

func TestOpenAICompatCapabilitiesDeclareContextCancellation(t *testing.T) {
	caps := (&OpenAICompat{}).Caps()
	if !caps.Cancellation {
		t.Fatal("context-aware HTTP provider did not declare cancellation support")
	}
	if caps.MTP || caps.Prefill || caps.Decode || caps.KVState || caps.Telemetry {
		t.Fatalf("OpenAI-compatible provider claimed undeclared engine capabilities: %+v", caps)
	}
}

func TestOpenAICompatRejectsNilContextsWithoutTransport(t *testing.T) {
	transportCalls := 0
	p := &OpenAICompat{
		ProviderName: "test",
		Endpoint:     "http://127.0.0.1:1/v1",
		Model:        "model",
		HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			transportCalls++
			return nil, errors.New("unexpected transport call")
		})},
	}
	if p.Available(nil) {
		t.Fatal("provider reported availability with a nil context")
	}
	if _, err := p.ServedModel(nil); err == nil {
		t.Fatal("ServedModel accepted a nil context")
	}
	if _, err := p.ProbeServedModel(nil); err == nil {
		t.Fatal("ProbeServedModel accepted a nil context")
	}
	if _, err := p.Complete(nil, Request{Prompt: "hello"}); err == nil {
		t.Fatal("Complete accepted a nil context")
	}
	if _, err := p.Stream(nil, Request{Prompt: "hello"}); err == nil {
		t.Fatal("Stream accepted a nil context")
	}
	if transportCalls != 0 {
		t.Fatalf("nil-context calls reached HTTP transport %d times", transportCalls)
	}
}

func TestDecodeOpenAIJSONBoundsResponseAndRejectsTrailingDocuments(t *testing.T) {
	var decoded modelsResponse
	if err := decodeOpenAIJSON(strings.NewReader(`{"data":[]} {"data":[]}`), &decoded); err == nil {
		t.Fatal("decoder accepted a second JSON document")
	}
	oversized := `{"data":[]}` + strings.Repeat(" ", maxOpenAIJSONResponseBytes)
	if err := decodeOpenAIJSON(strings.NewReader(oversized), &decoded); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v, want byte-limit rejection", err)
	}
}

func TestCompleteResolvesServedModel(t *testing.T) {
	var requestedModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
		case "/v1/chat/completions":
			var req ccRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			requestedModel = req.Model
			_, _ = w.Write([]byte(`{"model":"served-model","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	resp, err := p.Complete(context.Background(), Request{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if requestedModel != "served-model" {
		t.Fatalf("completion model = %q, want served-model", requestedModel)
	}
	if resp.Model != "served-model" || resp.Text != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCompletePreservesSamplingControlsOnWire(t *testing.T) {
	var received ccRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"model":"admitted-model","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	temperature := 0.7
	topP := 0.8
	seed := int64(42)
	p := &OpenAICompat{ProviderName: "remote", Endpoint: srv.URL + "/v1", Model: "admitted-model", TierValue: TierRemote, HTTP: srv.Client()}
	if _, err := p.Complete(context.Background(), Request{Prompt: "hello", MaxTokens: 4, Temperature: &temperature, TopP: &topP, Seed: &seed}); err != nil {
		t.Fatal(err)
	}
	if received.Temperature == nil || *received.Temperature != temperature || received.TopP == nil || *received.TopP != topP || received.Seed == nil || *received.Seed != seed {
		t.Fatalf("wire request lost sampling controls: %+v", received)
	}
}

func TestCompleteFailsWhenBrokerServesNoModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL, TierValue: TierLocal, HTTP: srv.Client()}
	if _, err := p.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
		t.Fatal("expected missing served model to fail")
	}
}

func TestCompleteRejectsResponseModelDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"model":"different-model","choices":[{"finish_reason":"stop","message":{"content":"wrong"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "mlx", Endpoint: srv.URL + "/v1", Model: "admitted-model", TierValue: TierLocal, HTTP: srv.Client()}
	if _, err := p.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
		t.Fatal("expected completion model drift to fail closed")
	}
}

func TestAvailableRejectsConfiguredModelDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"different-model"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "omlx", Endpoint: srv.URL + "/v1", Model: "admitted-model", TierValue: TierLocal, HTTP: srv.Client()}
	if p.Available(context.Background()) {
		t.Fatal("expected availability to reject configured model drift")
	}
}

func TestStreamParsesSSEChunksAndDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"stream-model"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"stream-model\",\"choices\":[{\"delta\":{\"content\":\"hel\"},\"finish_reason\":\"\"}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"model\":\"stream-model\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	chunks, err := p.Stream(context.Background(), Request{Prompt: "hello", MaxTokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var sawDone bool
	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		text += chunk.Text
		sawDone = sawDone || chunk.Done
	}
	if text != "hello" || !sawDone {
		t.Fatalf("stream = %q done=%v, want hello/done", text, sawDone)
	}
}

func TestStreamRejectsResponseModelDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"different-model\",\"choices\":[{\"delta\":{\"content\":\"wrong\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "mlx", Endpoint: srv.URL + "/v1", Model: "admitted-model", TierValue: TierLocal, HTTP: srv.Client()}
	chunks, err := p.Stream(context.Background(), Request{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	var sawDrift bool
	for chunk := range chunks {
		if chunk.Err != nil {
			sawDrift = true
		}
	}
	if !sawDrift {
		t.Fatal("expected stream model drift to fail closed")
	}
}

func TestStreamReportsProviderErrorEventsInsteadOfCompleting(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{name: "provider error", data: `{"error":{"message":"backend overloaded"}}`, want: "backend overloaded"},
		{name: "malformed provider error", data: `{"error":{"type":"server_error"}}`, want: "invalid stream error event"},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", test.data)
			}))
			defer srv.Close()

			p := &OpenAICompat{ProviderName: "mlx", Endpoint: srv.URL + "/v1", Model: "admitted-model", TierValue: TierLocal, HTTP: srv.Client()}
			chunks, err := p.Stream(context.Background(), Request{Prompt: "hello"})
			if err != nil {
				t.Fatal(err)
			}
			sawError := false
			for chunk := range chunks {
				if chunk.Err != nil {
					sawError = true
					if !strings.Contains(chunk.Err.Error(), test.want) {
						t.Fatalf("stream error = %v, want it to contain %q", chunk.Err, test.want)
					}
				}
				if chunk.Done {
					t.Fatal("provider error event was followed by successful completion")
				}
			}
			if !sawError {
				t.Fatal("provider error event was silently ignored")
			}
		})
	}
}

func TestStreamCancellationClosesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"cancel-model"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"cancel-model\",\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"\"}]}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	chunks, err := p.Stream(ctx, Request{Prompt: "hello", MaxTokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-chunks:
		if ok {
			for range chunks {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close after cancellation")
	}
}
