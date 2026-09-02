package routerstore

// Durable identifier allocation — ADR-061 step 1 support.
//
// Document numbers (ADR-NNN, REQ-NNN) were allocated by agents reading the
// filesystem and counting: "highest ADR on main is 054, so mine is 055". That
// races by construction. Two agents branching from the same main both observe
// the same highest number, both claim it, and neither can detect the other
// until merge — by which point both documents exist and every citation of that
// number is ambiguous. origin/main carries TWO distinct ADR-054 documents
// today, and open PRs claim 051/054/055/056 between them.
//
// The router and the task ledger already solve exactly this problem for items
// and tasks: allocation goes through one serialized store, so two claimants
// cannot both win. Identifiers now use the same mechanism. PRIMARY KEY
// (namespace, number) makes a cross-claim structurally impossible rather than
// detectable after the fact — a CI uniqueness gate reports the collision, this
// prevents it.
//
// Withdrawn allocations are retained, never deleted. If a withdrawn number
// were reused, every existing citation of it would silently retarget to a
// different document, which is the citation-collision failure the allocator
// exists to end.

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Allocation statuses. "claimed" is reserved-but-unpublished; "published"
// means the document exists at the recorded slug; "withdrawn" retires a number
// without ever freeing it for reuse.
const (
	IdentifierClaimed   = "claimed"
	IdentifierPublished = "published"
	IdentifierWithdrawn = "withdrawn"
)

// ErrIdentifierTaken is returned when a specific number is requested and some
// other owner already holds it.
var ErrIdentifierTaken = errors.New("routerstore: identifier already allocated")

// Identifier is one allocated document number.
type Identifier struct {
	Namespace string `json:"namespace"`
	Number    int    `json:"number"`
	Slug      string `json:"slug,omitempty"`
	Title     string `json:"title"`
	Owner     string `json:"owner"`
	Status    string `json:"status"`
	ClaimedAt string `json:"claimed_at"`
}

// Name renders the canonical citation form, e.g. "ADR-061".
func (i Identifier) Name() string { return fmt.Sprintf("%s-%03d", i.Namespace, i.Number) }

