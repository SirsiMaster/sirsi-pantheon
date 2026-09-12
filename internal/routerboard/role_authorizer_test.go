package routerboard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlRoleAuthorizerFailsClosedWhenMissing(t *testing.T) {
	h := &Handler{requireControlRole: true}
	if err := h.authorizeControlRole(context.Background(), "inspect"); err == nil {
		t.Fatal("missing role authorizer was accepted")
	}
}

func TestControlRoleAuthorizerBindsExactOperation(t *testing.T) {
	var got string
	h := &Handler{
		requireControlRole: true,
		controlRoleAuthorizer: func(_ context.Context, operation string) error {
			got = operation
			return nil
		},
	}
	if err := h.authorizeControlRole(context.Background(), " result_return "); err != nil {
		t.Fatalf("authorizeControlRole: %v", err)
	}
	if got != "result_return" {
		t.Fatalf("operation = %q, want result_return", got)
	}
}

func TestReceiptBoundHandlerRejectsRoleBeforeSnapshot(t *testing.T) {
	h := NewHandlerWithControlAuthAndRole(nil, "", "token", func(_ context.Context, operation string) error {
		if operation != "inspect" {
			t.Fatalf("operation = %q, want inspect", operation)
		}
		return errors.New("expired role")
	})
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	req.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
