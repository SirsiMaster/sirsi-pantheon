package sne

import (
	"context"
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
			fmt.Fprintf(w, `{"status":"ready","service_version":"2.4.1","api_version":"v0","api_contract":"sne.openai-chat.v2","profile":"interactive","runtime_sha256":%q,"model_id":"gemma-test","model_manifest_sha256":%q,"max_concurrent_requests":1,"max_queued_requests":8,"queue_discipline":"fifo","request_timeout_ms":120000}`, runtimeSHA, manifestSHA)
		case "/v1/sne/status":
			fmt.Fprintf(w, `{"profile":"interactive","api_contract":"sne.openai-chat.v2","runtime_sha256":%q,"loaded_model":"gemma-test","max_concurrent_requests":1,"max_queued_requests":8,"queue_discipline":"fifo","request_timeout_ms":120000}`, runtimeSHA)
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
	if identity.Status != "ready" || identity.ServiceVersion != "2.4.1" || identity.APIVersion != "v0" || identity.APIContract != "sne.openai-chat.v2" || identity.ReadyAPIContract != identity.APIContract || identity.Profile != "interactive" || identity.RuntimeSHA256 != runtimeSHA || identity.LoadedModel != "gemma-test" || len(identity.Models) != 1 || identity.Models[0].ManifestSHA256 != manifestSHA || identity.MaxConcurrentRequests != 1 || identity.MaxQueuedRequests != 8 || identity.QueueDiscipline != "fifo" || identity.RequestTimeoutMS != 120000 || identity.ReadyMaxConcurrentRequests != 1 || identity.ReadyMaxQueuedRequests != 8 {
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
