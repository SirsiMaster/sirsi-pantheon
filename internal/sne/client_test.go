package sne

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessIdentityCollectsCompleteTuple(t *testing.T) {
	runtimeSHA := strings.Repeat("a", 64)
	manifestSHA := strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health/ready":
			fmt.Fprintf(w, `{"status":"ready","service_version":"2.4.1","api_version":"v0","api_contract":"%s","capabilities":{"execution_modes":["plain"]},"profile":"interactive","runtime_sha256":%q,"model_id":"gemma-test","model_manifest_sha256":%q,"max_concurrent_requests":1,"max_queued_requests":8,"queue_discipline":"fifo","request_timeout_ms":120000}`, OpenAIChatContractV3, runtimeSHA, manifestSHA)
		case "/v1/sne/status":
			fmt.Fprintf(w, `{"profile":"interactive","api_contract":"%s","runtime_sha256":%q,"loaded_model":"gemma-test","max_concurrent_requests":1,"max_queued_requests":8,"queue_discipline":"fifo","request_timeout_ms":120000}`, OpenAIChatContractV3, runtimeSHA)
		case "/v1/models":
			fmt.Fprintf(w, `{"object":"list","data":[{"id":"gemma-test","manifest_sha256":%q}]}`, manifestSHA)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := client.ReadinessIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if identity.Status != "ready" || identity.ServiceVersion != "2.4.1" || identity.APIVersion != "v0" || identity.APIContract != OpenAIChatContractV3 || identity.ReadyAPIContract != identity.APIContract || len(identity.ReadyCapabilities.ExecutionModes) != 1 || identity.ReadyCapabilities.ExecutionModes[0] != ExecutionModePlain || identity.Profile != "interactive" || identity.RuntimeSHA256 != runtimeSHA || identity.LoadedModel != "gemma-test" || len(identity.Models) != 1 || identity.Models[0].ManifestSHA256 != manifestSHA || identity.MaxConcurrentRequests != 1 || identity.MaxQueuedRequests != 8 || identity.QueueDiscipline != "fifo" || identity.RequestTimeoutMS != 120000 || identity.ReadyMaxConcurrentRequests != 1 || identity.ReadyMaxQueuedRequests != 8 {
		t.Fatalf("identity=%+v", identity)
	}
}

func TestModelLifecyclePreservesRestartRequiredContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || (r.URL.Path != "/v1/sne/model/load" && r.URL.Path != "/v1/sne/model/unload" && r.URL.Path != "/v1/sne/model/reload") {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"restart_required","message":"supervised restart required","retryable":true}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	model := "gemma-4-12b-it-affine8-sne-v1"
	for action, invoke := range map[string]func(context.Context, string) error{
		"load": client.LoadModel, "unload": client.UnloadModel, "reload": client.ReloadModel,
	} {
		err = invoke(context.Background(), model)
		if !IsRestartRequired(err) {
			t.Fatalf("%s error=%v", action, err)
		}
	}
}

func TestCompleteRequiresAndSendsExplicitExecutionMode(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request CompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.ExecutionMode != ExecutionModePlain {
			t.Fatalf("execution_mode=%q", request.ExecutionMode)
		}
		w.Header().Set("Content-Type", "application/json")
		sha := strings.Repeat("a", 64)
		_, _ = fmt.Fprintf(w, `{"model":"gemma-test","choices":[{"message":{"role":"assistant","content":"ok"}}],"sne":{"runtime_sha256":%q,"native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"plain"}}}`, sha, sha, sha)
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	base := CompletionRequest{Model: "gemma-test", Messages: []Message{{Role: "user", Content: "hello"}}}
	if _, err := client.Complete(context.Background(), base); err == nil {
		t.Fatal("completion without explicit execution mode was accepted")
	}
	if requests != 0 {
		t.Fatalf("invalid request reached service %d times", requests)
	}
	base.ExecutionMode = ExecutionModePlain
	if _, err := client.Complete(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("valid request count=%d, want 1", requests)
	}
}

