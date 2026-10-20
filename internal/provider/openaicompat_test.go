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
	var requestBody ccRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
		case "/v1/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatal(err)
			}
			requestedModel = requestBody.Model
			_, _ = w.Write([]byte(`{"model":"served-model","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	temperature, topP, seed := 0.4, 0.85, int64(7)
	resp, err := p.Complete(context.Background(), Request{Prompt: "hello", Temperature: &temperature, TopP: &topP, Seed: &seed})
	if err != nil {
		t.Fatal(err)
	}
	if requestedModel != "served-model" {
		t.Fatalf("completion model = %q, want served-model", requestedModel)
	}
	if resp.Model != "served-model" || resp.Text != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if requestBody.Temperature == nil || *requestBody.Temperature != temperature || requestBody.TopP == nil || *requestBody.TopP != topP || requestBody.Seed == nil || *requestBody.Seed != seed {
		t.Fatalf("sampling controls were not forwarded: %+v", requestBody)
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

func TestCompleteCarriesStructuredToolCallAndToolResultConversation(t *testing.T) {
	var requests []ccRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"apollo-model"}]}`))
		case "/v1/chat/completions":
			var got ccRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			requests = append(requests, got)
			if len(requests) == 1 {
				_, _ = w.Write([]byte(`{"model":"apollo-model","choices":[{"finish_reason":"tool_calls","message":{"content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"machine.state","arguments":"{}"}}]}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"model":"apollo-model","choices":[{"finish_reason":"stop","message":{"content":"healthy"}}]}`))
		}
	}))
	defer srv.Close()
	p := &OpenAICompat{SupportsTools: true, ProviderName: "apollo", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	first, err := p.Complete(context.Background(), Request{Prompt: "status", Tools: []ToolSpec{{Name: "machine.state", Schema: map[string]any{"type": "object"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !first.ToolsHonored || len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call-1" {
		t.Fatalf("tool response = %+v", first)
	}
	toolResult := `{"summary":"healthy"}`
	final, err := p.Complete(context.Background(), Request{Messages: []Message{
		{Role: "user", Content: stringPointer("status")},
		{Role: "assistant", ToolCalls: first.ToolCalls},
		{Role: "tool", Content: &toolResult, ToolCallID: "call-1"},
	}})
	if err != nil || final.Text != "healthy" {
		t.Fatalf("final = %+v err=%v", final, err)
	}
	if len(requests) != 2 || requests[0].ToolChoice != "auto" || len(requests[1].Messages) != 3 || requests[1].Messages[2].ToolCallID != "call-1" {
		t.Fatalf("unexpected wire requests: %+v", requests)
	}
}

func stringPointer(s string) *string { return &s }

func TestStreamParsesSSEChunksAndDone(t *testing.T) {
	var requestBody ccRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"stream-model"}]}`))
		case "/v1/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"stream-model\",\"choices\":[{\"delta\":{\"content\":\"hel\"},\"finish_reason\":\"\"}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"model\":\"stream-model\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{SupportsStreaming: true, ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	temperature, topP, seed := 0.3, 0.9, int64(11)
	chunks, err := p.Stream(context.Background(), Request{Prompt: "hello", MaxTokens: 4, Temperature: &temperature, TopP: &topP, Seed: &seed})
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
	if requestBody.Temperature == nil || *requestBody.Temperature != temperature || requestBody.TopP == nil || *requestBody.TopP != topP || requestBody.Seed == nil || *requestBody.Seed != seed {
		t.Fatalf("stream sampling controls were not forwarded: %+v", requestBody)
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
	p := &OpenAICompat{SupportsStreaming: true, ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
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

func TestStreamRejectsEOFBeforeTerminalMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"stream-model"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"stream-model\",\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"\"}]}\n\n"))
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{SupportsStreaming: true, ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	chunks, err := p.Stream(context.Background(), Request{Prompt: "hello", MaxTokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	var terminalErr error
	for chunk := range chunks {
		if chunk.Err != nil {
			terminalErr = chunk.Err
		}
	}
	if terminalErr == nil {
		t.Fatal("truncated stream closed without an error")
	}
}
