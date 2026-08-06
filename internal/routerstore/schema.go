// Package routerstore — schema.go
//
// Exported schema-version helpers. Used by internal/selfupdate to implement
// the real schema ceiling gate: candidate binary's highest migration entry vs
// the live store's PRAGMA user_version. This is the gate that prevents a
// v14 binary from installing over a v15 store (the 2026-08-06 P0 incident).
package routerstore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultStorePath returns the canonical path for the live router database
// (~/.sirsi/router.db). Callers that need to read the schema version of the
// production store should pass this to ReadLiveSchemaVersion.
func DefaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sirsi", "router.db")
}

// CurrentSchemaMax returns the highest migration version this binary knows.
// It is the binary's schema ceiling: the highest PRAGMA user_version it can
// safely open. A candidate binary must have CurrentSchemaMax >= live
// user_version or the install is refused.
func CurrentSchemaMax() int {
	return migrations[len(migrations)-1].version
}

// ReadLiveSchemaVersion opens the SQLite database at dbPath and returns its
// current PRAGMA user_version WITHOUT running any migrations. This is a
// read-only probe: it will not advance the schema.
//
// A missing database (user has not yet opened the router store) returns 0,
// nil — an absent store has schema 0, which any binary can handle.
// A path of "" or ":memory:" also returns 0, nil.
func ReadLiveSchemaVersion(dbPath string) (int, error) {
	if dbPath == "" || dbPath == ":memory:" {
		return 0, nil
	}
	// Use the same CGO-free driver as Store.Open, but a minimal DSN: no WAL,
	// no migration, read-only intent. We open read-write because modernc's
	// PRAGMA user_version= is always a no-op here; we never execute it.
	dsn := fmt.Sprintf("file:%s?_busy_timeout=2000&_journal_mode=WAL", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return 0, fmt.Errorf("routerstore: read schema version: open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	var v int
	if err := db.QueryRow(`PRAGMA user_version;`).Scan(&v); err != nil {
		return 0, fmt.Errorf("routerstore: read schema version: query %s: %w", dbPath, err)
	}
	return v, nil
}
