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
// internal/work.Item exactly so a markdown item round-trips with zero loss.
type Item struct {
	ID           string
	From         string
	To           string
	Title        string
	Type         string // "proposal" | "task" | "review" | "decision" | ""
	Repo         string
	Status       string // "open" | "closed"
	Opened       string // RFC3339
	Closed       string // RFC3339, empty if open
	Instructions string
	Result       string
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

// migrate creates the schema if absent. Idempotent — safe to call on every Open.
func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS items (
    id           TEXT PRIMARY KEY,
    from_agent   TEXT NOT NULL,
    to_agent     TEXT NOT NULL,
    title        TEXT NOT NULL,
    type         TEXT NOT NULL DEFAULT '',
    repo         TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open',
    opened       TEXT NOT NULL DEFAULT '',
    closed       TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL DEFAULT '',
    result       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_items_to_status ON items(to_agent, status);

CREATE TABLE IF NOT EXISTS agents (
    id            TEXT PRIMARY KEY,
    registered_at TEXT NOT NULL DEFAULT '',
    last_seen     TEXT NOT NULL DEFAULT '',
    pid           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("routerstore: migrate: %w", err)
	}
	return nil
}
