package routerstore

// One send facade — Phase 2 Dispatch Contract §2b axioms 4–5 (ADR-035).
//
// Every item-emitting path (CLI send, MCP send, any executor tier) routes
// through SendGuarded, which enforces an idempotency key and per-sender
// quotas BEFORE insert. Over-quota UPDATES a singleton throttle item — it
// never appends. Escalations are keyed singletons on (source_item,
// failure_class): occurrences grow, rows do not. Both singleton properties
// are database invariants (partial unique indexes), not application promises
// — the structural negation of the 11,564-item flood.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Quota defaults (§2b axiom 7). Vars so tests can tighten them.
var (
	// MaxSendsPerSenderPerWindow caps new items per sender per time bucket.
	MaxSendsPerSenderPerWindow = 30
	// SendWindow is the quota (and idempotency time_bucket) granularity.
	SendWindow = time.Hour
)

// ErrOverQuota means the sender exhausted its window budget; the singleton
// throttle item was updated in place. Nothing was appended.
var ErrOverQuota = errors.New("routerstore: sender over quota — throttle singleton updated, nothing appended")

// SendReq is one guarded send. SubjectKey defaults to a slug of Title; set it
// explicitly when retries may rephrase the title (the idempotency key must
// survive rewording). SourceItem ties emissions to the item that caused them.
type SendReq struct {
	From, To, Title, Type, Instructions string
	SubjectKey                          string
	SourceItem                          string
}

// idemKey builds the §2b idempotency key:
// (from, to, type, subject_key, source_item_id, time_bucket).
func (s *Store) idemKey(r SendReq, now time.Time) string {
	subject := r.SubjectKey
	if strings.TrimSpace(subject) == "" {
		subject = slugify(r.Title)
	}
	bucket := now.Truncate(SendWindow).Format("2006-01-02T15")
	return strings.Join([]string{r.From, r.To, r.Type, subject, r.SourceItem, bucket}, "|")
}

