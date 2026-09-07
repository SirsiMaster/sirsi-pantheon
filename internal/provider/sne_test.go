package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

type fakeSNEClient struct {
	readiness   sne.ServiceReadinessIdentity
	readyErr    error
	complete    *sne.CompletionResponse
	completeErr error
	request     sne.CompletionRequest
}

func (f *fakeSNEClient) ReadinessIdentity(context.Context) (sne.ServiceReadinessIdentity, error) {
	if f.readyErr != nil {
		return sne.ServiceReadinessIdentity{}, f.readyErr
	}
	return f.readiness, nil
}

func (f *fakeSNEClient) Complete(_ context.Context, request sne.CompletionRequest) (*sne.CompletionResponse, error) {
	f.request = request
	return f.complete, f.completeErr
}

func fakeSNECompletion(t *testing.T, model, text string) *sne.CompletionResponse {
	t.Helper()
	var response sne.CompletionResponse
	if err := json.Unmarshal([]byte(`{"model":"`+model+`","choices":[{"message":{"content":"`+text+`"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`), &response); err != nil {
		t.Fatal(err)
	}
	return &response
}

func TestNewSNEProviderRequiresBoundClientAndModel(t *testing.T) {
	client := &fakeSNEClient{}
	if _, err := NewSNEProvider(nil, "model"); err == nil {
		t.Fatal("expected nil client rejection")
	}
	if _, err := NewSNEProvider(client, " "); err == nil {
		t.Fatal("expected empty model rejection")
	}
}

func TestSNEProviderBindsReadinessAndMapsCompletion(t *testing.T) {
	client := &fakeSNEClient{
		readiness: sne.ServiceReadinessIdentity{Status: "ready", ReadyModelID: "sne-model"},
		complete:  fakeSNECompletion(t, "sne-model", "hello"),
	}
	provider, err := NewSNEProvider(client, "sne-model")
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Available(context.Background()) {
		t.Fatal("expected ready SNE provider to be available")
	}
	response, err := provider.Complete(context.Background(), Request{System: "be concise", Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "hello" || response.Model != "sne-model" || response.Provider != "sne" || response.Tier != TierLocal {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.PromptTokens != 3 || response.OutputTokens != 2 || response.FinishReason != "stop" {
		t.Fatalf("unexpected usage/finish: %+v", response)
	}
	if len(client.request.Messages) != 2 || client.request.Messages[0].Role != "system" || client.request.Messages[1].Role != "user" {
		t.Fatalf("unexpected mapped messages: %+v", client.request.Messages)
	}
	if client.request.Model != "sne-model" || client.request.MaxTokens != 8 || client.request.Stream {
		t.Fatalf("unexpected native request: %+v", client.request)
	}
}

func TestSNEProviderRejectsReadinessAndCompletionIdentityDrift(t *testing.T) {
	client := &fakeSNEClient{
		readiness: sne.ServiceReadinessIdentity{Status: "ready", ReadyModelID: "other-model"},
		complete:  fakeSNECompletion(t, "sne-model", "ignored"),
	}
	provider, err := NewSNEProvider(client, "sne-model")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Available(context.Background()) {
		t.Fatal("expected readiness model drift to be unavailable")
	}
	if _, err := provider.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
		t.Fatal("expected completion to fail closed on readiness drift")
	}

	client.readiness.ReadyModelID = "sne-model"
	client.complete = fakeSNECompletion(t, "other-model", "ignored")
	if _, err := provider.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
		t.Fatal("expected served model drift rejection")
	}
}

func TestSNEProviderRejectsUnavailableAndMalformedResponses(t *testing.T) {
	client := &fakeSNEClient{
		readiness:   sne.ServiceReadinessIdentity{Status: "starting", ReadyModelID: "sne-model"},
		completeErr: errors.New("backend unavailable"),
	}
	provider, err := NewSNEProvider(client, "sne-model")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
		t.Fatal("expected non-ready service rejection")
	}

	client.readiness.Status = "ready"
	client.completeErr = nil
	client.complete = &sne.CompletionResponse{}
	if _, err := provider.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
		t.Fatal("expected empty completion rejection")
	}
	if _, err := provider.Complete(context.Background(), Request{}); err == nil {
		t.Fatal("expected empty prompt rejection")
	}
}
