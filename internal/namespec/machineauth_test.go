package namespec

import (
	"strings"
	"testing"
)

func TestAuthorizeMachineClaim_AllowsMatchingCredential(t *testing.T) {
	err := AuthorizeMachineClaim("m1", RegisteringSessionCredential{MachineID: "uuid-m1"}, "uuid-m1")
	if err != nil {
		t.Fatalf("matching session machine-id should be allowed, got: %v", err)
	}
}

func TestAuthorizeMachineClaim_CrossHostSpoofRefused(t *testing.T) {
	// An m5 session's credentialed machine-id never matches m1's bound
	// machine-id, no matter what alias string it requests.
	err := AuthorizeMachineClaim("m1", RegisteringSessionCredential{MachineID: "uuid-m5"}, "uuid-m1")
	if err == nil || !strings.Contains(err.Error(), "cross-host claim refused") {
		t.Fatalf("session on m5 claiming m1 must be refused as a cross-host claim, got: %v", err)
	}
}

func TestAuthorizeMachineClaim_RevokedCredentialRefused(t *testing.T) {
	err := AuthorizeMachineClaim("m1", RegisteringSessionCredential{MachineID: "uuid-m1", Revoked: true}, "uuid-m1")
	if err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked credential must be refused and named, got: %v", err)
	}
}

func TestAuthorizeMachineClaim_EmptySessionMachineIDRefused(t *testing.T) {
	err := AuthorizeMachineClaim("m1", RegisteringSessionCredential{}, "uuid-m1")
	if err == nil {
		t.Fatal("an empty session machine-id must be refused, hostname text alone carries no authority")
	}
}

func TestAuthorizeMachineClaim_UnboundAliasRefused(t *testing.T) {
	err := AuthorizeMachineClaim("m9", RegisteringSessionCredential{MachineID: "uuid-m9"}, "")
	if err == nil || !strings.Contains(err.Error(), "never adopted") {
		t.Fatalf("an alias with no machine-id bound on file must fail closed, got: %v", err)
	}
}

func TestAuthorizeMachineClaim_ServicePrincipalNeverSubstitutesForSession(t *testing.T) {
	// The router service's own outbound credential (used for its schema-
	// promotion/machine-id-verification hop) is a distinct type and cannot be
	// passed as the registering session's credential -- this is enforced by
	// the type system (ServicePrincipal vs RegisteringSessionCredential), not
	// a runtime check. This test documents the scenario: even though the
	// service authenticates as m1 for its own outbound call, the decision is
	// made from the REGISTERING session's credential, which is m5 here.
	servicesOwnPrincipal := ServicePrincipal{MachineID: "uuid-m1"}
	registeringSession := RegisteringSessionCredential{MachineID: "uuid-m5"}

	err := AuthorizeMachineClaim("m1", registeringSession, "uuid-m1")
	if err == nil {
		t.Fatal("the service's own m1 principal must not grant an m1 name to a session registering from m5")
	}
	_ = servicesOwnPrincipal // held only as the service's own hop credential, never passed above
}
