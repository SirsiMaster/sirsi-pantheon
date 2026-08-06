// Package routerstore is the durable, queryable index for the multi-agent
// work queue — Phase 1 of Router v2 (docs/prd/ROUTER_V2_DURABLE_DISPATCH.md).
//
// # Relationship to internal/work (the source of truth)
//
// The filesystem markdown items under <router>/items/*.md remain the CANONICAL
// source of truth (PANTHEON_RULES.md A26; Rule A12 additive-only). This package
// does NOT replace them. It is an ADDITIVE SQLite mirror/index: a queryable
// projection of the same items, so surfaces (dashboard, node-status, future
// event-driven dispatch) can ask "what's open for agent X?" with an indexed
// query instead of walking and parsing every file on every read.
//
// Nothing in the live router is wired to this store yet. It ships behind tests,
// in isolation, at zero risk to the working file router (PRD Phase 1 acceptance).
//
// # Design
//
//   - CGO-free SQLite via modernc.org/sqlite (Rule A3 static-binary mandate).
//   - The DB lives OUTSIDE any git repo (default ~/.sirsi/router.db) so runtime
//     state never pollutes the tree (PRD /goal #2). Callers pass the path.
//   - WAL mode + a single *sql.DB with a serialized writer (busy_timeout +
//     one connection for writes) keeps concurrent writes safe (Rule A21 intent
//     applied at the DB layer).
//   - The schema mirrors internal/work.Item field-for-field so import/export is
//     lossless (PRD /goal #4).
package routerstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite driver (Rule A3)
)

// ErrNotFound is returned by Get/Close when no item has the given id.
var ErrNotFound = errors.New("routerstore: item not found")

// ErrAlreadyClosed is returned by Close when the item is already closed,
// mirroring internal/work.Close semantics.
var ErrAlreadyClosed = errors.New("routerstore: item already closed")

// Item is the durable projection of one work item. Fields mirror
// internal/work.Item exactly — field-for-field, same names, same semantics —
// so a markdown item round-trips with zero loss (PRD /goal #4). The parity is
// enforced by TestFieldFidelityWithWorkItem: adding a field to work.Item fails
// that test until this struct AND the schema catch up.
type Item struct {
	ID           string
	From         string
	To           string
	Title        string
	Type         string // ADR-024 §5: "proposal" | "review" | "decision" | ""
	Status       string // "open" | "closed"
	Opened       string // RFC3339
	Closed       string // RFC3339, empty if open
	Instructions string
	Result       string
	BlockedBy    string // optional item id dependency; terminal dependencies do not block

	// Wake-delivery truth, mirroring internal/work.Item's wake_* frontmatter
	// (PR#2 wake-or-declare-unavailable). Dropping these in a backfill would
	// silently lose whether a stranded item was ever woken — exactly the data
	// loss PRD /goal #4 forbids.
	WakeStatus      string // "" when the wake pass has never touched this item
	WakeAttemptedAt string // RFC3339, set when an adapter was invoked
	WakeAdapter     string // the adapter that fired (cli-spawn/api-call/launchagent/...)
	WakeError       string // why the item is wake-unavailable, when it is
}

// Store is a durable index over the work queue backed by SQLite.
// A Store is safe for concurrent use by multiple goroutines.
type Store struct {
	db  *sql.DB
	now func() time.Time // injectable clock (Rule A16); nil means time.Now().UTC()

	// Event-driven dispatch (Phase 2): in-process waiters per agent, woken by
	// SendGuarded/NotifyAgent. Guarded by waitMu per Rule A21.
	waitMu  sync.Mutex
	waiters map[string][]chan struct{}
	// notifyDir overrides the cross-process FIFO directory (tests); empty
	// means ~/.sirsi/notify.
	notifyDir string
}

