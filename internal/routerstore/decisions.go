package routerstore

// Ma'at decision ledger ingest (SSA gate 20261009-184021, ACCEPT_BOUNDED_DESIGN,
// ra/maat-ledger-on-service). P1 scope only: schema (store.go migration v25)
// + this SQLite-dialect ingest method. NOT yet wired into serve.go's RPC
// dispatch table, NOT yet consumed by any CLI, and the Postgres schema
// mirror (pg/schema.sql) has NOT been bumped to v25 — each is its own
// follow-up slice; see the ledger task for the full checkpoint notes.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DecisionIngest is one decision record submitted for durable storage. host
// and session are deliberately not fields here: the server-verified caller
// identity is passed as separate arguments to IngestDecision so nothing in
// this struct can be mistaken for an authenticated claim (binding
// requirement #1 of the SSA gate).
type DecisionIngest struct {
	// SourceTime is the event time claimed by the record itself — untrusted,
	// stored as-is, never used to order records across producers.
	SourceTime string
	// PayloadVersion defaults to "v1" when empty.
	PayloadVersion string
	// RawBytes is the original decision payload, preserved verbatim.
	RawBytes []byte
}

// DecisionIngestReceipt confirms durable storage of one decision record.
type DecisionIngestReceipt struct {
	// ID is the full sha256 hex digest of the ingested RawBytes — never the
	// 8-hex display id internal/maat/decision.Read uses locally, which is
	// documented there as unsafe for deduplication.
	ID          string
	Duplicate   bool // true when this exact id already existed (idempotent re-ingest)
	ReceiptTime string
}

// ErrDecisionProducerMismatch is returned when a resubmitted decision's
// content digest already exists under a DIFFERENT producer host — content
// identity alone cannot distinguish a benign retry (same producer, same
// bytes) from one producer's bytes colliding with another's. Content-digest
// identity makes "same id, same producer, different bytes" structurally
// impossible, so this is the one conflict case this schema can represent.
var ErrDecisionProducerMismatch = errors.New("routerstore: decision content digest already recorded under a different producer host")

// IngestDecision durably records one Ma'at decision. The id is the full
// sha256 digest of rec.RawBytes, computed here rather than accepted from a
// caller. Resubmitting the exact same bytes from the SAME producer host is
// idempotent (Duplicate=true, no error) — the common case, a relay retry.
// The same bytes arriving under a DIFFERENT producer host is reported as
// ErrDecisionProducerMismatch instead of silently keeping the first writer's
// attribution.
func (s *SQLiteStore) IngestDecision(host, session string, rec DecisionIngest) (DecisionIngestReceipt, error) {
	host = strings.TrimSpace(host)
	session = strings.TrimSpace(session)
	if host == "" || session == "" {
		return DecisionIngestReceipt{}, errors.New("routerstore: ingest decision requires an authenticated host and session")
	}
	if len(rec.RawBytes) == 0 {
		return DecisionIngestReceipt{}, errors.New("routerstore: ingest decision requires non-empty raw bytes")
	}
	version := strings.TrimSpace(rec.PayloadVersion)
	if version == "" {
		version = "v1"
	}

	sum := sha256.Sum256(rec.RawBytes)
	id := hex.EncodeToString(sum[:])
	now := s.clock().UTC().Format(time.RFC3339)

	receipt := DecisionIngestReceipt{ID: id, ReceiptTime: now}
	if err := s.withTx(func(tx *txHandle) error {
		res, err := tx.Exec(
			`INSERT OR IGNORE INTO maat_decisions (id, producer_host, producer_session, receipt_time, source_time, payload_version, raw_bytes)
			 VALUES (?, ?, ?, ?, ?, ?, ?);`,
			id, host, session, now, rec.SourceTime, version, rec.RawBytes)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var existingHost string
			if scanErr := tx.QueryRow(`SELECT producer_host FROM maat_decisions WHERE id = ?;`, id).Scan(&existingHost); scanErr != nil {
				return scanErr
			}
			if existingHost != host {
				return ErrDecisionProducerMismatch
			}
			receipt.Duplicate = true
		}
		return nil
	}); err != nil {
		return DecisionIngestReceipt{}, fmt.Errorf("routerstore: ingest decision %s: %w", id, err)
	}
	return receipt, nil
}
