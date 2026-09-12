package routerboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
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
		controlRoleAuthorizer: func(_ context.Context, operation string) (rolereceipt.AuthenticatedReceipt, error) {
			got = operation
			return authenticatedRoleReceipt(t), nil
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
	h := NewHandlerWithControlAuthAndRole(nil, "", "token", func(_ context.Context, operation string) (rolereceipt.AuthenticatedReceipt, error) {
		if operation != "inspect" {
			t.Fatalf("operation = %q, want inspect", operation)
		}
		return rolereceipt.AuthenticatedReceipt{}, errors.New("expired role")
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

type roleReceiptTestAuthenticator struct{}

func (roleReceiptTestAuthenticator) VerifyRoleReceipt([]byte, rolereceipt.Receipt) error { return nil }

func authenticatedRoleReceipt(t *testing.T) rolereceipt.AuthenticatedReceipt {
	t.Helper()
	now := time.Now().UTC()
	receipt := rolereceipt.Receipt{
		Schema:    rolereceipt.Schema,
		ReceiptID: "rr-routerboard-test",
		Role:      rolereceipt.ConstrainedClient,
		HostProfile: rolereceipt.HostProfile{
			ID: "m1-test", OS: "macOS", Toolchain: "go", Transport: "tailscale",
		},
		Scope:               []string{"result_return", "inspect"},
		IssuedAt:            now.Add(-time.Minute),
		ExpiresAt:           now.Add(time.Minute),
		RevocationReference: "revocations-v1",
		Issuer:              "ssa",
		KeyID:               "test-key",
		PolicyVersion:       "v1",
		ObservedState: rolereceipt.ObservedState{
			RouterNamespace:    "sirsi-primary",
			ProtectedProcesses: []string{"codex"},
			ResourceFacts:      []rolereceipt.ResourceFact{{Name: "swap", Value: "1"}},
		},
		Signature: "test-signature",
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := rolereceipt.AuthenticateAndValidate(raw, rolereceipt.Constraints{
		Now: now, RequiredScope: []string{"inspect", "result_return"},
	}, roleReceiptTestAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func TestControlRoleAuthorizerRejectsUnboundReceipt(t *testing.T) {
	h := &Handler{
		requireControlRole: true,
		controlRoleAuthorizer: func(context.Context, string) (rolereceipt.AuthenticatedReceipt, error) {
			return rolereceipt.AuthenticatedReceipt{}, nil
		},
	}
	if err := h.authorizeControlRole(context.Background(), "inspect"); err == nil {
		t.Fatal("unbound receipt was accepted")
	}
}

func TestControlRoleAuthorizerRejectsReceiptForWrongRole(t *testing.T) {
	receipt := authenticatedRoleReceipt(t)
	h := &Handler{
		requireControlRole: true,
		controlRolePolicy:  ControlRolePolicy{Role: rolereceipt.RouterAuthority},
		controlRoleAuthorizer: func(context.Context, string) (rolereceipt.AuthenticatedReceipt, error) {
			return receipt, nil
		},
	}
	if err := h.authorizeControlRole(context.Background(), "inspect"); err == nil {
		t.Fatal("receipt for the wrong role was accepted")
	}
}
