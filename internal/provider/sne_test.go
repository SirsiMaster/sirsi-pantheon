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

const testSNEHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testSNEExpectation(model string) SNEIdentityExpectation {
	return SNEIdentityExpectation{
		ModelID:             model,
		RuntimeSHA256:       testSNEHash,
		NativeRuntimeSHA256: testSNEHash,
		ManifestSHA256:      testSNEHash,
	}
}

func testSNEReadiness(model string) sne.ServiceReadinessIdentity {
	return sne.ServiceReadinessIdentity{
		Status:                   "ready",
		ReadyModelID:             model,
		RuntimeSHA256:            testSNEHash,
		NativeRuntimeSHA256:      testSNEHash,
		ReadyRuntimeSHA256:       testSNEHash,
		ReadyNativeRuntimeSHA256: testSNEHash,
		ReadyManifestSHA256:      testSNEHash,
	}
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
	if _, err := NewSNEProvider(nil, testSNEExpectation("model")); err == nil {
		t.Fatal("expected nil client rejection")
	}
	if _, err := NewSNEProvider(client, testSNEExpectation(" ")); err == nil {
		t.Fatal("expected empty model rejection")
	}
	bad := testSNEExpectation("model")
	bad.ManifestSHA256 = "not-a-hash"
	if _, err := NewSNEProvider(client, bad); err == nil {
		t.Fatal("expected malformed identity rejection")
	}
}

func TestSNEProviderBindsReadinessAndMapsCompletion(t *testing.T) {
	client := &fakeSNEClient{
		readiness: testSNEReadiness("sne-model"),
		complete:  fakeSNECompletion(t, "sne-model", "hello"),
	}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Available(context.Background()) {
		t.Fatal("expected ready SNE provider to be available")
	}
	temperature := 0.65
	response, err := provider.Complete(context.Background(), Request{System: "be concise", Prompt: "hello", MaxTokens: 8, Temperature: &temperature})
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
	if client.request.Model != "sne-model" || client.request.MaxTokens != 8 || client.request.Temperature != temperature || client.request.Stream {
		t.Fatalf("unexpected native request: %+v", client.request)
	}
}

func TestSNEProviderRejectsReadinessAndCompletionIdentityDrift(t *testing.T) {
	client := &fakeSNEClient{
		readiness: testSNEReadiness("other-model"),
		complete:  fakeSNECompletion(t, "sne-model", "ignored"),
	}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
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

func TestSNEProviderRejectsRuntimeAndManifestIdentityDrift(t *testing.T) {
	client := &fakeSNEClient{
		readiness: testSNEReadiness("sne-model"),
		complete:  fakeSNECompletion(t, "sne-model", "ignored"),
	}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*sne.ServiceReadinessIdentity){
		"runtime": func(identity *sne.ServiceReadinessIdentity) {
			identity.RuntimeSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			identity.ReadyRuntimeSHA256 = identity.RuntimeSHA256
		},
		"native runtime": func(identity *sne.ServiceReadinessIdentity) {
			identity.NativeRuntimeSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			identity.ReadyNativeRuntimeSHA256 = identity.NativeRuntimeSHA256
		},
		"manifest": func(identity *sne.ServiceReadinessIdentity) {
			identity.ReadyManifestSHA256 = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		},
	} {
		identity := testSNEReadiness("sne-model")
		mutate(&identity)
		client.readiness = identity
		if provider.Available(context.Background()) {
			t.Fatalf("expected %s identity drift to be unavailable", name)
		}
	}
}

func TestSNEProviderRejectsUnavailableAndMalformedResponses(t *testing.T) {
	client := &fakeSNEClient{
		readiness:   sne.ServiceReadinessIdentity{Status: "starting", ReadyModelID: "sne-model", RuntimeSHA256: testSNEHash, NativeRuntimeSHA256: testSNEHash, ReadyManifestSHA256: testSNEHash},
		completeErr: errors.New("backend unavailable"),
	}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
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