func TestCompleteAcceptsMTPOnlyWithBoundAssistantAndRuntimeIdentity(t *testing.T) {
	sha := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request CompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.ExecutionMode != ExecutionModeMTP {
			t.Fatalf("execution_mode=%q, want %q", request.ExecutionMode, ExecutionModeMTP)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"model":"gemma-test","choices":[{"message":{"role":"assistant","content":"ok"}}],"sne":{"runtime_sha256":%q,"native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"mtp","assistant":{"model_id":"assistant-model","revision":"r1","checkpoint_sha256":%q,"precision":"int8"}}}}`, sha, sha, sha, sha)
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := client.Complete(context.Background(), CompletionRequest{
		Model: "gemma-test", ExecutionMode: ExecutionModeMTP,
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if completion.SNE.Execution.Assistant == nil || completion.SNE.Execution.Assistant.ModelID != "assistant-model" {
		t.Fatalf("assistant identity=%+v", completion.SNE.Execution.Assistant)
	}
}

func TestCompleteRejectsResponseModeAndIdentityDrift(t *testing.T) {
	sha := strings.Repeat("a", 64)
	plainSNE := fmt.Sprintf(`{"runtime_sha256":%q,"native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"plain"}}`, sha, sha, sha)
	mtpSNE := fmt.Sprintf(`{"runtime_sha256":%q,"native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"mtp"}}`, sha, sha, sha)
	plainWithAssistant := fmt.Sprintf(`{"runtime_sha256":%q,"native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"plain","assistant":{"model_id":"a","revision":"r1","checkpoint_sha256":%q,"precision":"int8"}}}`, sha, sha, sha, sha)
	cases := []struct {
		name          string
		model         string
		executionMode string
		sneJSON       string
	}{
		{name: "wrong model", model: "other-model", executionMode: ExecutionModePlain, sneJSON: plainSNE},
		{name: "wrong mode", model: "gemma-test", executionMode: ExecutionModePlain, sneJSON: mtpSNE},
		{name: "missing runtime identity", model: "gemma-test", executionMode: ExecutionModePlain, sneJSON: fmt.Sprintf(`{"native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"plain"}}`, sha, sha)},
		{name: "malformed runtime identity", model: "gemma-test", executionMode: ExecutionModePlain, sneJSON: fmt.Sprintf(`{"runtime_sha256":"%s","native_runtime_sha256":%q,"model_manifest_sha256":%q,"execution":{"mode":"plain"}}`, strings.Repeat("A", 64), sha, sha)},
		{name: "plain assistant", model: "gemma-test", executionMode: ExecutionModePlain, sneJSON: plainWithAssistant},
		{name: "mtp missing assistant", model: "gemma-test", executionMode: ExecutionModeMTP, sneJSON: mtpSNE},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":%q,"choices":[{"message":{"role":"assistant","content":"ok"}}],"sne":%s}`, tc.model, tc.sneJSON)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Complete(context.Background(), CompletionRequest{
				Model: "gemma-test", ExecutionMode: tc.executionMode,
				Messages: []Message{{Role: "user", Content: "hello"}},
			})
			if err == nil {
				t.Fatal("completion with response identity drift was accepted")
			}
		})
	}
}

func TestReadinessRejectsDuplicateJSONIdentityKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health/ready" {
			_, _ = w.Write([]byte(`{"status":"ready","api_contract":"sne.openai-chat.v3","api_contract":"sne.openai-chat.v2","capabilities":{"execution_modes":["plain"]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadinessIdentity(context.Background()); err == nil || !strings.Contains(err.Error(), `duplicate object key "api_contract"`) {
		t.Fatalf("duplicate readiness identity key error = %v", err)
	}
}

func TestReadinessCapabilitiesAndAssistantAreClosedShapes(t *testing.T) {
	for _, raw := range []string{
		`{"execution_modes":["plain"],"unexpected":true}`,
		`{"execution_modes":["mtp"],"assistant":{"model_id":"a","revision":"r1","checkpoint_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","precision":"int8","unexpected":true}}`,
		`{"execution_modes":["plain"],"execution_modes":["plain"]}`,
		`{"execution_modes":["mtp"],"assistant":{"model_id":"a","model_id":"b","revision":"r1","checkpoint_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","precision":"int8"}}`,
	} {
		var capabilities ReadinessCapabilities
		if err := json.Unmarshal([]byte(raw), &capabilities); err == nil {
			t.Fatalf("invalid readiness capability shape accepted: %s", raw)
		}
	}
	var assistant AssistantIdentity
	if err := json.Unmarshal([]byte(`{"model_id":"a","model_id":"b","revision":"r1","checkpoint_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","precision":"int8"}`), &assistant); err == nil {
		t.Fatal("assistant identity accepted a duplicate model_id")
	}
}

func TestServiceReadinessContractIsSharedAndFailClosed(t *testing.T) {
	valid := ServiceReadinessIdentity{
		Status: "ready", APIContract: OpenAIChatContractV3, ReadyAPIContract: OpenAIChatContractV3,
		ReadyCapabilities: ReadinessCapabilities{ExecutionModes: []string{ExecutionModePlain}},
	}
	if err := valid.ValidateContract(); err != nil {
		t.Fatalf("valid v3 readiness rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ServiceReadinessIdentity){
		"not-ready":            func(identity *ServiceReadinessIdentity) { identity.Status = "starting" },
		"old-status-contract":  func(identity *ServiceReadinessIdentity) { identity.APIContract = "sne.openai-chat.v2" },
		"mismatched-contracts": func(identity *ServiceReadinessIdentity) { identity.ReadyAPIContract = "sne.openai-chat.v2" },
		"invalid-capability-shape": func(identity *ServiceReadinessIdentity) {
			identity.ReadyCapabilities = ReadinessCapabilities{ExecutionModes: []string{ExecutionModeMTP}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			identity := valid
			mutate(&identity)
			if err := identity.ValidateContract(); err == nil {
				t.Fatal("invalid readiness contract was accepted")
			}
		})
	}
}
