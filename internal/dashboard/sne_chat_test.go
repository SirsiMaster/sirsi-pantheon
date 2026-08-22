package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestSNEChatPreflightAllowsNexusDevelopmentOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/api/sne/chat", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	recorder := httptest.NewRecorder()
	new(Server).apiSNEChat(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestProxySNEChatStreamsExactSSE(t *testing.T) {
	const model = "signed-model"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"signed-model\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/sne/chat", strings.NewReader(`{"model":"signed-model","stream":true}`))
	recorder := httptest.NewRecorder()
	proxySNEChatTo(recorder, request, upstream.URL, model)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"content":"hello"`) || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream was not preserved: %q", body)
	}
}

func TestProxySNEChatRejectsModelMismatch(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sne/chat", strings.NewReader(`{"model":"other"}`))
	recorder := httptest.NewRecorder()
	proxySNEChatTo(recorder, request, "http://127.0.0.1:8477", "signed-model")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var response sneOpenAIErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Error.Code != "model_identity_mismatch" || response.SNE == nil || !response.SNE.NoFallback {
		t.Fatalf("OpenAI error envelope = %+v err=%v", response, err)
	}
}

func TestWriteSNEOpenAIErrorPreservesSafeRecovery(t *testing.T) {
	resource := sne.ResourceAdmission{RequiredBytes: 25_545_459_702, AvailableRAMBytes: 18 << 30, SwapUsedBytes: 4 << 30, SwapLimitBytes: 3 << 30}
	lifecycle := SNELifecycleState{ErrorCode: "swap_cleanup_required", Recovery: "Restart the Mac and retry.", ResourceAdmission: &resource, Error: "/private/tmp/secret"}
	recorder := httptest.NewRecorder()
	writeSNEOpenAIError(recorder, http.StatusServiceUnavailable, lifecycle.ErrorCode, "the verified SNE runtime is not ready", &lifecycle)
	var response sneOpenAIErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "swap_cleanup_required" || response.SNE == nil || !response.SNE.NoFallback || response.SNE.Recovery != lifecycle.Recovery || response.SNE.SwapUsedBytes != resource.SwapUsedBytes {
		t.Fatalf("OpenAI recovery envelope = %+v", response)
	}
	if strings.Contains(recorder.Body.String(), "/private/tmp") {
		t.Fatalf("OpenAI recovery leaked private error: %s", recorder.Body.String())
	}
}
