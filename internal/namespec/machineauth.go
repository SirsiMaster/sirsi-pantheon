package namespec

import "fmt"

// MachineID is a credentialed ADR-067 machine-id. It has its own type so a
// caller cannot pass a bare string -- the two machine-id values namespec ever
// handles (the registering session's and the router service's own outbound
// principal) must never be interchangeable by accident (ADR-072 C3, SSA round
// 2: "registering session vs. outbound service principal are never
// substituted for each other").
type MachineID string

// RegisteringSessionCredential is the authenticated registering session's
// credentialed machine-id (ADR-067), as resolved by the caller -- the router
// service, from its own session authentication, never from client-supplied
// hostname text. namespec has no access to the credential store itself and
// trusts only what the caller supplies here (same separation as
// OfflineCredential for C2).
type RegisteringSessionCredential struct {
	MachineID MachineID
	Revoked   bool
}

// ServicePrincipal is the router service's OWN outbound machine-id credential
// -- used only when the service itself makes a remote call during
// registration (schema-promotion lookup, machine-id verification against a
// central authority). It is a distinct type from RegisteringSessionCredential
// so the service's own identity can never be passed where the registering
// session's is required: a request authenticated as the M1 service does not
// thereby grant an M1 name to a session running on M5.
type ServicePrincipal struct {
	MachineID MachineID
}

// AuthorizeMachineClaim is the ADR-072 C3 decision: whether the registering
// session, authenticated as cred, may be granted a name whose machine alias
// is claimedAlias, given boundMachineID -- the machine-id the ADR-067
// credential store has on file as authorized for that alias. namespec never
// resolves boundMachineID itself; the caller supplies it (resolved from the
// registering session's own credential store lookup, not the service's).
//
// It fails closed: a revoked or absent session credential, or an alias with
// no machine-id bound on file (never adopted), refuses. A session whose own
// machine-id does not match the alias's bound machine-id is refused -- this
// is what stops a session on m5 from claiming an m1 name no matter what
// string it requests; hostname text alone carries no authority.
func AuthorizeMachineClaim(claimedAlias string, cred RegisteringSessionCredential, boundMachineID MachineID) error {
	if cred.Revoked {
		return fmt.Errorf("namespec: registering session's machine-id credential is revoked -- refused, distinct from an unbound-alias refusal")
	}
	if cred.MachineID == "" {
		return fmt.Errorf("namespec: registering session presented no credentialed machine-id")
	}
	if boundMachineID == "" {
		return fmt.Errorf("namespec: machine alias %q has no machine-id bound on file (never adopted)", claimedAlias)
	}
	if cred.MachineID != boundMachineID {
		return fmt.Errorf("namespec: registering session's machine-id does not match the credential bound to machine alias %q (cross-host claim refused)", claimedAlias)
	}
	return nil
}
