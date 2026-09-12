package routerboard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

func TestWithControlRoleReceiptSourceFailsClosedWhenMissing(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true })
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	response := httptest.NewRecorder()
	WithControlRoleReceiptSource(next, nil).ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable || nextCalled {
		t.Fatalf("missing source status=%d nextCalled=%v", response.Code, nextCalled)
	}
}

func TestWithControlRoleReceiptSourceBindsExactReceiptToContext(t *testing.T) {
	receipt := authenticatedRoleReceipt(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bound, ok := AuthenticatedControlRoleFromContext(r.Context())
		if !ok || bound.RawSHA256() != receipt.RawSHA256() {
			t.Fatalf("request context lost authenticated receipt")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	source := func(*http.Request) (rolereceipt.AuthenticatedReceipt, error) { return receipt, nil }
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	response := httptest.NewRecorder()
	WithControlRoleReceiptSource(next, source).ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestWithControlRoleReceiptSourceRejectsSourceFailure(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next handler was called") })
	source := func(*http.Request) (rolereceipt.AuthenticatedReceipt, error) {
		return rolereceipt.AuthenticatedReceipt{}, errors.New("revoked")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	response := httptest.NewRecorder()
	WithControlRoleReceiptSource(next, source).ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestContextControlRoleAuthorizerRequiresBoundReceipt(t *testing.T) {
	if _, err := ContextControlRoleAuthorizer(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "inspect"); err == nil {
		t.Fatal("missing context receipt was accepted")
	}
}

func TestValidateControlRoleOperationRejectsBlank(t *testing.T) {
	if err := ValidateControlRoleOperation(" "); err == nil {
		t.Fatal("blank operation was accepted")
	}
}