// Open opens (creating if absent) the SQLite store at path and applies the
// schema. path is a filesystem path such as ~/.sirsi/router.db; it MUST live
// outside any git repo. Use ":memory:" for tests.
//
// The caller owns the returned Store and must Close it.
func Open(path string) (*Store, error) {
	// WAL for reader/writer concurrency; busy_timeout so a momentarily locked
	// DB retries instead of erroring; foreign_keys off (single-table core).
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("routerstore: open %q: %w", path, err)
	}
	// SQLite is single-writer; serializing to one connection avoids
	// "database is locked" under concurrent writes (Rule A21 intent).
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// clock returns the store's time source (UTC), honoring an injected clock.
func (s *Store) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

// migrations is the ordered, append-only list of schema migrations.
// Each entry advances to its explicit version. Gaps are reserved schema
// versions owned by companion lineages and are never filled with fake no-ops.
// NEVER edit an entry once it has shipped — append a new migration instead
// (e.g. `ALTER TABLE items ADD COLUMN ...`), so future column adds work
// against existing databases without wiping them.
//
// Migration 1 uses plain CREATE TABLE (no IF NOT EXISTS) deliberately: a
// database that has tables but user_version 0 predates schema versioning and
// has an unknown shape — failing loudly beats silently stamping it v1. No such
// database can exist outside tests (the store has never been wired to
// anything), so nothing shipped is stranded by this choice.
type schemaMigration struct {
	version int
	sql     string
}

var migrations = []schemaMigration{
	// v1 — initial schema. items mirrors internal/work.Item field-for-field
	// (incl. the wake_* delivery-truth columns; see Item).
	{1, `
CREATE TABLE items (
    id                TEXT PRIMARY KEY,
    from_agent        TEXT NOT NULL,
    to_agent          TEXT NOT NULL,
    title             TEXT NOT NULL,
    type              TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'open',
    opened            TEXT NOT NULL DEFAULT '',
    closed            TEXT NOT NULL DEFAULT '',
    instructions      TEXT NOT NULL DEFAULT '',
    result            TEXT NOT NULL DEFAULT '',
    wake_status       TEXT NOT NULL DEFAULT '',
    wake_attempted_at TEXT NOT NULL DEFAULT '',
    wake_adapter      TEXT NOT NULL DEFAULT '',
    wake_error        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_items_to_status ON items(to_agent, status);

CREATE TABLE agents (
    id            TEXT PRIMARY KEY,
    registered_at TEXT NOT NULL DEFAULT '',
    last_seen     TEXT NOT NULL DEFAULT '',
    pid           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
`},

	// v2 — Phase 2 Dispatch Contract (PRD §2b, codex-SME APPROVED 2026-07-04;
	// ADR-035). Fenced-lease lifecycle columns, idempotency + singleton keys
	// (enforced as DATABASE invariants via partial unique indexes — the
	// property whose absence produced the 11,564-item flood), send quotas,
	// circuit breakers, and dispatch counters.
	{2, `
ALTER TABLE items ADD COLUMN lease_token   TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN lease_expires TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN claimed_by    TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN attempts      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN idem_key      TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN source_item   TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN failure_class TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN occurrences   INTEGER NOT NULL DEFAULT 1;
ALTER TABLE items ADD COLUMN first_seen    TEXT    NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN last_seen     TEXT    NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_items_idem ON items(idem_key) WHERE idem_key <> '';
CREATE UNIQUE INDEX idx_items_singleton ON items(source_item, failure_class)
    WHERE source_item <> '' AND failure_class <> '';
CREATE INDEX idx_items_lease ON items(status, lease_expires);

CREATE TABLE breakers (
    domain        TEXT PRIMARY KEY,
    failures      INTEGER NOT NULL DEFAULT 0,
    tripped_at    TEXT    NOT NULL DEFAULT '',
    operator_item TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE send_quota (
    sender TEXT NOT NULL,
    bucket TEXT NOT NULL,
    count  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (sender, bucket)
);

CREATE TABLE counters (
    name  TEXT PRIMARY KEY,
    value INTEGER NOT NULL DEFAULT 0
);
	`},

	// v3 — Universal Task Ledger. Items gain an optional dependency edge and
	// agents gain a durable task registry distinct from routed messages.
	{3, `
ALTER TABLE items ADD COLUMN blocked_by TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_items_blocked_by ON items(blocked_by) WHERE blocked_by <> '';

CREATE TABLE tasks (
    agent             TEXT NOT NULL,
    task_id           TEXT NOT NULL,
    subject           TEXT NOT NULL,
    status            TEXT NOT NULL,
    responsible_party TEXT NOT NULL,
    blocked_by        TEXT NOT NULL DEFAULT '',
    created           TEXT NOT NULL,
    updated           TEXT NOT NULL,
    PRIMARY KEY (agent, task_id)
);
CREATE INDEX idx_tasks_agent_status ON tasks(agent, status);
`},

	// v4 — Phase grouping for the Ledger Board (ADR-050 cross-surface spec).
	// Phase clusters tasks into plain-English groups for owner-facing rendering.
	{4, `ALTER TABLE tasks ADD COLUMN phase TEXT NOT NULL DEFAULT '';`},

	// v7 — Ledger Board drill-down contract (ADR-054 Part B). Versions 5 and
	// 6 belong to the ADR-051 conduit lineage; this migration intentionally
	// advances directly from any v4-v6 database to v7 in one additive step.
	{7, `
ALTER TABLE tasks ADD COLUMN charter TEXT;
ALTER TABLE tasks ADD COLUMN commissioned_at TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN commissioned_by TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN outline TEXT;
ALTER TABLE tasks ADD COLUMN timeline TEXT NOT NULL DEFAULT '[]';
ALTER TABLE tasks ADD COLUMN links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE tasks ADD COLUMN test_state TEXT NOT NULL DEFAULT 'untested';
ALTER TABLE tasks ADD COLUMN stage TEXT NOT NULL DEFAULT 'spec';
ALTER TABLE tasks ADD COLUMN tokens_consumed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN duration_seconds INTEGER NOT NULL DEFAULT 0;
UPDATE tasks SET commissioned_at = created WHERE commissioned_at = '';
UPDATE tasks SET commissioned_by = agent WHERE commissioned_by = '';
`},
}

