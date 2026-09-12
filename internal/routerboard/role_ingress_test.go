package routerboard

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

func TestHeaderControlRoleReceiptSourceDelegatesExactBytes(t *testing.T) {
	receipt := authenticatedRoleReceipt(t)
	raw, err := json.Marshal(receipt.Receipt())
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	source := NewHeaderControlRoleReceiptSource(func(candidate []byte) (rolereceipt.AuthenticatedReceipt, error) {
		got = append([]byte(nil), candidate...)
		return receipt, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	req.Header.Set(ControlRoleReceiptHeader, base64.RawURLEncoding.EncodeToString(raw))
	gotReceipt, err := source(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) || gotReceipt.RawSHA256() != receipt.RawSHA256() {
		t.Fatalf("receipt bytes or binding changed: bytes_equal=%t digest=%q want %q", bytes.Equal(got, raw), gotReceipt.RawSHA256(), receipt.RawSHA256())
	}
}

func TestHeaderControlRoleReceiptSourceRejectsDuplicateOrMalformedHeader(t *testing.T) {
	source := NewHeaderControlRoleReceiptSource(func([]byte) (rolereceipt.AuthenticatedReceipt, error) {
		return authenticatedRoleReceipt(t), nil
	})
	duplicate := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	duplicate.Header.Add(ControlRoleReceiptHeader, "one")
	duplicate.Header.Add(ControlRoleReceiptHeader, "two")
	if _, err := source(duplicate); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("accepted duplicate role receipt headers: %v", err)
	}
	malformed := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	malformed.Header.Set(ControlRoleReceiptHeader, "not-base64")
	if _, err := source(malformed); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("accepted malformed role receipt header: %v", err)
	}
}

func TestHeaderControlRoleReceiptSourceRejectsAuthenticatorByteMismatch(t *testing.T) {
	receipt := authenticatedRoleReceipt(t)
	raw, err := json.Marshal(receipt.Receipt())
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, ' ')
	source := NewHeaderControlRoleReceiptSource(func([]byte) (rolereceipt.AuthenticatedReceipt, error) {
		return receipt, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	req.Header.Set(ControlRoleReceiptHeader, base64.RawURLEncoding.EncodeToString(raw))
	if _, err := source(req); err == nil || !strings.Contains(err.Error(), "different bytes") {
		t.Fatalf("accepted authenticator result bound to different bytes: %v", err)
	}
}

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

func TestNewReceiptBoundControlHandlerDoesNotFallBackWithoutSource(t *testing.T) {
	handler := NewReceiptBoundControlHandler(nil, "", "", ControlRolePolicy{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/control", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusServiceUnavailable)
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
