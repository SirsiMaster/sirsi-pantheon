package routerstore

// Fenced-lease lifecycle — Phase 2 Dispatch Contract §2b axioms 1–3 (ADR-035).
//
// The routerstore is the ONLY executable dispatch authority. Claim is the only
// door to execution (pull stays read-only observation); every lifecycle
// mutation is atomic (BEGIN IMMEDIATE) and fenced on the lease token IN THE
// WHERE CLAUSE, so there is no check→update race window. Terminal states are
// terminal: "give up" is a dead_letter with metadata, never an item left open
// forever — the structural negation of the 19,195-sessions/0-closed incident.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Lifecycle states (§2b axiom 1). "closed" remains valid as the legacy
// terminal state carried by mirrored file items (Phase-1 fidelity).
const (
	StatusOpen       = "open"
	StatusClaimed    = "claimed"
	StatusWorking    = "working"
	StatusBlocked    = "blocked"
	StatusDeadLetter = "dead_letter"
	StatusCompleted  = "completed"
	StatusClosed     = "closed" // legacy terminal (file-router vocabulary)
)

// IsTerminal reports whether a status is terminal. Terminal states are
// terminal — no lifecycle op may leave one (only a human ForceOwner reopen).
func IsTerminal(status string) bool {
	return status == StatusDeadLetter || status == StatusCompleted || status == StatusClosed
}

// Budgets & backpressure (§2b axiom 7). Vars, not consts, so tests can
// tighten them; production callers treat them as configuration defaults.
var (
	// MaxConcurrentClaimsPerTarget caps live leases held per target agent.
	MaxConcurrentClaimsPerTarget = 2
	// MaxRetriesPerItem is the attempts ceiling before an item dead-letters.
	MaxRetriesPerItem = 3
	// MaxTotalActive caps items in claimed/working across the whole store.
	MaxTotalActive = 200
	// MaxLeaseTTL is the hard ceiling for every item/task claim and renewal.
	// Legitimate long work renews periodically; no caller may suppress recovery
	// for an arbitrary duration by minting a year-long lease.
	MaxLeaseTTL = 30 * time.Minute
)

func boundedLeaseTTL(ttl time.Duration) (time.Duration, error) {
	if ttl <= 0 {
		return 10 * time.Minute, nil
	}
	if ttl > MaxLeaseTTL {
		return 0, fmt.Errorf("routerstore: lease ttl %s exceeds maximum %s; renew bounded leases for long work", ttl, MaxLeaseTTL)
	}
	return ttl, nil
}

// Lifecycle errors.
var (
	// ErrNoWork means the agent's inbox has no claimable open item.
	ErrNoWork = errors.New("routerstore: no open item to claim")
	// ErrNoClaimableTask is the TASK-ledger counterpart of ErrNoWork.
	//
	// It exists because the two sources are independent: an agent can have an
	// empty inbox and a full task ledger at the same time. Returning ErrNoWork
	// ("no open ITEM to claim") from the task path told such an agent it had no
	// work, which is false about the source it just queried.
	//
	// Measured cost, 2026-08-07: codex-finalwishes held two pending, unblocked,
	// unleased, zero-attempt task rows — provably claimable, verified by running
	// the claim by hand — read this message as "the store will not let me claim",
	// and escalated to the owner for "router-store repair" that was never needed.
	// The message names the ledger and the verb so the next agent does not.
	ErrNoClaimableTask = errors.New("routerstore: no claimable task in the ledger (rows may be done, blocked on an unfinished dependency, already leased, or at the retry ceiling; `router task list <agent>` shows which — note this is the TASK ledger, separate from the inbox)")
	// ErrLeaseInvalid means the token is missing, expired, or mismatched —
	// including an expired worker trying to complete newer-leased work.
	ErrLeaseInvalid = errors.New("routerstore: lease token invalid, expired, or superseded")
	// ErrTerminal means the item is in a terminal state; terminal is terminal.
	ErrTerminal = errors.New("routerstore: item is in a terminal state")
	// ErrBudgetExceeded means a §2b budget (concurrent claims, total active)
	// refused the operation. Back off; do not retry in a tight loop.
	ErrBudgetExceeded = errors.New("routerstore: dispatch budget exceeded")
	// ErrReasonRequired guards the human-only override path.
	ErrReasonRequired = errors.New("routerstore: --force-owner requires an explicit reason")
)

