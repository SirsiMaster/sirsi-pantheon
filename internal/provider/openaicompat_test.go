package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
