package namespec

import (
	"strings"
	"testing"
	"time"
)

func TestLoadPromotionReceipt(t *testing.T) {
	good := `{"schema_version":1,"schema_hash":"abc123","promoted_by":"owner","promoted_at":"2026-10-09T00:00:00Z"}`
	r, err := LoadPromotionReceipt([]byte(good))
	if err != nil {
		t.Fatalf("LoadPromotionReceipt: %v", err)
	}
	if r.SchemaVersion != 1 || r.SchemaHash != "abc123" || r.PromotedBy != "owner" {
		t.Fatalf("unexpected receipt: %+v", r)
	}
	if !r.PromotedAt.Equal(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected promoted_at: %v", r.PromotedAt)
	}
}

func TestLoadPromotionReceiptRejections(t *testing.T) {
	cases := map[string]string{
		"no version":     `{"schema_hash":"a","promoted_by":"o","promoted_at":"2026-10-09T00:00:00Z"}`,
		"no hash":        `{"schema_version":1,"promoted_by":"o","promoted_at":"2026-10-09T00:00:00Z"}`,
		"no promoted_by": `{"schema_version":1,"schema_hash":"a","promoted_at":"2026-10-09T00:00:00Z"}`,
		"no promoted_at": `{"schema_version":1,"schema_hash":"a","promoted_by":"o"}`,
		"unknown field":  `{"schema_version":1,"schema_hash":"a","promoted_by":"o","promoted_at":"2026-10-09T00:00:00Z","extra":true}`,
		"malformed json": `{not json`,
	}
	for name, raw := range cases {
		if _, err := LoadPromotionReceipt([]byte(raw)); err == nil {
			t.Errorf("%s: expected rejection, got none", name)
		}
	}
}

func TestValidateAgainstActive(t *testing.T) {
	active := PromotionReceipt{SchemaVersion: 2, SchemaHash: "hash-v2"}

	if err := active.ValidateAgainstActive(2, "hash-v2"); err != nil {
		t.Errorf("matching version+hash should pass, got: %v", err)
	}

	if err := active.ValidateAgainstActive(1, "hash-v1"); err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Errorf("stale version should be rejected as a downgrade, got: %v", err)
	}

	if err := active.ValidateAgainstActive(3, "hash-v3"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("newer-than-active version should be rejected as unknown, got: %v", err)
	}

	if err := active.ValidateAgainstActive(2, "wrong-hash"); err == nil || !strings.Contains(err.Error(), "divergence") {
		t.Errorf("hash mismatch at the active version should be rejected as divergence, got: %v", err)
	}
}