// migrate applies any pending numbered migrations, tracked via the SQLite
// user_version pragma. Idempotent — safe to call on every Open: a database
// already at the current version applies nothing.
// migrate is safe under CROSS-PROCESS concurrency on a fresh database. Two
// `sirsi` processes opening the same new store both start at user_version 0;
// because WAL lets a reader see a snapshot, a naive read-then-migrate lets the
// slower process re-run an already-applied step (e.g. `ALTER … ADD lease_token`
// → "duplicate column name"). The cutover makes this common — every agent opens
// the store. So each step runs under an explicit `BEGIN IMMEDIATE` (which grabs
// the write lock up front, contending via busy_timeout) and RE-READS
// user_version inside that lock: if a peer already advanced it, this process
// rolls back and re-loops instead of double-applying. All statements run on one
// pinned connection so the manual BEGIN/COMMIT is not spread across the pool.
func (s *Store) migrate() error {
	ctx := context.Background()
	// Pinning the connection establishes it — which on a FRESH database runs the
	// DSN pragmas, and setting journal_mode=WAL needs a brief exclusive lock. Two
	// processes opening the same new store race there and one sees SQLITE_BUSY at
	// connection-establishment, before any migration statement. Retry the pin on
	// busy so a concurrent first-open never fails the whole Open.
	conn, err := pinConnWithRetry(ctx, s.db)
	if err != nil {
		return fmt.Errorf("routerstore: migrate: pin connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Re-assert busy_timeout on this pinned connection for the BEGIN IMMEDIATE loop.
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("routerstore: migrate: set busy_timeout: %w", err)
	}

	for {
		var version int
		if err := conn.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&version); err != nil {
			return fmt.Errorf("routerstore: migrate: read user_version: %w", err)
		}
		maxVersion := migrations[len(migrations)-1].version
		if version > maxVersion {
			return fmt.Errorf("routerstore: migrate: database is at schema version %d, newer than this binary understands (max %d) — refusing to touch it", version, maxVersion)
		}
		if version == maxVersion {
			return nil // up to date
		}
		var next schemaMigration
		for _, candidate := range migrations {
			if candidate.version > version {
				next = candidate
				break
			}
		}

		// BEGIN IMMEDIATE acquires the write lock now (waiting up to busy_timeout
		// for a peer mid-migration) so the version re-check below is authoritative.
		// A fresh-init race can still surface SQLITE_BUSY before the lock is
		// grantable; retry with a bounded backoff rather than failing the Open.
		if err := beginImmediateWithRetry(ctx, conn); err != nil {
			return fmt.Errorf("routerstore: migrate to v%d: begin immediate: %w", next.version, err)
		}
		var locked int
		if err := conn.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&locked); err != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return fmt.Errorf("routerstore: migrate to v%d: re-read user_version: %w", next.version, err)
		}
		if locked != version {
			// A concurrent opener already advanced past i under the lock. Bail out
			// of this attempt and re-evaluate from the top — no double-apply.
			if _, err := conn.ExecContext(ctx, `ROLLBACK;`); err != nil {
				return fmt.Errorf("routerstore: migrate to v%d: rollback stale: %w", next.version, err)
			}
			continue
		}
		if _, err := conn.ExecContext(ctx, next.sql); err != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return fmt.Errorf("routerstore: migrate to v%d: %w", next.version, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d;", next.version)); err != nil {
			_, _ = conn.ExecContext(ctx, `ROLLBACK;`)
			return fmt.Errorf("routerstore: migrate to v%d: set user_version: %w", next.version, err)
		}
		if _, err := conn.ExecContext(ctx, `COMMIT;`); err != nil {
			return fmt.Errorf("routerstore: migrate to v%d: commit: %w", next.version, err)
		}
	}
}

