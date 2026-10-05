package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
		ExecutionMode:       sne.ExecutionModePlain,
	}
}

func testSNEReadiness(model string) sne.ServiceReadinessIdentity {
	return sne.ServiceReadinessIdentity{
		Status:                   "ready",
		APIContract:              sne.OpenAIChatContractV3,
		ReadyAPIContract:         sne.OpenAIChatContractV3,
		ReadyCapabilities:        sne.ReadinessCapabilities{ExecutionModes: []string{sne.ExecutionModePlain}},
		ReadyModelID:             model,
		LoadedModel:              model,
		RuntimeSHA256:            testSNEHash,
		NativeRuntimeSHA256:      testSNEHash,
		ReadyRuntimeSHA256:       testSNEHash,
		ReadyNativeRuntimeSHA256: testSNEHash,
		ReadyManifestSHA256:      testSNEHash,
		Models:                   []sne.Model{{ID: model, ManifestSHA256: testSNEHash}},
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
	if err := json.Unmarshal([]byte(`{"model":"`+model+`","choices":[{"message":{"content":"`+text+`"}}],"usage":{"prompt_tokens":3,"completion_tokens":2},"sne":{"execution":{"mode":"plain"}}}`), &response); err != nil {
		t.Fatal(err)
	}
	response.SNE.RuntimeSHA256 = testSNEHash
	response.SNE.NativeRuntimeSHA256 = testSNEHash
	response.SNE.ModelManifestSHA256 = testSNEHash
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
	missingMode := testSNEExpectation("model")
	missingMode.ExecutionMode = ""
	if _, err := NewSNEProvider(client, missingMode); err == nil || !strings.Contains(err.Error(), "explicit execution mode") {
		t.Fatalf("missing execution mode error = %v", err)
	}
	bad := testSNEExpectation("model")
	bad.ManifestSHA256 = "not-a-hash"
	if _, err := NewSNEProvider(client, bad); err == nil {
		t.Fatal("expected malformed identity rejection")
	}
	bad = testSNEExpectation("model")
	bad.RuntimeSHA256 = "invalid-runtime"
	bad.NativeRuntimeSHA256 = "invalid-native-runtime"
	bad.ManifestSHA256 = "invalid-manifest"
	if _, err := NewSNEProvider(client, bad); err == nil || !strings.Contains(err.Error(), "runtime identity must be lowercase SHA-256") {
		t.Fatalf("multiple malformed identities should report runtime first, got %v", err)
	}
}

func TestSNEProviderRejectsNilContextsWithoutClientCalls(t *testing.T) {
	client := &fakeSNEClient{readiness: testSNEReadiness("sne-model")}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
	if err != nil {
		t.Fatal(err)
	}
	if provider.Available(nil) {
		t.Fatal("SNE provider reported availability with a nil context")
	}
	if _, err := provider.Complete(nil, Request{Prompt: "hello", MaxTokens: 8}); err == nil {
		t.Fatal("SNE provider Complete accepted a nil context")
	}
	if client.complete != nil {
		t.Fatal("SNE provider contacted the client with a nil context")
	}
}

func TestSNEProviderBindsReadinessAndMapsCompletion(t *testing.T) {
	completion := fakeSNECompletion(t, "sne-model", "hello")
	completion.SNE.RuntimeSHA256 = testSNEHash
	completion.SNE.NativeRuntimeSHA256 = testSNEHash
	completion.SNE.ModelManifestSHA256 = testSNEHash
	client := &fakeSNEClient{
		readiness: testSNEReadiness("sne-model"),
		complete:  completion,
	}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
	if err != nil {
		t.Fatal(err)
	}
	if caps := provider.Caps(); !caps.Cancellation || caps.MTP || caps.Prefill || caps.Decode || caps.KVState || caps.Telemetry {
		t.Fatalf("SNE provider capability declaration is not truthful: %+v", caps)
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
	if client.request.Model != "sne-model" || client.request.ExecutionMode != sne.ExecutionModePlain || client.request.MaxTokens != 8 || client.request.Temperature != temperature || client.request.Stream {
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

func TestSNEProviderRequiresConsistentReadyStatusAndCatalogIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*sne.ServiceReadinessIdentity){
		"loaded model": func(identity *sne.ServiceReadinessIdentity) {
			identity.LoadedModel = "other-model"
		},
		"status runtime": func(identity *sne.ServiceReadinessIdentity) {
			identity.RuntimeSHA256 = strings.Repeat("a", 64)
		},
		"status native runtime": func(identity *sne.ServiceReadinessIdentity) {
			identity.NativeRuntimeSHA256 = strings.Repeat("b", 64)
		},
		"catalog manifest": func(identity *sne.ServiceReadinessIdentity) {
			identity.Models[0].ManifestSHA256 = strings.Repeat("c", 64)
		},
		"missing catalog identity": func(identity *sne.ServiceReadinessIdentity) {
			identity.Models = nil
		},
		"duplicate catalog identity": func(identity *sne.ServiceReadinessIdentity) {
			identity.Models = append(identity.Models, identity.Models[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			identity := testSNEReadiness("sne-model")
			mutate(&identity)
			client := &fakeSNEClient{readiness: identity, complete: fakeSNECompletion(t, "sne-model", "must not be sent")}
			provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
				t.Fatal("inconsistent readiness identity was accepted")
			}
			if client.request.Model != "" {
				t.Fatal("completion was sent before readiness identity consistency was established")
			}
		})
	}
}

func TestSNEProviderRejectsCompletionIdentityDrift(t *testing.T) {
	for name, mutate := range map[string]func(*sne.CompletionResponse){
		"runtime": func(response *sne.CompletionResponse) {
			response.SNE.RuntimeSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"native runtime": func(response *sne.CompletionResponse) {
			response.SNE.NativeRuntimeSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"manifest": func(response *sne.CompletionResponse) {
			response.SNE.ModelManifestSHA256 = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		},
	} {
		t.Run(name, func(t *testing.T) {
			completion := fakeSNECompletion(t, "sne-model", "ignored")
			mutate(completion)
			client := &fakeSNEClient{readiness: testSNEReadiness("sne-model"), complete: completion}
			provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Complete(context.Background(), Request{Prompt: "hello"}); err == nil {
				t.Fatalf("completion %s identity drift was accepted", name)
			}
		})
	}
}

func TestSNEProviderReportsCompletionIdentityDriftInStableOrder(t *testing.T) {
	completion := fakeSNECompletion(t, "sne-model", "ignored")
	completion.SNE.RuntimeSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	completion.SNE.NativeRuntimeSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	completion.SNE.ModelManifestSHA256 = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	client := &fakeSNEClient{readiness: testSNEReadiness("sne-model"), complete: completion}
	provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Complete(context.Background(), Request{Prompt: "hello"})
	if err == nil || !strings.Contains(err.Error(), "completion runtime identity") {
		t.Fatalf("multiple identity mismatches should report runtime first, got %v", err)
	}
}

func TestSNEProviderRequiresRatifiedContractAndExplicitExecutionMode(t *testing.T) {
	for name, mutate := range map[string]func(*sne.ServiceReadinessIdentity){
		"legacy contract": func(identity *sne.ServiceReadinessIdentity) {
			identity.APIContract = "sne.openai-chat.v2"
			identity.ReadyAPIContract = identity.APIContract
		},
		"contract disagreement": func(identity *sne.ServiceReadinessIdentity) {
			identity.APIContract = "sne.openai-chat.v2"
		},
		"unknown mode": func(identity *sne.ServiceReadinessIdentity) {
			identity.ReadyCapabilities.ExecutionModes = []string{"plain", "automatic"}
		},
		"duplicate mode": func(identity *sne.ServiceReadinessIdentity) {
			identity.ReadyCapabilities.ExecutionModes = []string{"plain", "plain"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			identity := testSNEReadiness("sne-model")
			mutate(&identity)
			client := &fakeSNEClient{readiness: identity, complete: fakeSNECompletion(t, "sne-model", "unused")}
			provider, err := NewSNEProvider(client, testSNEExpectation("sne-model"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Complete(context.Background(), Request{Prompt: "hello", MaxTokens: 8}); err == nil {
				t.Fatal("non-v3 or malformed execution-mode readiness was accepted")
			}
			if client.request.Model != "" {
				t.Fatal("completion was sent before readiness contract validation")
			}
		})
	}
}

func TestSNEProviderBindsMTPAssistantAcrossIdentityReadinessAndCompletion(t *testing.T) {
	assistant := &sne.AssistantIdentity{
		ModelID: "assistant-model", Revision: "r7", CheckpointSHA256: testSNEHash, Precision: "int8",
	}
	readiness := testSNEReadiness("sne-model")
	readyAssistant := *assistant
	readiness.ReadyCapabilities = sne.ReadinessCapabilities{
		ExecutionModes: []string{sne.ExecutionModePlain, sne.ExecutionModeMTP},
		Assistant:      &readyAssistant,
	}
	completion := fakeSNECompletion(t, "sne-model", "mtp result")
	completion.SNE.Execution.Mode = sne.ExecutionModeMTP
	completionAssistant := *assistant
	completion.SNE.Execution.Assistant = &completionAssistant
	client := &fakeSNEClient{readiness: readiness, complete: completion}
	expectation := testSNEExpectation("sne-model")
	expectation.ExecutionMode = sne.ExecutionModeMTP
	expectation.Assistant = assistant
	provider, err := NewSNEProvider(client, expectation)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Caps().MTP {
		t.Fatal("MTP was advertised before readiness")
	}
	caps, err := provider.CapabilitiesForContext(context.Background())
	if err != nil || !caps.MTP {
		t.Fatalf("readiness did not resolve MTP capability: caps=%+v err=%v", caps, err)
	}
	response, err := provider.Complete(context.Background(), Request{Prompt: "hello", MaxTokens: 8})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "mtp result" || client.request.ExecutionMode != sne.ExecutionModeMTP {
		t.Fatalf("MTP response/request mode mismatch: response=%+v request=%+v", response, client.request)
	}

	client.complete.SNE.Execution.Assistant.Revision = "substituted"
	if _, err := provider.Complete(context.Background(), Request{Prompt: "hello", MaxTokens: 8}); err == nil || !strings.Contains(err.Error(), "assistant identity") {
		t.Fatalf("MTP assistant substitution error = %v", err)
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