// withTx runs fn inside a serialized write transaction. beginImmediate relies
// on MaxOpenConns(1) + busy_timeout to serialize writers, which is exactly the
// property the allocator needs: the MAX(number) read and the INSERT cannot be
// interleaved by another allocator.
func (s *SQLiteStore) withTx(fn func(*sql.Tx) error) error {
	// The whole transaction retries under READONLY contention, not just the
	// begin: SQLite frequently surfaces the lock failure at Commit. Nothing is
	// committed on the failing path, so re-running fn observes no partial work.
	// See writeretry.go for why contention arrives as READONLY and not BUSY.
	return s.retryWrite(func() error {
		tx, err := s.beginImmediate()
		if err != nil {
			return fmt.Errorf("routerstore: identifiers: begin: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit()
	})
}

var namespacePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,15}$`)

func normalizeNamespace(ns string) (string, error) {
	ns = strings.ToUpper(strings.TrimSpace(ns))
	if !namespacePattern.MatchString(ns) {
		return "", fmt.Errorf("routerstore: invalid identifier namespace %q: want uppercase alphanumeric, 2-16 chars (e.g. ADR, REQ)", ns)
	}
	return ns, nil
}

// AllocateIdentifier reserves the next free number in namespace for owner.
//
// The read of MAX(number) and the INSERT happen inside one BEGIN IMMEDIATE
// transaction, so two concurrent allocators cannot both observe the same
// highest number — the second blocks, re-reads, and gets the next one. This is
// the whole point: the race is closed by the store, not by convention.
func (s *SQLiteStore) AllocateIdentifier(namespace, title, owner string) (Identifier, error) {
	ns, err := normalizeNamespace(namespace)
	if err != nil {
		return Identifier{}, err
	}
	title = strings.TrimSpace(title)
	owner = strings.TrimSpace(owner)
	if title == "" {
		return Identifier{}, errors.New("routerstore: identifier title is required")
	}
	if owner == "" {
		return Identifier{}, errors.New("routerstore: identifier owner is required — an unowned allocation is how numbers get orphaned")
	}

	var out Identifier
	err = s.withTx(func(tx *sql.Tx) error {
		var next int
		// COALESCE over the whole namespace INCLUDING withdrawn rows, so a
		// retired number is never handed out a second time.
		if scanErr := tx.QueryRow(
			`SELECT COALESCE(MAX(number), 0) + 1 FROM identifiers WHERE namespace = ?;`, ns,
		).Scan(&next); scanErr != nil {
			return fmt.Errorf("routerstore: allocate %s: read high-water: %w", ns, scanErr)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, execErr := tx.Exec(
			`INSERT INTO identifiers (namespace, number, title, owner, status, claimed_at)
			 VALUES (?, ?, ?, ?, ?, ?);`,
			ns, next, title, owner, IdentifierClaimed, now,
		); execErr != nil {
			return fmt.Errorf("routerstore: allocate %s-%03d: %w", ns, next, execErr)
		}
		out = Identifier{Namespace: ns, Number: next, Title: title, Owner: owner, Status: IdentifierClaimed, ClaimedAt: now}
		return nil
	})
	if err != nil {
		return Identifier{}, err
	}
	return out, nil
}

// ClaimIdentifierNumber reserves one specific number — used to backfill
// documents that already exist on disk from before the allocator. It fails
// rather than silently reassigning when the number is held by someone else,
// and is idempotent for the same owner so backfill can be re-run safely.
func (s *SQLiteStore) ClaimIdentifierNumber(namespace string, number int, title, slug, owner string) (Identifier, error) {
	ns, err := normalizeNamespace(namespace)
	if err != nil {
		return Identifier{}, err
	}
	if number < 1 {
		return Identifier{}, fmt.Errorf("routerstore: identifier number must be >= 1, got %d", number)
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return Identifier{}, errors.New("routerstore: identifier owner is required")
	}

	var out Identifier
	err = s.withTx(func(tx *sql.Tx) error {
		var existingOwner, existingTitle, existingStatus, existingSlug, claimedAt string
		row := tx.QueryRow(
			`SELECT owner, title, status, slug, claimed_at FROM identifiers WHERE namespace = ? AND number = ?;`, ns, number)
		switch scanErr := row.Scan(&existingOwner, &existingTitle, &existingStatus, &existingSlug, &claimedAt); {
		case scanErr == nil:
			if existingOwner != owner {
				return fmt.Errorf("%w: %s-%03d is held by %q (you are %q) — pick a new number with AllocateIdentifier rather than reassigning a live citation",
					ErrIdentifierTaken, ns, number, existingOwner, owner)
			}
			out = Identifier{Namespace: ns, Number: number, Title: existingTitle, Slug: existingSlug, Owner: existingOwner, Status: existingStatus, ClaimedAt: claimedAt}
			return nil
		case errors.Is(scanErr, sql.ErrNoRows):
			now := time.Now().UTC().Format(time.RFC3339)
			if _, execErr := tx.Exec(
				`INSERT INTO identifiers (namespace, number, slug, title, owner, status, claimed_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?);`,
				ns, number, slug, title, owner, IdentifierPublished, now,
			); execErr != nil {
				return fmt.Errorf("routerstore: claim %s-%03d: %w", ns, number, execErr)
			}
			out = Identifier{Namespace: ns, Number: number, Slug: slug, Title: title, Owner: owner, Status: IdentifierPublished, ClaimedAt: now}
			return nil
		default:
			return fmt.Errorf("routerstore: claim %s-%03d: read: %w", ns, number, scanErr)
		}
	})
	if err != nil {
		return Identifier{}, err
	}
	return out, nil
}

// PublishIdentifier marks an allocation as published at slug.
func (s *SQLiteStore) PublishIdentifier(namespace string, number int, slug string) error {
	ns, err := normalizeNamespace(namespace)
	if err != nil {
		return err
	}
	return s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE identifiers SET slug = ?, status = ? WHERE namespace = ? AND number = ?;`,
			slug, IdentifierPublished, ns, number)
		if err != nil {
			return fmt.Errorf("routerstore: publish %s-%03d: %w", ns, number, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("routerstore: publish %s-%03d: not allocated — claim it first", ns, number)
		}
		return nil
	})
}

// WithdrawIdentifier retires a number. The row is retained so the number is
// never reissued.
func (s *SQLiteStore) WithdrawIdentifier(namespace string, number int) error {
	ns, err := normalizeNamespace(namespace)
	if err != nil {
		return err
	}
	return s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE identifiers SET status = ? WHERE namespace = ? AND number = ?;`,
			IdentifierWithdrawn, ns, number)
		if err != nil {
			return fmt.Errorf("routerstore: withdraw %s-%03d: %w", ns, number, err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("routerstore: withdraw %s-%03d: not allocated", ns, number)
		}
		return nil
	})
}

// ListIdentifiers returns every allocation in a namespace, lowest first.
func (s *SQLiteStore) ListIdentifiers(namespace string) ([]Identifier, error) {
	ns, err := normalizeNamespace(namespace)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT namespace, number, slug, title, owner, status, claimed_at
		   FROM identifiers WHERE namespace = ? ORDER BY number;`, ns)
	if err != nil {
		return nil, fmt.Errorf("routerstore: list %s: %w", ns, err)
	}
	defer rows.Close()

	var out []Identifier
	for rows.Next() {
		var i Identifier
		if err := rows.Scan(&i.Namespace, &i.Number, &i.Slug, &i.Title, &i.Owner, &i.Status, &i.ClaimedAt); err != nil {
			return nil, fmt.Errorf("routerstore: list %s: scan: %w", ns, err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
