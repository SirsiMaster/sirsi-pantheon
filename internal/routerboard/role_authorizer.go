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

func (h *Handler) authorizeControlRole(ctx context.Context, operation string) error {
	if !h.requireControlRole {
		return nil
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return fmt.Errorf("control role authorization requires an operation")
	}
	if h.controlRoleAuthorizer == nil {
		return fmt.Errorf("control role authorization is not configured")
	}
	granted, err := h.controlRoleAuthorizer(ctx, operation)
	if err != nil {
		return fmt.Errorf("control role authorization for %q: %w", operation, err)
	}
	if err := granted.Authorizes(operation, time.Now().UTC()); err != nil {
		return fmt.Errorf("control role authorization for %q: %w", operation, err)
	}
	return nil
}
