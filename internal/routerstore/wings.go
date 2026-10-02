package routerstore

// ADR-066 (rs-31a/b/c): Stack Lab wing admission.
//
// rs-31a: schema validation + atomic persist with idempotency-on-identical-
// bytes / conflicting-identity-rejection semantics.
//
// rs-31b/c (wingauthority.go): RegisterWing's principal argument is the
// caller identity the CLI already resolves via resolveCurrentAgent (the same
// binding AckItem/respond use) — never the record's self-declared "owner"
// field, which codex-apollo's SNE disposition on item 20261002-211532 named
// explicitly as unable to establish authority on its own. principal must
// hold an active wingauthority.go grant for (rec.ProjectID,
// rec.RouterNamespace) whose roots canonically contain every workspace root
// the record claims (repository_root, each writable_root, evidence_root) —
// component-wise, symlink-resolved, never a string prefix. The grant check
// and the insert run in one transaction so a grant revoked between check and
// write cannot admit (the validate-then-use race Apollo's disposition names).
//
// allowed_peer_wings is handoffs metadata only: it is never read here and
// cannot grant another wing anything (Apollo: "a submitted allowed_peer_wings
// list cannot grant another wing permission").

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
// (via stacklab.ValidateWing, reused rather than re-implemented), checks that
// principal holds an active wing-authority grant covering every workspace
// root the record claims, and persists it atomically, keyed by wing id.
//
// Idempotent on identical bytes: registering the same wing id with the same
// content hash returns the existing receipt rather than erroring or
// duplicating the row. A conflicting identity (same id, different bytes)
// returns ErrWingConflict — never a silent overwrite.
func (s *SQLiteStore) RegisterWing(principal string, raw []byte) (WingReceipt, error) {
	rec, err := stacklab.ValidateWing(raw)
	if err != nil {
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: %w", err)
	}
	if strings.TrimSpace(principal) == "" {
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: principal is required")
	}
	hash := stacklab.ContentSHA256(raw)

	tx, err := s.db.Begin()
	if err != nil {
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if receipt, found, err := lookupWingTx(tx, rec.ID, hash); err != nil {
		return WingReceipt{}, err
	} else if found {
		// Idempotent re-register of bytes already admitted under this id:
		// still requires the grant to exist and still be active (a revoked
		// principal cannot keep "re-admitting" its own prior record), but
		// does not need to re-check containment against a fresh grant since
		// nothing about the record is changing.
		if _, err := activeWingGrantTx(tx, principal, rec.ProjectID, rec.RouterNamespace); err != nil {
			return WingReceipt{}, err
		}
		return receipt, tx.Commit()
	}

	grant, err := activeWingGrantTx(tx, principal, rec.ProjectID, rec.RouterNamespace)
	if err != nil {
		return WingReceipt{}, err
	}
	for _, root := range append([]string{rec.Workspace.RepositoryRoot, rec.Workspace.EvidenceRoot}, rec.Workspace.WritableRoots...) {
		ok, err := containedInAny(root, grant.RepositoryRoot, grant.EvidenceRoots)
		if err != nil {
			return WingReceipt{}, fmt.Errorf("%w: %v", ErrWingRootNotContained, err)
		}
		if !ok {
			return WingReceipt{}, ErrWingRootNotContained
		}
	}

	now := s.clock().Format(time.RFC3339)
	_, err = tx.Exec(
		`INSERT INTO wings(wing_id,project_id,router_namespace,owner,content_hash,record_json,created,updated) VALUES(?,?,?,?,?,?,?,?)`,
		rec.ID, rec.ProjectID, rec.RouterNamespace, rec.Owner, hash, string(raw), now, now,
	)
	if err == nil {
		if commitErr := tx.Commit(); commitErr != nil {
			return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: commit: %w", commitErr)
		}
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
	// and resolve exactly as the pre-check above would have, inside the same
	// transaction so the result is still a consistent snapshot.
	receipt, found, lookupErr := lookupWingTx(tx, rec.ID, hash)
	if lookupErr != nil {
		return WingReceipt{}, lookupErr
	}
	if !found {
		// The unique-constraint row that caused the insert to fail is gone by
		// the time of this re-read (e.g. raced with a since-reverted insert).
		// Surface the original insert error rather than mis-reporting ErrWingConflict.
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: insert: %w", err)
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return WingReceipt{}, fmt.Errorf("routerstore: RegisterWing: commit: %w", commitErr)
	}
	return receipt, nil
}

// lookupWingTx resolves an existing row for wingID within tx: found=false
// means no row exists (caller should insert). A row with a different content
// hash than hash returns ErrWingConflict directly, so every caller gets the
// same conflict behavior.
func lookupWingTx(tx *txHandle, wingID, hash string) (WingReceipt, bool, error) {
	var out WingReceipt
	var existingHash string
	err := tx.QueryRow(`SELECT wing_id,project_id,router_namespace,content_hash,created FROM wings WHERE wing_id=?`, wingID).
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
