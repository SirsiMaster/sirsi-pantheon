package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSNEControlPreflightAllowsKnownNexusOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/api/sne/start", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	recorder := httptest.NewRecorder()
	new(Server).apiSNEStart(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestSNEControlRejectsUnknownOriginBeforeMutation(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sne/stop", nil)
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	new(Server).apiSNEStop(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}
