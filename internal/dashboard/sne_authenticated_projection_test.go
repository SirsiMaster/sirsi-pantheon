package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSNEReadClientUsesCurrentRotatedCapability(t *testing.T) {
	wantToken := "abcdefghijklmnopqrstuvwxyz123456"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+wantToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/health/ready":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ready", "service_version": "test", "api_version": "v0", "api_contract": "sne.openai-chat.v2",
				"profile": "interactive", "runtime_sha256": string(make([]byte, 64)), "model_id": "model", "model_manifest_sha256": string(make([]byte, 64)),
				"max_concurrent_requests": 1, "max_queued_requests": 8, "queue_discipline": "fifo", "request_timeout_ms": 120000,
			})
		case "/v1/sne/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"profile": "interactive", "runtime_sha256": string(make([]byte, 64)), "loaded_model": "model", "api_contract": "sne.openai-chat.v2",
				"max_concurrent_requests": 1, "max_queued_requests": 8, "queue_discipline": "fifo", "request_timeout_ms": 120000,
			})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "model", "manifest_sha256": string(make([]byte, 64))}}})
		case "/v1/sne/metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{"requests_active": 1, "requests_queued": 2, "max_concurrent_requests": 1, "max_queued_requests": 8, "queue_discipline": "fifo", "request_timeout_ms": 120000})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := New(Config{SNELocalAccessToken: "old-capability-abcdefghijklmnopqrstuvwxyz"})
	server.sneAccess.replace(wantToken)
	client, err := server.newSNEReadClient(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ReadinessIdentity(context.Background()); err != nil {
		t.Fatalf("rotated capability did not reach secured SNE readiness projection: %v", err)
	}
	metrics, err := client.Metrics(context.Background())
	if err != nil || metrics.RequestsActive != 1 || metrics.RequestsQueued != 2 || metrics.QueueDiscipline != "fifo" {
		t.Fatalf("rotated capability did not expose exact queue telemetry: metrics=%+v err=%v", metrics, err)
	}
}
