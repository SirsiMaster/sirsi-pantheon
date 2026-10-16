package namespec

import (
	"strings"
	"testing"
	"time"
)

func validLastVerifiedReceipt() *PromotionReceipt {
	return &PromotionReceipt{
		SchemaVersion: 1,
		SchemaHash:    "abc123",
		PromotedBy:    "owner",
		PromotedAt:    time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC),
	}
}

func TestAuthorizeOffline_ReadOnlyAllowedWithValidReceiptAndCredential(t *testing.T) {
	err := AuthorizeOffline(validLastVerifiedReceipt(), OfflineCredential{}, OpReadOnly)
	if err != nil {
		t.Fatalf("read-only op with a valid receipt and unrevoked credential should be allowed, got: %v", err)
	}
}

func TestAuthorizeOffline_MutatingAlwaysRefusedEvenWithValidReceiptAndCredential(t *testing.T) {
	err := AuthorizeOffline(validLastVerifiedReceipt(), OfflineCredential{}, OpMutating)
	if err == nil || !strings.Contains(err.Error(), "never authorized while disconnected") {
		t.Fatalf("mutating op should be refused unconditionally while disconnected, got: %v", err)
	}
}

func TestAuthorizeOffline_TwoDisconnectedHostsBothRefusedOnMutatingOp(t *testing.T) {
	// Each host presents its own last-verified receipt; neither is ever
	// upgraded into mutation authority, regardless of how it was obtained.
	hostA := &PromotionReceipt{SchemaVersion: 1, SchemaHash: "hash-a", PromotedBy: "owner", PromotedAt: time.Now()}
	hostB := &PromotionReceipt{SchemaVersion: 2, SchemaHash: "hash-b", PromotedBy: "owner", PromotedAt: time.Now()}

	if err := AuthorizeOffline(hostA, OfflineCredential{}, OpMutating); err == nil {
		t.Error("host A's mutating attempt should be refused locally")
	}
	if err := AuthorizeOffline(hostB, OfflineCredential{}, OpMutating); err == nil {
		t.Error("host B's mutating attempt should be refused locally")
	}
}

func TestAuthorizeOffline_MissingReceiptFailsClosedForBothOperationKinds(t *testing.T) {
	for _, op := range []OperationKind{OpReadOnly, OpMutating} {
		if err := AuthorizeOffline(nil, OfflineCredential{}, op); err == nil {
			t.Errorf("%s op with no last-verified receipt should fail closed, got no error", op)
		}
	}
}

func TestAuthorizeOffline_RevokedCredentialRefusedDistinctlyFromStaleSchema(t *testing.T) {
	err := AuthorizeOffline(validLastVerifiedReceipt(), OfflineCredential{Revoked: true}, OpReadOnly)
	if err == nil {
		t.Fatal("a revoked machine-id credential should refuse even a read-only offline operation")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked-credential refusal must name the credential, not just the schema, got: %v", err)
	}
	if strings.Contains(err.Error(), "downgrade") || strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "divergence") {
		t.Fatalf("revoked-credential refusal must be distinct from a stale-schema refusal, got: %v", err)
	}
}

func TestAuthorizeOffline_ReconnectReplayUsesTheNormalPathNotOfflineBypass(t *testing.T) {
	// Once reachable, a queued request is validated by ValidateAgainstActive
	// like any other request -- there is no "it was queued while offline"
	// exemption. A presented version/hash that no longer matches the active
	// promotion (e.g. superseded while the host was disconnected) is
	// rejected exactly as a live request would be.
	active := PromotionReceipt{SchemaVersion: 3, SchemaHash: "hash-v3"}
	staleFromWhileDisconnected := validLastVerifiedReceipt() // v1, "abc123"

	err := active.ValidateAgainstActive(staleFromWhileDisconnected.SchemaVersion, staleFromWhileDisconnected.SchemaHash)
	if err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("a replayed request against a superseded schema must be rejected by the normal path, got: %v", err)
	}
}
