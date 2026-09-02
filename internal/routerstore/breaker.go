package routerstore

// Circuit breakers by failure domain — Phase 2 Dispatch Contract §2b axiom 6
// (ADR-035). Domains: sender:<id>, target:<id>, class:<c>, global. A tripped
// breaker pauses dispatch through that domain and writes ONE bounded operator
// item (keyed singleton). N distinct failures ≠ N escalations.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BreakerThreshold trips a domain after this many recorded failures without a
// reset. The global domain uses 4× the threshold so a single noisy sender
// trips its own breaker long before it can pause the whole fabric.
var BreakerThreshold = 5

// ErrBreakerOpen means a circuit breaker has this dispatch path paused.
// Operator action (inspect + ResetBreaker) is the way through — retrying
// into an open breaker is how floods happen.
var ErrBreakerOpen = errors.New("routerstore: circuit breaker open — dispatch paused for this domain")

// Breaker is one failure domain's state.
type Breaker struct {
	Domain       string `json:"domain"`
	Failures     int    `json:"failures"`
	TrippedAt    string `json:"tripped_at,omitempty"`
	OperatorItem string `json:"operator_item,omitempty"`
}

// breakerGateTx fails with ErrBreakerOpen if any given domain is tripped.
func (s *SQLiteStore) breakerGateTx(tx *txHandle, domains ...string) error {
	for _, d := range domains {
		var tripped string
		err := tx.QueryRow(`SELECT tripped_at FROM breakers WHERE domain = ?;`, d).Scan(&tripped)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("routerstore: breaker gate %s: %w", d, err)
		}
		if tripped != "" {
			return fmt.Errorf("%w: %s", ErrBreakerOpen, d)
		}
	}
	return nil
}

// recordFailureTx bumps each domain's failure count and trips any domain that
// crosses its threshold. Tripping writes ONE keyed-singleton operator item in
// the same transaction — the operator sees one red card, not a flood.
func (s *SQLiteStore) recordFailureTx(tx *txHandle, now time.Time, domains ...string) error {
	for _, d := range domains {
		threshold := BreakerThreshold
		if d == "global" {
			threshold = BreakerThreshold * 4
		}
		if _, err := tx.Exec(
			`INSERT INTO breakers(domain, failures) VALUES (?, 1)
			 ON CONFLICT(domain) DO UPDATE SET failures = breakers.failures + 1;`, d); err != nil {
			return fmt.Errorf("routerstore: breaker record %s: %w", d, err)
		}
		var failures int
		var tripped string
		if err := tx.QueryRow(`SELECT failures, tripped_at FROM breakers WHERE domain = ?;`, d).Scan(&failures, &tripped); err != nil {
			return fmt.Errorf("routerstore: breaker read %s: %w", d, err)
		}
		if failures >= threshold && tripped == "" {
			opID := fmt.Sprintf("breaker:%s", d)
			if _, err := tx.Exec(`UPDATE breakers SET tripped_at = ?, operator_item = ? WHERE domain = ?;`,
				now.Format(time.RFC3339), opID, d); err != nil {
				return fmt.Errorf("routerstore: breaker trip %s: %w", d, err)
			}
			if err := s.escalateTx(tx, now, opID, "breaker_tripped",
				fmt.Sprintf("breaker tripped: %s", d),
				fmt.Sprintf("Circuit breaker %s tripped after %d failures at %s. Dispatch through this domain is paused. Inspect the dead letters, fix the cause, then `sirsi router breaker-reset %s`.",
					d, failures, now.Format(time.RFC3339), d),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// Breakers lists every domain with recorded failures, tripped first.
func (s *SQLiteStore) Breakers() ([]Breaker, error) {
	rows, err := s.db.Query(`SELECT domain, failures, tripped_at, operator_item FROM breakers ORDER BY tripped_at DESC, failures DESC;`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: Breakers: %w", err)
	}
	defer rows.Close()
	var out []Breaker
	for rows.Next() {
		var b Breaker
		if err := rows.Scan(&b.Domain, &b.Failures, &b.TrippedAt, &b.OperatorItem); err != nil {
			return nil, fmt.Errorf("routerstore: Breakers: scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ResetBreaker is the operator's deliberate re-arm of one domain after the
// cause is fixed. It clears the trip and the failure count.
func (s *SQLiteStore) ResetBreaker(domain string) error {
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("routerstore: ResetBreaker: domain is required")
	}
	res, err := s.exec(`UPDATE breakers SET failures = 0, tripped_at = '', operator_item = '' WHERE domain = ?;`, domain)
	if err != nil {
		return fmt.Errorf("routerstore: ResetBreaker %s: %w", domain, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