// pinConnWithRetry acquires a pinned connection, retrying on a transient
// SQLITE_BUSY at connection-establishment — the fresh-DB WAL-init window where
// two processes contend to set journal_mode. Bounded ~5s (25 × 200ms).
func pinConnWithRetry(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	const attempts = 25
	var err error
	for a := 0; a < attempts; a++ {
		var conn *sql.Conn
		conn, err = db.Conn(ctx)
		if err == nil {
			// Force the connection to actually establish (run DSN pragmas) now, so
			// a WAL-init busy surfaces here where we can retry it, not later.
			if _, perr := conn.ExecContext(ctx, `PRAGMA user_version;`); perr == nil {
				return conn, nil
			} else {
				_ = conn.Close()
				err = perr
			}
		}
		if !isBusy(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, err
}

// beginImmediateWithRetry runs BEGIN IMMEDIATE, retrying on a transient
// SQLITE_BUSY / "database is locked" — the fresh-init WAL window where two
// processes race to create -wal/-shm and grab the first write lock. busy_timeout
// covers most contention; this bounded backoff covers the residual init race so
// a concurrent first-open never fails the whole Open. ~5s ceiling (25 × 200ms),
// well beyond any real fresh-DB migration (a few ALTERs).
func beginImmediateWithRetry(ctx context.Context, conn *sql.Conn) error {
	const attempts = 25
	var err error
	for a := 0; a < attempts; a++ {
		if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE;`); err == nil {
			return nil
		}
		if !isBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return err
}

// isBusy reports whether err is a SQLite busy/locked condition (driver-agnostic
// string match — modernc surfaces these as text, e.g. "database is locked (5)").
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "SQLITE_BUSY") ||
		strings.Contains(msg, "(5)")
}
