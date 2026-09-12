package routerboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

// ControlRoleReceiptSource is the external trust-root boundary for HTTP
// ingress. It must authenticate the exact request-bound receipt, including
// issuer, signature, revocation, and transport policy, before returning it.
// Pantheon does not provide a default source or read private key material.
type ControlRoleReceiptSource func(*http.Request) (rolereceipt.AuthenticatedReceipt, error)

// NewReceiptBoundControlHandler composes the canonical role-policy handler
// with the external receipt ingress boundary. Supplying no source intentionally
// yields a fail-closed HTTP handler; it never falls back to bearer-only access.
func NewReceiptBoundControlHandler(b *Board, dir, token string, policy ControlRolePolicy, source ControlRoleReceiptSource) http.Handler {
	handler := NewHandlerWithControlAuthAndRolePolicy(b, dir, token, policy, ContextControlRoleAuthorizer)
	return WithControlRoleReceiptSource(handler, source)
}

// ContextControlRoleAuthorizer adapts an ingress-bound receipt to the handler's
// operation callback. It never treats a missing context value as local access.
func ContextControlRoleAuthorizer(ctx context.Context, _ string) (rolereceipt.AuthenticatedReceipt, error) {
	receipt, ok := AuthenticatedControlRoleFromContext(ctx)
	if !ok || receipt.RawSHA256() == "" {
		return rolereceipt.AuthenticatedReceipt{}, errors.New("authenticated control role is missing from request context")
	}
	return receipt, nil
}

// WithControlRoleReceiptSource creates the explicit HTTP ingress boundary for
// the receipt-bound worker plane. A missing source or source failure is a
// visible denial; the wrapped handler is never reached without a receipt.
func WithControlRoleReceiptSource(next http.Handler, source ControlRoleReceiptSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if next == nil {
			http.Error(w, `{"error":"control role handler is not configured"}`, http.StatusServiceUnavailable)
			return
		}
		if source == nil {
			http.Error(w, `{"error":"control role receipt source is not configured"}`, http.StatusServiceUnavailable)
			return
		}
		receipt, err := source(r)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "control role receipt rejected: "+err.Error()), http.StatusForbidden)
			return
		}
		if receipt.RawSHA256() == "" {
			http.Error(w, `{"error":"control role receipt is unbound"}`, http.StatusForbidden)
			return
		}
		ctx := WithAuthenticatedControlRole(r.Context(), receipt)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ValidateControlRoleOperation is a small ingress-side guard for adapters
// that authorize before invoking the handler. The handler still performs its
// own operation, policy, and temporal recheck at the point of use.
func ValidateControlRoleOperation(operation string) error {
	if strings.TrimSpace(operation) == "" {
		return errors.New("control role operation is required")
	}
	return nil
}
