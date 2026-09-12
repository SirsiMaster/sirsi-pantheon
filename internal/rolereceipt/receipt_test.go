package rolereceipt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validReceipt(t *testing.T) Receipt {
	t.Helper()
	return Receipt{
		Schema:    Schema,
		ReceiptID: "rr-001",
		Role:      ConstrainedClient,
		HostProfile: HostProfile{
			ID: "mac-local-001", OS: "macOS 26", Toolchain: "go1.25", Transport: "tailscale",
		},
		Scope:               []string{"inspect", "message"},
		IssuedAt:            time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC),
		ExpiresAt:           time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC),
		RevocationReference: "revocations-v1",
		Issuer:              "ssa-authority",
		KeyID:               "ssa-ed25519-2026-01",
		PolicyVersion:       "role-policy-v1",
		ObservedState: ObservedState{
			RouterNamespace:    "sirsi-primary",
			ProtectedProcesses: []string{"codex"},
			ResourceFacts:      []ResourceFact{{Name: "swap_mib", Value: "1024"}},
		},
		Signature: "opaque-signature-material",
	}
}
func TestParseAndValidateForRoleReceipt(t *testing.T) {
	want := validReceipt(t)
	body, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	err = got.ValidateFor(Constraints{
		Now:           want.IssuedAt.Add(10 * time.Minute),
		Role:          ConstrainedClient,
		HostID:        want.HostProfile.ID,
		Issuer:        want.Issuer,
		KeyID:         want.KeyID,
		RequiredScope: []string{"inspect"},
	})
	if err != nil {
		t.Fatalf("ValidateFor: %v", err)
	}
}

func TestParseRejectsAmbiguousOrUnknownRoleReceiptJSON(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate key": `{"schema":"pantheon.role-receipt/v1","schema":"forged"}`,
		"unknown field": `{"schema":"pantheon.role-receipt/v1","receipt_id":"rr","role":"constrained-client","host_profile":{"id":"h","os":"o","toolchain":"t","transport":"x"},"scope":["inspect"],"issued_at":"2026-09-11T03:00:00Z","expires_at":"2026-09-11T04:00:00Z","revocation_reference":"r","issuer":"i","key_id":"k","policy_version":"p","observed_state":{"router_namespace":"n","protected_processes":[],"resource_facts":[{"name":"swap","value":"1"}]},"signature":"s","forged":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body)); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
}

func TestReceiptRejectsTemporalBindingAndInventoryDrift(t *testing.T) {
	base := validReceipt(t)
	cases := map[string]func(*Receipt, *Constraints){
		"expired":            func(_ *Receipt, c *Constraints) { c.Now = base.ExpiresAt },
		"not yet valid":      func(_ *Receipt, c *Constraints) { c.Now = base.IssuedAt.Add(-time.Second) },
		"wrong role":         func(_ *Receipt, c *Constraints) { c.Role = RouterAuthority },
		"wrong host":         func(_ *Receipt, c *Constraints) { c.HostID = "other-host" },
		"missing scope":      func(_ *Receipt, c *Constraints) { c.RequiredScope = []string{"claim"} },
		"duplicate scope":    func(r *Receipt, _ *Constraints) { r.Scope = []string{"inspect", "inspect"} },
		"one-sided handback": func(r *Receipt, _ *Constraints) { r.Predecessor = "rr-old" },
		"duplicate resource fact": func(r *Receipt, _ *Constraints) {
			r.ObservedState.ResourceFacts = append(r.ObservedState.ResourceFacts, ResourceFact{Name: "swap_mib", Value: "2048"})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			receipt := base
			receipt.Scope = append([]string(nil), base.Scope...)
			receipt.ObservedState.ProtectedProcesses = append([]string(nil), base.ObservedState.ProtectedProcesses...)
			receipt.ObservedState.ResourceFacts = append([]ResourceFact(nil), base.ObservedState.ResourceFacts...)
			constraints := Constraints{Now: base.IssuedAt.Add(time.Minute), Role: base.Role, HostID: base.HostProfile.ID, RequiredScope: []string{"inspect"}}
			mutate(&receipt, &constraints)
			if err := receipt.ValidateFor(constraints); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
}

func TestCanonicalScopeTrimsAndSortsWithoutAuthorityClaim(t *testing.T) {
	got := CanonicalScope([]string{" message ", "inspect"})
	if strings.Join(got, ",") != "inspect,message" {
		t.Fatalf("CanonicalScope = %q", got)
	}
}
