package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

func roleReceiptFixture(t *testing.T) []byte {
	t.Helper()
	receipt := rolereceipt.Receipt{
		Schema:    rolereceipt.Schema,
		ReceiptID: "rr-cli",
		Role:      rolereceipt.ConstrainedClient,
		HostProfile: rolereceipt.HostProfile{
			ID: "host-1", OS: "macOS", Toolchain: "go", Transport: "tailscale",
		},
		Scope:               []string{"inspect"},
		IssuedAt:            time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC),
		ExpiresAt:           time.Date(2026, 9, 11, 5, 0, 0, 0, time.UTC),
		RevocationReference: "revocations-v1",
		Issuer:              "ssa",
		KeyID:               "key-1",
		PolicyVersion:       "v1",
		ObservedState: rolereceipt.ObservedState{
			RouterNamespace: "sirsi", ProtectedProcesses: []string{}, ResourceFacts: []rolereceipt.ResourceFact{{Name: "swap_mib", Value: "1024"}},
		},
		Signature: "opaque",
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func TestRoleReceiptValidationOutputIsRedactedAndExplicitlyUnauthenticated(t *testing.T) {
	var receipt rolereceipt.Receipt
	if err := json.Unmarshal(roleReceiptFixture(t), &receipt); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(roleReceiptValidationOutput{Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "opaque") || strings.Contains(string(body), `"signature"`) {
		t.Fatalf("validation output leaked opaque signature material: %s", body)
	}
	if !strings.Contains(string(body), `"signature_authentication":"not_performed_trust_root_required"`) {
		t.Fatalf("validation output did not state its authentication boundary: %s", body)
	}
}

func TestRoleReceiptCLIHelpersValidateBoundedInput(t *testing.T) {
	body := roleReceiptFixture(t)
	got, err := readRoleReceipt(bytes.NewReader(body))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("readRoleReceipt = %q, %v", got, err)
	}
	constraints, err := roleReceiptConstraints("constrained-client", "host-1", "ssa", "key-1", []string{"inspect"}, "2026-09-11T04:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := rolereceipt.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.ValidateFor(constraints); err != nil {
		t.Fatal(err)
	}
}

func TestRoleReceiptCLIHelpersRejectBadClockAndOversizeInput(t *testing.T) {
	if _, err := roleReceiptConstraints("", "", "", "", nil, "not-a-clock"); err == nil || !strings.Contains(err.Error(), "parse --now") {
		t.Fatalf("invalid clock accepted: %v", err)
	}
	if _, err := readRoleReceipt(bytes.NewReader(make([]byte, maxRoleReceiptBytes+1))); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized receipt accepted: %v", err)
	}
}
