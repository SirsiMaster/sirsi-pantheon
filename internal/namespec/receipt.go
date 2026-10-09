package namespec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// PromotionReceipt binds a promoted schema version+hash to the operator and
// moment that activated it (ADR-072 C2). The router never trusts a
// caller-presented schema_hash on its own; it is meaningful only alongside a
// receipt the service itself wrote at promotion time.
type PromotionReceipt struct {
	SchemaVersion int       `json:"schema_version"`
	SchemaHash    string    `json:"schema_hash"`
	PromotedBy    string    `json:"promoted_by"`
	PromotedAt    time.Time `json:"promoted_at"`
}

// LoadPromotionReceipt parses and sanity-checks a promotion receipt. A
// receipt missing any field proves nothing, so it is a load error rather
// than a permissive zero value.
func LoadPromotionReceipt(raw []byte) (PromotionReceipt, error) {
	var r PromotionReceipt
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return PromotionReceipt{}, fmt.Errorf("namespec: parse promotion receipt: %w", err)
	}
	if r.SchemaVersion < 1 {
		return PromotionReceipt{}, fmt.Errorf("namespec: promotion receipt has no schema_version")
	}
	if r.SchemaHash == "" {
		return PromotionReceipt{}, fmt.Errorf("namespec: promotion receipt has no schema_hash")
	}
	if r.PromotedBy == "" {
		return PromotionReceipt{}, fmt.Errorf("namespec: promotion receipt has no promoted_by")
	}
	if r.PromotedAt.IsZero() {
		return PromotionReceipt{}, fmt.Errorf("namespec: promotion receipt has no promoted_at")
	}
	return r, nil
}

// ValidateAgainstActive is the C2 trust check: it compares a presented
// (schema_version, schema_hash) pair against the service's own active
// promotion receipt, which is the only source of truth. It rejects an
// unknown (newer-than-active) version, an unauthorized downgrade (older than
// active), and a hash mismatch at the active version (working-tree
// divergence, A37) -- never the caller's claim alone.
func (active PromotionReceipt) ValidateAgainstActive(presentedVersion int, presentedHash string) error {
	if presentedVersion < active.SchemaVersion {
		return fmt.Errorf("namespec: schema version %d is older than the active promoted version %d (unauthorized downgrade)", presentedVersion, active.SchemaVersion)
	}
	if presentedVersion > active.SchemaVersion {
		return fmt.Errorf("namespec: schema version %d is unknown (active promoted version is %d)", presentedVersion, active.SchemaVersion)
	}
	if presentedHash != active.SchemaHash {
		return fmt.Errorf("namespec: schema hash %q does not match the active promoted hash for version %d (working-tree divergence)", presentedHash, presentedVersion)
	}
	return nil
}