// Lease is executable ownership of one item, held by one agent, until expiry.
type Lease struct {
	ItemID  string
	Agent   string
	Token   string
	Expires time.Time
}

// newToken returns a 128-bit random hex fence token.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("routerstore: token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// beginImmediate opens a write transaction that takes the write lock up front
// (SQLite BEGIN IMMEDIATE), so every lifecycle mutation serializes cleanly
// under WAL instead of failing mid-transaction on lock upgrade.
func (s *Store) beginImmediate() (*sql.Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	// database/sql has no IMMEDIATE knob; with MaxOpenConns(1) + busy_timeout
	// the single connection serializes writers, which is the property we need.
	return tx, nil
}

// ClaimNext atomically claims the oldest open item addressed to agent and
// returns its fenced lease. It is the ONLY transition into executable
// ownership (§2b axiom 3). Order of checks inside one transaction:
//
//  1. Reclaim expired leases store-wide (crash-safe self-heal; an item whose
//     holder died returns to open — or dead-letters when attempts are spent).
//  2. Circuit breakers (global, target) — a tripped breaker pauses dispatch.
//  3. Budgets — per-target concurrent claims and total active work.
//  4. Claim: status open→claimed with a fresh token, guarded in the UPDATE.
func (s *Store) ClaimNext(agent string, ttl time.Duration) (*Lease, error) {
	// Retry the WHOLE transaction under READONLY contention. ClaimNext is the only
	// transition into executable ownership, so a lane that loses this race does not
	// merely retry later — it silently fails to pick up work at all.
	//
	// Contention arrives as SQLITE_READONLY(8), never SQLITE_BUSY(5), so the DSN's
	// busy_timeout never engages (see writeretry.go). The transaction body must
	// re-run rather than a single statement: SQLite frequently surfaces the lock
	// failure at Commit, and nothing is committed on the failing path, so a re-run
	// observes no partial work. A fresh token is minted per attempt because
	// claimNextOnce generates it inside the body.
	//
	// Budget and breaker errors are not contention, so retryWrite returns them
	// immediately rather than burning the budget on a decision that will not change.
	var out *Lease
	err := s.retryWrite(func() error {
		l, err := s.claimNextOnce(agent, ttl)
		if err != nil {
			return err
		}
		out = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// claimNextOnce is one attempt of ClaimNext. Never call it directly — callers
// must go through ClaimNext so contention is retried.
func (s *Store) claimNextOnce(agent string, ttl time.Duration) (*Lease, error) {
	if strings.TrimSpace(agent) == "" {
		return nil, fmt.Errorf("routerstore: ClaimNext: agent is required")
	}
	var err error
	ttl, err = boundedLeaseTTL(ttl)
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := s.clock()

	tx, err := s.beginImmediate()
	if err != nil {
		return nil, fmt.Errorf("routerstore: ClaimNext: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err = s.reclaimExpiredTx(tx, now); err != nil {
		return nil, err
	}
	if err = s.breakerGateTx(tx, "global", "target:"+agent); err != nil {
		return nil, err
	}

	// Budgets (§2b axiom 7).
	var active int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM items WHERE status IN ('claimed','working');`).Scan(&active); err != nil {
		return nil, fmt.Errorf("routerstore: ClaimNext: count active: %w", err)
	}
	if active >= MaxTotalActive {
		return nil, fmt.Errorf("%w: %d items already active (max %d)", ErrBudgetExceeded, active, MaxTotalActive)
	}
	var mine int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM items WHERE status IN ('claimed','working') AND claimed_by = ?;`, agent).Scan(&mine); err != nil {
		return nil, fmt.Errorf("routerstore: ClaimNext: count agent claims: %w", err)
	}
	if mine >= MaxConcurrentClaimsPerTarget {
		return nil, fmt.Errorf("%w: %s already holds %d leases (max %d)", ErrBudgetExceeded, agent, mine, MaxConcurrentClaimsPerTarget)
	}

	var id string
	err = tx.QueryRow(`SELECT i.id FROM items i WHERE i.status = 'open' AND i.to_agent = ? AND `+
		actionableItemDependency("i")+` ORDER BY i.id ASC LIMIT 1;`, agent).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoWork
	}
	if err != nil {
		return nil, fmt.Errorf("routerstore: ClaimNext: select: %w", err)
	}

	expires := now.Add(ttl)
	res, err := tx.Exec(
		`UPDATE items AS i SET status='claimed', claimed_by=?, lease_token=?, lease_expires=?, lease_updated=? WHERE id=? AND status='open' AND `+actionableItemDependency("i")+`;`,
		agent, token, expires.Format(time.RFC3339), now.Format(time.RFC3339), id,
	)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ClaimNext: claim: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		// Another claimant won between SELECT and UPDATE within our own lock —
		// cannot happen with one write connection, but stay honest if it does.
		return nil, ErrNoWork
	}
	if err := bumpCounterTx(tx, "claims", 1); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("routerstore: ClaimNext: commit: %w", err)
	}
	return &Lease{ItemID: id, Agent: agent, Token: token, Expires: expires}, nil
}

