package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeCompletionRejectsGreenHealthWhenInferenceFails(t *testing.T) {
	var healthCalls, completionCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			healthCalls++
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
		case "/v1/chat/completions":
			completionCalls++
			http.Error(w, "queue unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &OpenAICompat{ProviderName: "sne", Endpoint: srv.URL + "/v1", TierValue: TierLocal, HTTP: srv.Client()}
	err := p.ProbeCompletion(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ProbeCompletion() error = %v, want ErrUnavailable", err)
	}
	if healthCalls != 0 {
		t.Fatalf("readiness consulted /health %d time(s); health is not an inference proof", healthCalls)
	}
	if completionCalls != 1 {
		t.Fatalf("completion probes = %d, want exactly 1", completionCalls)
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

func TestDiscoverCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sirsi/capabilities" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"contract_version":"1","model":{"id":"gemma","qualified_prompt_tokens":1024},"serving":{"streaming":true},"determinism":{"batch_invariant_active":true}}`))
	}))
	defer srv.Close()
	p := &OpenAICompat{ProviderName: "sne", Endpoint: srv.URL + "/v1", SupportsStreaming: true, HTTP: srv.Client()}
	caps, model, err := p.DiscoverCapabilities(context.Background())
	if err != nil || model != "gemma" || caps.ContextTokens != 1024 || !caps.Streaming || !caps.Deterministic || caps.JSONMode || caps.Tools {
		t.Fatalf("caps=%+v model=%q err=%v", caps, model, err)
	}
}

func TestDiscoverCapabilitiesLegacyDoesNotInventQualification(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p := &OpenAICompat{ProviderName: "legacy", Endpoint: srv.URL + "/v1", SupportsStreaming: true, SupportsJSON: true, ContextTokens: 8192, HTTP: srv.Client()}
	caps, model, err := p.DiscoverCapabilities(context.Background())
	if err != nil || model != "" || caps.ContextTokens != 0 || caps.JSONMode || !caps.Streaming {
		t.Fatalf("caps=%+v model=%q err=%v", caps, model, err)
	}
}

func TestDiscoverCapabilitiesRejectsUnknownMajor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"contract_version":"2"}`)) }))
	defer srv.Close()
	p := &OpenAICompat{ProviderName: "future", Endpoint: srv.URL + "/v1", HTTP: srv.Client()}
	if _, _, err := p.DiscoverCapabilities(context.Background()); err == nil {
		t.Fatal("expected unknown major contract to fail closed")
	}
}

func TestAvailabilityClassifiesCapacityAndBudget(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   Availability
	}{{http.StatusTooManyRequests, RateLimit}, {http.StatusPaymentRequired, Budgeted}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }))
		p := &OpenAICompat{ProviderName: "remote", Endpoint: srv.URL + "/v1", HTTP: srv.Client()}
		if got := p.Availability(context.Background()); got != tc.want {
			t.Errorf("status %d: got %q want %q", tc.status, got, tc.want)
		}
		srv.Close()
	}
}

func TestCompletePreservesBackendTextByteForByte(t *testing.T) {
	const raw = "backend-marker:thought|backend-marker:final"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"gemma"}]}`))
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "gemma", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": raw}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := &OpenAICompat{ProviderName: "sne", Endpoint: srv.URL + "/v1", HTTP: srv.Client()}
	resp, err := p.Complete(context.Background(), Request{Prompt: "hello"})
	if err != nil || resp.Text != raw {
		t.Fatalf("text=%q err=%v", resp.Text, err)
	}
}
