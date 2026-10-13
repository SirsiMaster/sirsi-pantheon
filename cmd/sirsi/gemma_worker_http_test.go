package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGemmaWorkerHTTPFailureAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"refused", 400, "bounded request refused", "HTTP 400"},
		{"truncated", 200, `{"choices":[{"finish_reason":"length","message":{"content":"partial draft"}}]}`, "truncated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if stream, ok := request["stream"].(bool); !ok || stream {
					t.Error("worker request must explicitly disable streaming")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			answer, err := gemmaWarmComplete(srv.URL, "model", "draft", 1024)
			if err == nil || !strings.Contains(err.Error(), tc.want) || answer != "" {
				t.Fatalf("want unresolved %s, got answer=%q err=%v", tc.want, answer, err)
			}
		})
	}
}
