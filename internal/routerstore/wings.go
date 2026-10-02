package routerstore

// ADR-066 (rs-31a): Stack Lab wing admission — persistence layer only.
//
// This file deliberately stops at schema validation + atomic persist with
// idempotency-on-identical-bytes / conflicting-identity-rejection semantics.
// Caller-authority binding (rs-31b: binding the caller to an INDEPENDENTLY
// established project/repo authority) and canonical-path containment
// enforcement (rs-31c) are separate, explicitly named sub-builds layered on
// top of RegisterWing — see the task's own split in the ledger
// (rs-31-wing-schema-enforcement) and
// docs/continuations/ra-rs31-wing-register-scoping-20261002-f06bcae5.md for
// the reuse inventory this was built from.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stacklab"
)

// WingReceipt is what RegisterWing returns: the admitted identity plus the
// digest a caller can use to prove admission happened, per the task's own
// requirement ("return wing id/project_id/router_namespace/digest/receipt").
type WingReceipt struct {
	WingID          string `json:"wing_id"`
	ProjectID       string `json:"project_id"`
	RouterNamespace string `json:"router_namespace"`
	ContentHash     string `json:"content_hash"`
	Created         string `json:"created"`
}

// ErrWingConflict: the wing id is already admitted under a DIFFERENT record
// (content hash mismatch). No silent overwrite — an authorized,
// version-checked update path is explicitly a later build, not this one.
var ErrWingConflict = errors.New("routerstore: wing id already admitted under a different record")

// RegisterWing schema-validates raw against contracts/stacklab/v2/wing.schema.json
// (via stacklab.ValidateWing, reused rather than re-implemented) and persists
// it atomically, keyed by wing id.
//
// Idempotent on identical bytes: registering the same wing id with the same
// content hash returns the existing receipt rather than erroring or
// duplicating the row. A conflicting identity (same id, different bytes)
// returns ErrWingConflict — never a silent overwrite.
func (s *SQLiteStore) RegisterWing(raw []byte) (WingReceipt, error) {
	rec, err := stacklab.ValidateWing(raw)
	if err != nil {
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: %w", err)
	}
	hash := stacklab.ContentSHA256(raw)

	if receipt, found, err := s.lookupWing(rec.ID, hash); err != nil {
		return WingReceipt{}, err
	} else if found {
		return receipt, nil
	}

	now := s.clock().Format(time.RFC3339)
	_, err = s.exec(
		`INSERT INTO wings(wing_id,project_id,router_namespace,owner,content_hash,record_json,created,updated) VALUES(?,?,?,?,?,?,?,?)`,
		rec.ID, rec.ProjectID, rec.RouterNamespace, rec.Owner, hash, string(raw), now, now,
	)
	if err == nil {
		return WingReceipt{
			WingID:          rec.ID,
			ProjectID:       rec.ProjectID,
			RouterNamespace: rec.RouterNamespace,
			ContentHash:     hash,
			Created:         now,
		}, nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: insert: %w", err)
	}
	// Lost a race with a concurrent registration of the same wing id: re-read
	// and resolve exactly as the pre-check above would have.
	receipt, found, lookupErr := s.lookupWing(rec.ID, hash)
	if lookupErr != nil {
		return WingReceipt{}, lookupErr
	}
	if !found {
		// The unique-constraint row that caused the insert to fail is gone by
		// the time of this re-read (e.g. raced with a since-reverted insert).
		// Surface the original insert error rather than mis-reporting ErrWingConflict.
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: insert: %w", err)
	}
	return receipt, nil
}

// lookupWing resolves an existing row for wingID: found=false means no row
// exists (caller should insert). A row with a different content hash than
// hash returns ErrWingConflict directly, so every caller gets the same
// conflict behavior.
func (s *SQLiteStore) lookupWing(wingID, hash string) (WingReceipt, bool, error) {
	var out WingReceipt
	var existingHash string
	err := s.db.QueryRow(`SELECT wing_id,project_id,router_namespace,content_hash,created FROM wings WHERE wing_id=?`, wingID).
		Scan(&out.WingID, &out.ProjectID, &out.RouterNamespace, &existingHash, &out.Created)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return WingReceipt{}, false, nil
	case err != nil:
		return WingReceipt{}, false, fmt.Errorf("routerstore: RegisterWing: lookup: %w", err)
	case existingHash != hash:
		return WingReceipt{}, false, ErrWingConflict
	default:
		out.ContentHash = existingHash
		return out, true, nil
	}
}
