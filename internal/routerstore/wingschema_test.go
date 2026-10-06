package routerstore

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// pinnedWingSchemaHash is SNE's stated authority for contracts/stacklab/v2/
// wing.schema.json (router item 20260912-032526; see
// contracts/stacklab/v2/PROVENANCE.md). Pantheon vendors an exact byte copy —
// SNE remains schema authority, this repo's copy is a reproducible dependency,
// never a separately editable contract. A future schema change requires an
// explicit newly-pinned SNE handoff, which means updating THIS constant in the
// same change that updates the vendored file — never editing the file alone.
const pinnedWingSchemaHash = "a69e0094b8ec4596c30b316fc8bd6ce5f806f5a801b5c80c10a972e3d86338ad"

// TestVendoredWingSchemaMatchesPinnedHash fails closed (per SNE's explicit
// instruction) if the vendored contract goes missing or its bytes drift from
// the pinned hash — no normalization, raw bytes only. This is a build-time
// integrity check on the vendored dependency; it does not exercise schema
// validation or admission logic (rs-31a, not yet implemented).
func TestVendoredWingSchemaMatchesPinnedHash(t *testing.T) {
	b, err := os.ReadFile("../../contracts/stacklab/v2/wing.schema.json")
	if err != nil {
		t.Fatalf("vendored wing.schema.json missing or unreadable: %v", err)
	}
	sum := sha256.Sum256(b)
	got := hex.EncodeToString(sum[:])
	if got != pinnedWingSchemaHash {
		t.Fatalf("vendored wing.schema.json hash = %s, want pinned %s (drift from SNE's authority — fail closed, never silently accept)", got, pinnedWingSchemaHash)
	}
}
