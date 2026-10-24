package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAvailableRealCompletionProbeValidatesTerminalEnvelope(t *testing.T) {
	validStop := `{"model":"served-model","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`
	validLength := `{"model":"served-model","choices":[{"finish_reason":"length","message":{"content":"x"}}]}`
	validReasoningOnly := `{"model":"served-model","choices":[{"finish_reason":"length","message":{"content":"","reasoning":"thinking"}}]}`
	oversized := validStop + strings.Repeat(" ", readinessCompletionResponseLimit)

	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "stop completion", body: validStop, want: true},
		{name: "one-token length completion", body: validLength, want: true},
		{name: "reasoning-only length completion", body: validReasoningOnly, want: true},
		{name: "malformed JSON", body: `{"choices":[}`, want: false},
		{name: "error envelope", body: `{"error":{"message":"failed"},"choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`, want: false},
		{name: "no choices", body: `{"choices":[]}`, want: false},
		{name: "trailing JSON", body: validStop + `{}`, want: false},
		{name: "oversized response", body: oversized, want: false},
		{name: "missing terminal reason", body: `{"choices":[{"message":{"content":"ok"}}]}`, want: false},
		{name: "terminal response without generated content", body: `{"choices":[{"finish_reason":"length","message":{"content":""}}]}`, want: false},
		{name: "unsupported terminal reason", body: `{"choices":[{"finish_reason":"pending","message":{"content":"ok"}}]}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var modelRequests, completionRequests int
			var completionRequest ccRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
					modelRequests++
					_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
				case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
					completionRequests++
					if err := json.NewDecoder(r.Body).Decode(&completionRequest); err != nil {
						t.Errorf("decode readiness request: %v", err)
					}
					_, _ = w.Write([]byte(tt.body))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			p := &OpenAICompat{
				ProviderName:           "local",
				Endpoint:               srv.URL + "/v1",
				Model:                  "configured-model",
				TierValue:              TierLocal,
				HTTP:                   srv.Client(),
				UseRealCompletionProbe: true,
			}
			if got := p.Available(context.Background()); got != tt.want {
				t.Fatalf("Available() = %v, want %v", got, tt.want)
			}
			mu.Lock()
			defer mu.Unlock()
			if modelRequests != 1 || completionRequests != 1 {
				t.Fatalf("requests = models:%d completions:%d, want one each", modelRequests, completionRequests)
			}
			if completionRequest.Model != "served-model" || completionRequest.MaxTokens != 1 {
				t.Fatalf("readiness request = model:%q max_tokens:%d, want served-model/1", completionRequest.Model, completionRequest.MaxTokens)
			}
		})
	}
}

func TestAvailableRealCompletionProbeRejectsCompletionTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("HTTP fixture does not support connection hijacking")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack completion connection: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client(), UseRealCompletionProbe: true}
	if p.Available(context.Background()) {
		t.Fatal("completion transport failure was reported as available")
	}
}

func TestAvailableRealCompletionProbeRejectsCompletionCancellation(t *testing.T) {
	postReached := make(chan struct{})
	completionStarted := make(chan struct{})
	cancellationObserved := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerDone := make(chan struct{})
	bodyReadError := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" {
			defer close(handlerDone)
			close(postReached)
			if r.ContentLength < 0 || r.ContentLength > 64<<10 {
				bodyReadError <- errors.New("completion request body exceeds the fixture bound")
				return
			}
			if _, err := io.CopyN(io.Discard, r.Body, r.ContentLength); err != nil {
				bodyReadError <- err
				return
			}
			if err := r.Body.Close(); err != nil {
				bodyReadError <- err
				return
			}
			// Do not signal the test until the complete request body is consumed
			// and closed; otherwise net/http may not observe client cancellation.
			close(completionStarted)
			select {
			case <-r.Context().Done():
				close(cancellationObserved)
			case <-releaseHandler:
			}
			return
		}
		http.NotFound(w, r)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancellationTriggered := make(chan struct{})
	stopCancellation := make(chan struct{})
	cancellationDone := make(chan struct{})
	go func() {
		defer close(cancellationDone)
		select {
		case <-completionStarted:
			cancel()
			close(cancellationTriggered)
		case <-stopCancellation:
		}
	}()
	t.Cleanup(func() {
		cancel()
		close(stopCancellation)
		select {
		case <-cancellationDone:
		case <-time.After(2 * time.Second):
			t.Error("cancellation coordinator did not exit")
		}

		postReachedBeforeCleanup := false
		select {
		case <-postReached:
			postReachedBeforeCleanup = true
		case <-time.After(2 * time.Second):
		}
		if postReachedBeforeCleanup {
			select {
			case <-cancellationObserved:
			case <-time.After(2 * time.Second):
				t.Error("completion handler did not observe request cancellation before cleanup release")
			}
		}
		close(releaseHandler)
		srv.Close() // joins handlers after cancellation or the explicit release.
		if postReachedBeforeCleanup {
			select {
			case <-handlerDone:
			case <-time.After(2 * time.Second):
				t.Error("completion handler did not exit after cleanup release")
			}
		}
	})
	p := &OpenAICompat{ProviderName: "local", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client(), UseRealCompletionProbe: true}
	if p.Available(ctx) {
		t.Fatal("cancelled completion probe was reported as available")
	}
	select {
	case <-postReached:
	case <-time.After(2 * time.Second):
		t.Fatal("completion POST was not reached")
	}
	select {
	case <-completionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("readiness probe returned without reaching completion POST")
	}
	select {
	case err := <-bodyReadError:
		t.Fatalf("completion request body was not consumed and closed: %v", err)
	default:
	}
	select {
	case <-cancellationTriggered:
	case <-time.After(2 * time.Second):
		t.Fatal("completion POST did not trigger context cancellation")
	}
	select {
	case <-cancellationObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("completion handler did not observe request cancellation")
	}
}

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
	p := &OpenAICompat{SupportsTools: true, SupportsRequiredToolChoice: true, ProviderName: "apollo", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	first, err := p.Complete(context.Background(), Request{Prompt: "status", Tools: []ToolSpec{{Name: "machine.state", Schema: map[string]any{"type": "object"}}}, RequireTool: true})
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
	if len(requests) != 2 || requests[0].ToolChoice != "required" || len(requests[1].Messages) != 3 || requests[1].Messages[2].ToolCallID != "call-1" {
		t.Fatalf("unexpected wire requests: %+v", requests)
	}
}

func TestCompleteRejectsRequiredToolWithoutAnOfferedTool(t *testing.T) {
	p := &OpenAICompat{SupportsTools: true, ProviderName: "apollo", Endpoint: "http://unused", TierValue: TierLocal}
	if _, err := p.Complete(context.Background(), Request{Prompt: "status", RequireTool: true}); err == nil {
		t.Fatal("expected require-tool without tools to fail before contacting provider")
	}
}

func TestCompleteKeepsAutoForAutoOnlyToolEndpoint(t *testing.T) {
	var requestBody ccRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"apollo-model"}]}`))
		case "/v1/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"model":"apollo-model","choices":[{"finish_reason":"stop","message":{"content":"direct answer"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := &OpenAICompat{SupportsTools: true, ProviderName: "apollo", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	if _, err := p.Complete(context.Background(), Request{Prompt: "status", RequireTool: true, Tools: []ToolSpec{{Name: "process.census", Schema: map[string]any{"type": "object"}}}}); err != nil {
		t.Fatal(err)
	}
	if requestBody.ToolChoice != "auto" {
		t.Fatalf("auto-only endpoint tool choice = %q, want auto", requestBody.ToolChoice)
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
