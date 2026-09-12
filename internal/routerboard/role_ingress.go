package routerboard

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

const ControlRoleReceiptHeader = "X-Sirsi-Role-Receipt"

// ControlRoleReceiptAuthenticator is the external trust-root boundary for a
// wire receipt. It must authenticate the decoded bytes against the governed
// issuer keyring and revocation state; this package never treats parsing as
// authentication.
type ControlRoleReceiptAuthenticator func([]byte) (rolereceipt.AuthenticatedReceipt, error)

// NewHeaderControlRoleReceiptSource adapts the canonical role receipt header
// to the receipt-bound HTTP handler. The value is base64url-encoded JSON so
// transport intermediaries cannot reinterpret JSON punctuation. Exactly one
// bounded header is required, and the supplied authenticator remains the sole
// authority for signature, issuer, and revocation decisions.
func NewHeaderControlRoleReceiptSource(authenticator ControlRoleReceiptAuthenticator) ControlRoleReceiptSource {
	return func(r *http.Request) (rolereceipt.AuthenticatedReceipt, error) {
		if authenticator == nil {
			return rolereceipt.AuthenticatedReceipt{}, errors.New("control role receipt authenticator is not configured")
		}
		if r == nil {
			return rolereceipt.AuthenticatedReceipt{}, errors.New("control role receipt request is nil")
		}
		values := r.Header.Values(ControlRoleReceiptHeader)
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return rolereceipt.AuthenticatedReceipt{}, errors.New("control role receipt header must contain exactly one value")
		}
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(values[0]))
		if err != nil {
			return rolereceipt.AuthenticatedReceipt{}, fmt.Errorf("decode control role receipt header: %w", err)
		}
		if len(raw) == 0 || len(raw) > 64*1024 {
			return rolereceipt.AuthenticatedReceipt{}, errors.New("control role receipt header exceeds the 64 KiB envelope limit")
		}
		receipt, err := authenticator(raw)
		if err != nil {
			return rolereceipt.AuthenticatedReceipt{}, fmt.Errorf("authenticate control role receipt header: %w", err)
		}
		sum := sha256.Sum256(raw)
		if receipt.RawSHA256() == "" {
			return rolereceipt.AuthenticatedReceipt{}, errors.New("control role receipt authenticator returned an unbound receipt")
		}
		if receipt.RawSHA256() != hex.EncodeToString(sum[:]) {
			return rolereceipt.AuthenticatedReceipt{}, errors.New("control role receipt authenticator returned a receipt for different bytes")
		}
		return receipt, nil
	}
}

// NewReceiptBoundControlHandler composes the canonical role-policy handler
// with the external receipt ingress boundary. Supplying no source intentionally
// yields a fail-closed HTTP handler; it never falls back to bearer-only access.
func NewReceiptBoundControlHandler(b *Board, dir, token string, policy ControlRolePolicy, source ControlRoleReceiptSource) http.Handler {
	handler := NewHandlerWithControlAuthAndRolePolicy(b, dir, token, policy, ContextControlRoleAuthorizer)
	return WithControlRoleReceiptSource(handler, source)
}

// NewHeaderReceiptBoundControlHandler is the canonical M5 composition point
// for HTTP deployments that transport the external role receipt in the
// standard header. A nil authenticator deliberately produces a handler that
// denies every request; it never downgrades to bearer-only control.
func NewHeaderReceiptBoundControlHandler(b *Board, dir, token string, policy ControlRolePolicy, authenticator ControlRoleReceiptAuthenticator) http.Handler {
	return NewReceiptBoundControlHandler(b, dir, token, policy, NewHeaderControlRoleReceiptSource(authenticator))
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
