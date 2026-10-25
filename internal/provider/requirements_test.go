package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGenerationRequirementsDirectRefuseBeforeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		req    Request
		state  CapabilityState
		stream bool
		want   string
	}{
		{"unknown-json", Request{ResponseFormat: ResponseFormatJSONObject}, CapabilityUnknown, false, "unknown"},
		{"unsupported-json", Request{ResponseFormat: ResponseFormatJSONObject}, CapabilityUnsupported, false, "declared unsupported"},
		{"invalid-state", Request{ResponseFormat: ResponseFormatJSONObject}, "invented", false, "unknown"},
		{"invalid-format", Request{ResponseFormat: "json_schema"}, CapabilitySupported, false, "unsupported response format"},
		{"unknown-determinism", Request{Determinism: DeterminismBatchInvariant}, CapabilitySupported, false, "unknown"},
		{"invented-mode", Request{Determinism: "deterministic:true"}, CapabilitySupported, false, "unsupported determinism"},
		{"json-stream", Request{ResponseFormat: ResponseFormatJSONObject}, CapabilitySupported, true, "streaming"},
		{"determinism-stream", Request{Determinism: DeterminismBatchInvariant}, CapabilitySupported, true, "unknown"},
		{"json-tool", Request{ResponseFormat: ResponseFormatJSONObject, RequireTool: true}, CapabilitySupported, false, "tools"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "must not reach HTTP", http.StatusInternalServerError)
			}))
			defer server.Close()
			p := &OpenAICompat{Endpoint: server.URL, JSONObject: tc.state, SupportsStreaming: true, UseRealCompletionProbe: true}
			var err error
			if tc.stream {
				_, err = p.Stream(context.Background(), tc.req)
			} else {
				_, err = p.Complete(context.Background(), tc.req)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || calls.Load() != 0 {
				t.Fatalf("direct requirement escaped: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}

func TestGenerationRequirementsExactJSONObjectWireAndDefault(t *testing.T) {
	for _, format := range []ResponseFormat{"", ResponseFormatText, ResponseFormatJSONObject} {
		t.Run("format-"+string(format), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				got, exists := body["response_format"]
				if format == ResponseFormatJSONObject {
					if string(got) != `{"type":"json_object"}` {
						t.Errorf("required exact response_format missing/wrong: %s", got)
					}
				} else if exists {
					t.Errorf("default/text changed wire: %s", got)
				}
				if _, exists := body["deterministic"]; exists {
					t.Error("invented deterministic wire flag")
				}
				_, _ = io.WriteString(w, `{"model":"fixture-model","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`)
			}))
			defer server.Close()
			p := &OpenAICompat{Endpoint: server.URL, Model: "fixture-model", JSONObject: CapabilitySupported}
			resp, err := p.Complete(context.Background(), Request{ResponseFormat: format, Prompt: "Return JSON"})
			if err != nil || resp.Text != `{"ok":true}` || calls.Load() != 1 {
				t.Fatalf("response=%+v calls=%d err=%v", resp, calls.Load(), err)
			}
		})
	}
}

func TestGenerationRequirementsRejectInvalidJSONResponse(t *testing.T) {
	for _, tc := range []struct{ name, content, finish string }{
		{"trailing-object", `{} {}`, "stop"},
		{"trailing-text", `{} extra`, "stop"},
		{"truncated", `{"a":`, "stop"},
		{"array", `[]`, "stop"},
		{"null", `null`, "stop"},
		{"empty", "", "stop"},
		{"token-limit", `{}`, "length"},
		{"filter", `{}`, "content_filter"},
		{"missing-terminal", `{}`, ""},
		{"tool-only", "", "tool_calls"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": tc.finish, "message": map[string]any{"content": tc.content}}}})
			}))
			defer server.Close()
			p := &OpenAICompat{Endpoint: server.URL, Model: "fixture", JSONObject: CapabilitySupported}
			resp, err := p.Complete(context.Background(), Request{ResponseFormat: ResponseFormatJSONObject})
			if !errors.Is(err, ErrInvalidJSONObject) || resp.Text != "" || calls.Load() != 1 {
				t.Fatalf("invalid result accepted/replayed: resp=%+v calls=%d err=%v", resp, calls.Load(), err)
			}
		})
	}
}

func TestGenerationRequirementsRejectEnvelopeAndRefusal(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]} {}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"{}","refusal":"refused"}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"{}","tool_calls":[{"id":"x"}]}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}],"error":{"message":"private body"}}`,
		`{"choices":[`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer server.Close()
			_, err := (&OpenAICompat{Endpoint: server.URL, Model: "fixture", JSONObject: CapabilitySupported}).Complete(context.Background(), Request{ResponseFormat: ResponseFormatJSONObject})
			if !errors.Is(err, ErrInvalidJSONObject) || strings.Contains(err.Error(), "private body") {
				t.Fatalf("invalid envelope error = %v", err)
			}
		})
	}
}

func TestGenerationRequirementsCanceledDirectDoesNotSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &OpenAICompat{Endpoint: "http://must-not-resolve.invalid", JSONObject: CapabilitySupported}
	_, err := p.Complete(ctx, Request{ResponseFormat: ResponseFormatJSONObject})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation did not precede discovery: %v", err)
	}
}