// fenceTx returns the item's current status iff the (id, token) fence holds:
// token matches AND the lease is unexpired AND the item is in a live claimed/
// working state. Every mutation below embeds the same predicate in its UPDATE;
// this pre-read only exists to disambiguate the error.
func (s *Store) fenceErrTx(tx *sql.Tx, id, token string, now time.Time) error {
	var status, curToken, expires string
	err := tx.QueryRow(`SELECT status, lease_token, lease_expires FROM items WHERE id = ?;`, id).
		Scan(&status, &curToken, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("routerstore: fence read %q: %w", id, err)
	}
	if IsTerminal(status) {
		return ErrTerminal
	}
	if curToken == "" || curToken != token {
		return ErrLeaseInvalid
	}
	if exp, perr := time.Parse(time.RFC3339, expires); perr != nil || !exp.After(now) {
		return ErrLeaseInvalid
	}
	return nil
}

// leaseFence is the shared SQL predicate every fenced mutation embeds.
const leaseFence = ` AND lease_token = ? AND lease_expires > ? AND status IN ('claimed','working')`

// RenewLease extends a live lease's expiry (token-fenced).
func (s *Store) RenewLease(id, token string, ttl time.Duration) error {
	var err error
	ttl, err = boundedLeaseTTL(ttl)
	if err != nil {
		return err
	}
	now := s.clock()
	return s.fencedUpdate(id, token, now,
		`UPDATE items SET lease_expires = ?, lease_updated = ? WHERE id = ?`+leaseFence+`;`,
		now.Add(ttl).Format(time.RFC3339), now.Format(time.RFC3339), id, token, now.Format(time.RFC3339))
}

// StartWork transitions claimed→working (token-fenced). Idempotent for an
// item already working under the same live lease.
func (s *Store) StartWork(id, token string) error {
	now := s.clock()
	return s.fencedUpdate(id, token, now,
		`UPDATE items SET status = 'working', lease_updated = ? WHERE id = ?`+leaseFence+`;`,
		now.Format(time.RFC3339), id, token, now.Format(time.RFC3339))
}

// Complete terminally finishes an item under a live lease (token-fenced) —
// an expired or superseded worker CANNOT complete newer-leased work, because
// its token no longer matches the fence. This includes owner-session closes:
// there is no unfenced complete short of the audited ForceOwner.
func (s *Store) Complete(id, token, result string) error {
	now := s.clock()
	body := strings.TrimSpace(result)
	if body == "" {
		body = "(completed without result)"
	}
	return s.fencedUpdate(id, token, now,
		`UPDATE items SET status='completed', closed=?, result=?, lease_token='', lease_expires='' WHERE id = ?`+leaseFence+`;`,
		now.Format(time.RFC3339), body, id, token, now.Format(time.RFC3339))
}

