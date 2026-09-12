package routerboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

func TestReceiptBoundConstructorExposesOnlyPolicyCheckedControl(t *testing.T) {
	receipt := authenticatedRoleReceipt(t)
	b := New("/nonexistent-test-binary", "", "test")
	b.payload = []byte(`{"tasks":{"restricted":true},"board":{"restricted":true}}`)
	b.version = 1
	handler := NewReceiptBoundControlHandler(b, t.TempDir(), "token", ControlRolePolicy{Role: rolereceipt.RouterAuthority},
		func(*http.Request) (rolereceipt.AuthenticatedReceipt, error) { return receipt, nil })
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/api/control", http.StatusForbidden},
		{http.MethodPost, "/api/control/action", http.StatusForbidden},
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodGet, "/index.html", http.StatusNotFound},
		{http.MethodGet, "/api/tasks", http.StatusNotFound},
		{http.MethodGet, "/api/ledger", http.StatusNotFound},
		{http.MethodGet, "/api/stream", http.StatusNotFound},
		{http.MethodGet, "/api/arm", http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Prevent a predecessor stream route from waiting indefinitely.
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"verb":"delegate","agent":"codex","task_id":"test","subject":"test"}`)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer token")
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status=%d, want %d; body=%s", response.Code, tc.status, response.Body.String())
			}
		})
	}
	b.payload = []byte(`{"build":"test","generated_at":"2026-09-07T12:00:00Z","evidence":[],"fleet":[],"activity":[],"data_errors":[],"threads":[],"registration_gaps":[],"tasks":[],"board":{},"ledger":{},"counters":{}}`)
	allowed := NewReceiptBoundControlHandler(b, t.TempDir(), "token", ControlRolePolicy{Role: rolereceipt.ConstrainedClient},
		func(*http.Request) (rolereceipt.AuthenticatedReceipt, error) { return receipt, nil })
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	req.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	allowed.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("matching-role inspect status=%d, body=%s", response.Code, response.Body.String())
	}
}
