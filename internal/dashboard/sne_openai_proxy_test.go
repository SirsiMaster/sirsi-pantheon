package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxySNEChatPreservesNonStreamingOpenAIContract(t *testing.T) {
	const model = "signed-model"
	const completion = `{"id":"chatcmpl-local","object":"chat.completion","model":"signed-model","choices":[{"message":{"role":"assistant","content":"local"}}]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, completion)
	}))
	defer upstream.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"signed-model","stream":false,"messages":[{"role":"user","content":"hi"}]}`))
	recorder := httptest.NewRecorder()
	proxySNEChatTo(recorder, request, upstream.URL, model)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/json" || recorder.Body.String() != completion {
		t.Fatalf("response status=%d type=%q body=%q", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
}

func TestProxySNEChatPreservesQueueRetryHint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"queue_full","retryable":true}}`)
	}))
	defer upstream.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"signed-model","stream":false}`))
	recorder := httptest.NewRecorder()
	proxySNEChatTo(recorder, request, upstream.URL, "signed-model")
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "1" || !strings.Contains(recorder.Body.String(), "queue_full") {
		t.Fatalf("queue response status=%d retry=%q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
}

func TestProxySNEChatPropagatesStreamingCancellation(t *testing.T) {
	upstreamCanceled := make(chan struct{})
	firstChunk := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(firstChunk)
		<-request.Context().Done()
		close(upstreamCanceled)
	}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"signed-model","stream":true}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() {
		proxySNEChatTo(recorder, request, upstream.URL, "signed-model")
		close(returned)
	}()
	select {
	case <-firstChunk:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not reach local SNE upstream")
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Pantheon proxy did not return after cancellation")
	}
}

func TestProxySNEChatHonorsCallerDeadlineBeforeResponseHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer upstream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"signed-model","stream":false}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	started := time.Now()
	proxySNEChatTo(recorder, request, upstream.URL, "signed-model")
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Pantheon ignored caller deadline: elapsed=%s", elapsed)
	}
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProxySNEChatPreservesAdvancedOpenAIFieldsVerbatim(t *testing.T) {
	const body = `{"model":"signed-model","stream":false,"messages":[{"role":"system","content":"return JSON"},{"role":"user","content":"hi"}],"max_tokens":128,"stop":["END"],"response_format":{"type":"json_object"},"temperature":0}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		got, err := io.ReadAll(request.Body)
		if err != nil || string(got) != body {
			t.Fatalf("request body changed: err=%v body=%s", err, got)
		}
		var decoded map[string]any
		if err := json.Unmarshal(got, &decoded); err != nil || decoded["max_tokens"] != float64(128) {
			t.Fatalf("advanced request contract lost: err=%v body=%s", err, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`)
	}))
	defer upstream.Close()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	proxySNEChatTo(recorder, request, upstream.URL, "signed-model")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProxySNEChatRejectsOversizedRequestBeforeUpstream(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) { called = true }))
	defer upstream.Close()

	body := `{"model":"signed-model","messages":[{"role":"user","content":"` + strings.Repeat("x", maxSNEChatBody) + `"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	proxySNEChatTo(recorder, request, upstream.URL, "signed-model")
	if recorder.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("status=%d upstream_called=%v body=%s", recorder.Code, called, recorder.Body.String())
	}
}
