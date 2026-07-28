package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