// SendGuarded is THE send facade. Returns (id, deduped, err):
//   - fresh insert → (new id, false, nil)
//   - idempotent duplicate → (existing id, true, nil); occurrences bumped
//   - over quota → ("", false, ErrOverQuota); throttle singleton updated
//   - tripped breaker → ("", false, ErrBreakerOpen)
func (s *Store) SendGuarded(r SendReq) (string, bool, error) {
	if strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" {
		return "", false, fmt.Errorf("routerstore: SendGuarded: from and to are required")
	}
	if err := validateMessageType(r.Type); err != nil {
		return "", false, err
	}
	now := s.clock()
	key := s.idemKey(r, now)

	tx, err := s.beginImmediate()
	if err != nil {
		return "", false, fmt.Errorf("routerstore: SendGuarded: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err = s.breakerGateTx(tx, "global", "sender:"+r.From); err != nil {
		return "", false, err
	}

	// Idempotency first: a duplicate is a dedupe, not quota spend.
	var existing string
	err = tx.QueryRow(`SELECT id FROM items WHERE idem_key = ?;`, key).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("routerstore: SendGuarded: idem lookup: %w", err)
	}
	if existing != "" {
		_, err = tx.Exec(`UPDATE items SET occurrences = occurrences + 1, last_seen = ? WHERE id = ?;`,
			now.Format(time.RFC3339), existing)
		if err != nil {
			return "", false, fmt.Errorf("routerstore: SendGuarded: dedupe bump: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return "", false, fmt.Errorf("routerstore: SendGuarded: commit: %w", err)
		}
		return existing, true, nil
	}

	// Quota (§2b axiom 7): spend one slot in this sender's window bucket.
	bucket := now.Truncate(SendWindow).Format("2006-01-02T15")
	if _, err = tx.Exec(
		`INSERT INTO send_quota(sender, bucket, count) VALUES (?, ?, 1)
		 ON CONFLICT(sender, bucket) DO UPDATE SET count = count + 1;`, r.From, bucket); err != nil {
		return "", false, fmt.Errorf("routerstore: SendGuarded: quota: %w", err)
	}
	var used int
	if err = tx.QueryRow(`SELECT count FROM send_quota WHERE sender = ? AND bucket = ?;`, r.From, bucket).Scan(&used); err != nil {
		return "", false, fmt.Errorf("routerstore: SendGuarded: quota read: %w", err)
	}
	if used > MaxSendsPerSenderPerWindow {
		// Over-quota UPDATES a singleton throttle item; it never appends.
		if err = s.escalateTx(tx, now, "throttle:"+r.From, "over_quota",
			fmt.Sprintf("throttled: %s over send quota", r.From),
			fmt.Sprintf("Sender %s exceeded %d sends in window %s. Dropped sends are counted here, not appended.", r.From, MaxSendsPerSenderPerWindow, bucket),
		); err != nil {
			return "", false, err
		}
		if err = bumpCounterTx(tx, "rate_limit_drops", 1); err != nil {
			return "", false, err
		}
		if err = s.recordFailureTx(tx, now, "sender:"+r.From); err != nil {
			return "", false, err
		}
		if err = tx.Commit(); err != nil {
			return "", false, fmt.Errorf("routerstore: SendGuarded: commit: %w", err)
		}
		return "", false, ErrOverQuota
	}

	id := fmt.Sprintf("%s-%s-%s-%s", now.Format("20060102-150405"), slugify(r.From), slugify(r.To), slugify(r.Title))
	_, err = tx.Exec(
		`INSERT INTO items (id, from_agent, to_agent, title, type, status, opened, instructions, idem_key, source_item, first_seen, last_seen)
		 VALUES (?, ?, ?, ?, ?, 'open', ?, ?, ?, ?, ?, ?);`,
		id, r.From, r.To, r.Title, strings.TrimSpace(r.Type), now.Format(time.RFC3339),
		strings.TrimSpace(r.Instructions), key, r.SourceItem,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return "", false, fmt.Errorf("routerstore: SendGuarded: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("routerstore: SendGuarded: commit: %w", err)
	}
	s.notifyWaiters(r.To)
	return id, false, nil
}

// escalateTx upserts the keyed-singleton escalation/throttle item on
// (source_item, failure_class) — §2b axiom 5. First occurrence creates ONE
// bounded item; every recurrence bumps occurrences/last_seen in place. The
// partial unique index makes duplicate rows impossible even under races.
func (s *Store) escalateTx(tx *sql.Tx, now time.Time, sourceItem, failureClass, title, body string) error {
	if sourceItem == "" || failureClass == "" {
		return fmt.Errorf("routerstore: escalate: source_item and failure_class are required")
	}
	id := fmt.Sprintf("%s-escalation-%s-%s", now.Format("20060102-150405"), slugify(sourceItem), slugify(failureClass))
	recipient := strings.TrimSpace(s.escalationAgent)
	if recipient == "" {
		recipient = "owner"
	}
	_, err := tx.Exec(
		`INSERT INTO items (id, from_agent, to_agent, title, type, status, opened, instructions, source_item, failure_class, first_seen, last_seen)
		 VALUES (?, 'routerstore', ?, ?, '', 'open', ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_item, failure_class) WHERE source_item <> '' AND failure_class <> '' DO UPDATE SET
		     occurrences = occurrences + 1,
		     last_seen   = excluded.last_seen;`,
		id, recipient, title, now.Format(time.RFC3339), strings.TrimSpace(body),
		sourceItem, failureClass, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("routerstore: escalate %s/%s: %w", sourceItem, failureClass, err)
	}
	return nil
}

// DispatchCounters is the ONE aggregate node-status renders (§2b axiom 9):
// the next incident must be one red number, not 11,564 files.
type DispatchCounters struct {
	Claims         int64 `json:"claims"`
	LeaseExpiries  int64 `json:"lease_expiries"`
	Retries        int64 `json:"retries"`
	RateLimitDrops int64 `json:"rate_limit_drops"`
	DeadLetters    int64 `json:"dead_letters"`
	BreakersOpen   int64 `json:"breakers_open"`
	ActiveClaims   int64 `json:"active_claims"`
	OpenItems      int64 `json:"open_items"`
}

// Counters returns the dispatch health aggregate.
func (s *Store) Counters() (DispatchCounters, error) {
	var c DispatchCounters
	rows, err := s.db.Query(`SELECT name, value FROM counters;`)
	if err != nil {
		return c, fmt.Errorf("routerstore: Counters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var v int64
		if err := rows.Scan(&name, &v); err != nil {
			return c, fmt.Errorf("routerstore: Counters: scan: %w", err)
		}
		switch name {
		case "claims":
			c.Claims = v
		case "lease_expiries":
			c.LeaseExpiries = v
		case "retries":
			c.Retries = v
		case "rate_limit_drops":
			c.RateLimitDrops = v
		case "dead_letters":
			c.DeadLetters = v
		}
	}
	if err := rows.Err(); err != nil {
		return c, fmt.Errorf("routerstore: Counters: rows: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM breakers WHERE tripped_at <> '';`).Scan(&c.BreakersOpen); err != nil {
		return c, fmt.Errorf("routerstore: Counters: breakers: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE status IN ('claimed','working');`).Scan(&c.ActiveClaims); err != nil {
		return c, fmt.Errorf("routerstore: Counters: active: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE status = 'open';`).Scan(&c.OpenItems); err != nil {
		return c, fmt.Errorf("routerstore: Counters: open: %w", err)
	}
	return c, nil
}

// GC applies retention (§2b axiom 9): terminal items older than keep are
// deleted (their audit lives in the exported markdown / git history), and
// quota buckets older than two windows are dropped. Returns rows removed.
func (s *Store) GC(keep time.Duration) (int64, error) {
	now := s.clock()
	cutoff := now.Add(-keep).Format(time.RFC3339)
	res, err := s.exec(
		`DELETE FROM items WHERE status IN ('completed','dead_letter','closed') AND closed <> '' AND closed <= ?;`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("routerstore: GC items: %w", err)
	}
	n, _ := res.RowsAffected()
	oldBucket := now.Add(-2 * SendWindow).Truncate(SendWindow).Format("2006-01-02T15")
	if _, err := s.exec(`DELETE FROM send_quota WHERE bucket < ?;`, oldBucket); err != nil {
		return n, fmt.Errorf("routerstore: GC quota: %w", err)
	}
	return n, nil
}
