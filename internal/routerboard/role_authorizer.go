package routerboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

// ControlRoleAuthorizer is the trust-root integration seam for the worker
// plane. The callback must authenticate the externally issued role receipt and
// authorize the exact operation before the handler reads or mutates state.
// Pantheon deliberately does not provide a default implementation: a missing
// trust root must fail closed rather than become an implicit local role.
type ControlRoleAuthorizer func(context.Context, string) (rolereceipt.AuthenticatedReceipt, error)

// ControlRolePolicy is the local consumer policy applied after the external
// trust root authenticates a receipt. Role is normally constrained-client for
// M1 requests; host, issuer, and key can be pinned when the deployment has
// those independently governed identities.
type ControlRolePolicy = rolereceipt.Constraints

type controlRoleReceiptContextKey struct{}

// WithAuthenticatedControlRole binds an externally authenticated receipt to
// the request context. An ingress verifier may use this seam after checking
// the raw receipt against its governed trust root and revocation policy.
func WithAuthenticatedControlRole(ctx context.Context, receipt rolereceipt.AuthenticatedReceipt) context.Context {
	return context.WithValue(ctx, controlRoleReceiptContextKey{}, receipt)
}

// AuthenticatedControlRoleFromContext retrieves the exact receipt bound by an
// ingress verifier. Absence is distinct from an unbound zero receipt.
func AuthenticatedControlRoleFromContext(ctx context.Context) (rolereceipt.AuthenticatedReceipt, bool) {
	receipt, ok := ctx.Value(controlRoleReceiptContextKey{}).(rolereceipt.AuthenticatedReceipt)
	return receipt, ok
}

func (h *Handler) authorizeControlRole(ctx context.Context, operation string) error {
	if !h.requireControlRole {
		return nil
	}
	operation = strings.TrimSpace(operation)
	if err := ValidateControlRoleOperation(operation); err != nil {
		return fmt.Errorf("control role authorization requires an operation: %w", err)
	}
	if h.controlRoleAuthorizer == nil {
		return fmt.Errorf("control role authorization is not configured")
	}
	granted, err := h.controlRoleAuthorizer(ctx, operation)
	if err != nil {
		return fmt.Errorf("control role authorization for %q: %w", operation, err)
	}
	if bound, ok := AuthenticatedControlRoleFromContext(ctx); ok {
		if bound.RawSHA256() == "" || granted.RawSHA256() == "" || bound.RawSHA256() != granted.RawSHA256() {
			return fmt.Errorf("control role authorization for %q: receipt is not bound to the request context", operation)
		}
		granted = bound
	}
	if err := granted.AuthorizesWith(operation, time.Now().UTC(), h.controlRolePolicy); err != nil {
		return fmt.Errorf("control role authorization for %q: %w", operation, err)
	}
	return nil
}