// Fail records a failed attempt under a live lease (token-fenced). Below the
// retry ceiling the item returns to open (lease cleared) for a later claim;
// at the ceiling it dead-letters TERMINALLY, emits exactly one keyed-singleton
// escalation (§2b axiom 5 — occurrences grow, rows do not), and records the
// failure against the sender/target/class breaker domains.
func (s *Store) Fail(id, token, reason, failureClass string) error {
	now := s.clock()
	if failureClass == "" {
		failureClass = "unclassified"
	}
	tx, err := s.beginImmediate()
	if err != nil {
		return fmt.Errorf("routerstore: Fail: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err = s.fenceErrTx(tx, id, token, now); err != nil {
		return err
	}
	var attempts int
	var from, to string
	if err = tx.QueryRow(`SELECT attempts, from_agent, to_agent FROM items WHERE id = ?;`, id).Scan(&attempts, &from, &to); err != nil {
		return fmt.Errorf("routerstore: Fail: read: %w", err)
	}
	attempts++
	if err = bumpCounterTx(tx, "retries", 1); err != nil {
		return err
	}

	if attempts >= MaxRetriesPerItem {
		_, err = tx.Exec(
			`UPDATE items SET status='dead_letter', attempts=?, failure_class=?, closed=?, result=?, lease_token='', lease_expires='' WHERE id=?;`,
			attempts, failureClass, now.Format(time.RFC3339),
			fmt.Sprintf("DEAD LETTER after %d attempts — %s", attempts, strings.TrimSpace(reason)), id,
		)
		if err != nil {
			return fmt.Errorf("routerstore: Fail: dead-letter: %w", err)
		}
		if err = bumpCounterTx(tx, "dead_letters", 1); err != nil {
			return err
		}
		if err = s.escalateTx(tx, now, id, failureClass,
			fmt.Sprintf("dead-letter: %s (→%s)", id, to),
			fmt.Sprintf("Item %s dead-lettered after %d attempts. Class: %s. Last error: %s", id, attempts, failureClass, strings.TrimSpace(reason)),
		); err != nil {
			return err
		}
		if err = s.recordFailureTx(tx, now, "sender:"+from, "target:"+to, "class:"+failureClass, "global"); err != nil {
			return err
		}
	} else {
		_, err = tx.Exec(
			`UPDATE items SET status='open', attempts=?, failure_class=?, lease_token='', lease_expires='', claimed_by='' WHERE id=?;`,
			attempts, failureClass, id,
		)
		if err != nil {
			return fmt.Errorf("routerstore: Fail: reopen: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("routerstore: Fail: commit: %w", err)
	}
	return nil
}

// Block parks an item as waiting on an external/owner dependency (token-
// fenced). Blocked is NOT terminal: an owner reopen or a future facade verb
// unblocks it deliberately — but it is out of every claim path meanwhile.
func (s *Store) Block(id, token, reason string) error {
	now := s.clock()
	return s.fencedUpdate(id, token, now,
		`UPDATE items SET status='blocked', result=?, lease_token='', lease_expires='' WHERE id = ?`+leaseFence+`;`,
		"BLOCKED: "+strings.TrimSpace(reason), id, token, now.Format(time.RFC3339))
}

// ForceOwner is the HUMAN-ONLY override (§2b axiom 2): it bypasses the token
// fence, requires an explicit reason, and writes an audit record in the same
// transaction. action ∈ {complete, dead_letter, reopen}.
func (s *Store) ForceOwner(id, action, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	now := s.clock()
	tx, err := s.beginImmediate()
	if err != nil {
		return fmt.Errorf("routerstore: ForceOwner: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRow(`SELECT status FROM items WHERE id = ?;`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("routerstore: ForceOwner: read: %w", err)
	}

	var q string
	switch action {
	case "complete":
		q = `UPDATE items SET status='completed', closed=?, result=?, lease_token='', lease_expires='' WHERE id=?;`
	case "dead_letter":
		q = `UPDATE items SET status='dead_letter', closed=?, result=?, lease_token='', lease_expires='' WHERE id=?;`
	case "reopen":
		q = `UPDATE items SET status='open', closed='', result=?, lease_token='', lease_expires='', claimed_by='', attempts=0 WHERE id=?;`
	default:
		return fmt.Errorf("routerstore: ForceOwner: unknown action %q (complete|dead_letter|reopen)", action)
	}
	note := fmt.Sprintf("FORCE-OWNER %s: %s", action, strings.TrimSpace(reason))
	if action == "reopen" {
		_, err = tx.Exec(q, note, id)
	} else {
		_, err = tx.Exec(q, now.Format(time.RFC3339), note, id)
	}
	if err != nil {
		return fmt.Errorf("routerstore: ForceOwner: %w", err)
	}
	// Audited: who/when/what/why, durably, in the same transaction.
	_, err = tx.Exec(`INSERT INTO state(key, value) VALUES (?, ?);`,
		fmt.Sprintf("audit:force-owner:%s:%s", now.Format("20060102-150405.000000000"), id),
		fmt.Sprintf("action=%s prior_status=%s reason=%s", action, status, strings.TrimSpace(reason)))
	if err != nil {
		return fmt.Errorf("routerstore: ForceOwner: audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("routerstore: ForceOwner: commit: %w", err)
	}
	return nil
}

// fencedUpdate runs one token-fenced UPDATE inside an immediate transaction
// and disambiguates a zero-row result into the precise lifecycle error.
func (s *Store) fencedUpdate(id, token string, now time.Time, q string, args ...any) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return fmt.Errorf("routerstore: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(q, args...)
	if err != nil {
		return fmt.Errorf("routerstore: fenced update: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return s.fenceErrTx(tx, id, token, now) // precise error: not-found/terminal/invalid
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("routerstore: commit: %w", err)
	}
	return nil
}

// reclaimExpiredTx returns expired-lease items to the claimable pool (or
// dead-letters them when attempts are spent) — the crash-safe path that makes
// "restart mid-lease" survivable without stranding work. Counted so the next
// incident is one red number (§2b axiom 9).
func (s *Store) reclaimExpiredTx(tx *sql.Tx, now time.Time) error {
	return s.reclaimExpiredForTx(tx, now, "")
}

func (s *Store) reclaimExpiredForTx(tx *sql.Tx, now time.Time, agent string) error {
	filter := ""
	args := []any{now.Format(time.RFC3339)}
	if agent != "" {
		filter = " AND to_agent = ?"
		args = append(args, agent)
	}
	rows, err := tx.Query(
		`SELECT id, attempts, from_agent, to_agent, failure_class FROM items
		 WHERE status IN ('claimed','working') AND lease_expires <> '' AND lease_expires <= ?`+filter+`;`, args...)
	if err != nil {
		return fmt.Errorf("routerstore: reclaim: select: %w", err)
	}
	type expired struct {
		id, from, to, class string
		attempts            int
	}
	var exp []expired
	for rows.Next() {
		var e expired
		if err = rows.Scan(&e.id, &e.attempts, &e.from, &e.to, &e.class); err != nil {
			_ = rows.Close()
			return fmt.Errorf("routerstore: reclaim: scan: %w", err)
		}
		exp = append(exp, e)
	}
	_ = rows.Close()
	if err = rows.Err(); err != nil {
		return fmt.Errorf("routerstore: reclaim: rows: %w", err)
	}

	for _, e := range exp {
		attempts := e.attempts + 1 // an expiry consumes an attempt: a crash-looping holder must still converge to dead_letter
		if e.class == "" {
			e.class = "lease_expired"
		}
		if attempts >= MaxRetriesPerItem {
			_, err = tx.Exec(
				`UPDATE items SET status='dead_letter', attempts=?, failure_class=?, closed=?, result=?, lease_token='', lease_expires='' WHERE id=?;`,
				attempts, e.class, now.Format(time.RFC3339),
				fmt.Sprintf("DEAD LETTER after %d attempts — final lease expired unreleased", attempts), e.id)
			if err != nil {
				return fmt.Errorf("routerstore: reclaim: dead-letter %s: %w", e.id, err)
			}
			if err = bumpCounterTx(tx, "dead_letters", 1); err != nil {
				return err
			}
			if err = s.escalateTx(tx, now, e.id, e.class,
				fmt.Sprintf("dead-letter: %s (→%s)", e.id, e.to),
				fmt.Sprintf("Item %s dead-lettered after %d attempts (lease expired unreleased).", e.id, attempts),
			); err != nil {
				return err
			}
			if err = s.recordFailureTx(tx, now, "sender:"+e.from, "target:"+e.to, "class:"+e.class, "global"); err != nil {
				return err
			}
		} else {
			_, err = tx.Exec(
				`UPDATE items SET status='open', attempts=?, lease_token='', lease_expires='', claimed_by='' WHERE id=?;`,
				attempts, e.id)
			if err != nil {
				return fmt.Errorf("routerstore: reclaim: reopen %s: %w", e.id, err)
			}
		}
		if err := bumpCounterTx(tx, "lease_expiries", 1); err != nil {
			return err
		}
	}
	return nil
}

// bumpCounterTx increments a named dispatch counter (§2b axiom 9 observability).
func bumpCounterTx(tx *sql.Tx, name string, delta int) error {
	_, err := tx.Exec(
		`INSERT INTO counters(name, value) VALUES (?, ?)
		 ON CONFLICT(name) DO UPDATE SET value = value + excluded.value;`, name, delta)
	if err != nil {
		return fmt.Errorf("routerstore: counter %s: %w", name, err)
	}
	return nil
}
