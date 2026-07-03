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
	"database/sql"
	"errors"
	"fmt"
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
// migrations[i] moves a database from user_version i to user_version i+1.
// NEVER edit an entry once it has shipped — append a new migration instead
// (e.g. `ALTER TABLE items ADD COLUMN ...`), so future column adds work
// against existing databases without wiping them.
//
// Migration 1 uses plain CREATE TABLE (no IF NOT EXISTS) deliberately: a
// database that has tables but user_version 0 predates schema versioning and
// has an unknown shape — failing loudly beats silently stamping it v1. No such
// database can exist outside tests (the store has never been wired to
// anything), so nothing shipped is stranded by this choice.
var migrations = []string{
	// v1 — initial schema. items mirrors internal/work.Item field-for-field
	// (incl. the wake_* delivery-truth columns; see Item).
	`
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
`,
}

// migrate applies any pending numbered migrations, tracked via the SQLite
// user_version pragma. Idempotent — safe to call on every Open: a database
// already at the current version applies nothing.
func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		return fmt.Errorf("routerstore: migrate: read user_version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("routerstore: migrate: database is at schema version %d, newer than this binary understands (max %d) — refusing to touch it", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("routerstore: migrate to v%d: begin: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("routerstore: migrate to v%d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d;", i+1)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("routerstore: migrate to v%d: set user_version: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("routerstore: migrate to v%d: commit: %w", i+1, err)
		}
	}
	return nil
}
