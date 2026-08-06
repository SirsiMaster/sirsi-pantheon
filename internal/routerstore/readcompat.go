package routerstore

import (
	"context"
	"database/sql"
	"fmt"
)

// Read-compatibility with a store newer than this binary.
//
// The version guard in migrate() is correct and stays: a binary that does not
// understand a schema must never WRITE to it. But refusing to READ turns a
// coordination problem into a blackout. On 2026-08-05 the live store was
// migrated to v14 by a build whose source no longer exists; every surface —
// dashboard, CLI, menubar — went dark, and the owner lost visibility into a
// fleet that was otherwise fine.
//
// This is the narrowly-justified read path, and its narrowness is the point:
//
//   - It NEVER migrates and NEVER writes. Opened read-only at the driver level,
//     so a bug cannot promote it to a writer.
//   - It reads ONLY the tables this binary's own migrations define. Columns and
//     tables added by a newer schema are not read, not guessed at, and not
//     rendered. A surface built on this is honestly a SUBSET, never a view of
//     the whole store.
//   - It reports the version gap so every consumer can say so out loud.
//
// What it deliberately does not do is pretend to comprehend v14. Porting the
// newer migrations to pass the version check would make the binary *look*
// compatible while silently ignoring execution-critical columns — a more
// dangerous false green than a refusal, which is exactly what codex-pantheon's
// review of PR #523 established. Reading a declared subset is a different claim
// from understanding the schema, and only the first one is true here.

// SchemaGap describes a store newer than this binary understands.
type SchemaGap struct {
	// StoreVersion is the schema the database is actually at.
	StoreVersion int
	// BinaryVersion is the newest schema this binary defines.
	BinaryVersion int
	// MigratedBy is the recorded provenance of the migration, or "" if the
	// migrating build predated provenance recording.
	MigratedBy string
}

// Degraded reports whether the store is ahead of this binary.
func (g SchemaGap) Degraded() bool { return g.StoreVersion > g.BinaryVersion }

// Banner is the operator-facing sentence a surface must display when degraded.
// Surfaces call this instead of composing their own wording, so three surfaces
// cannot describe the same condition three different ways — the divergence the
// fleet board exists to end.
func (g SchemaGap) Banner() string {
	if !g.Degraded() {
		return ""
	}
	b := fmt.Sprintf("Reading a PARTIAL view: store schema v%d is newer than this build (v%d). "+
		"Fields added after v%d are not shown.", g.StoreVersion, g.BinaryVersion, g.BinaryVersion)
	if g.MigratedBy != "" {
		b += " Migrated by: " + g.MigratedBy + "."
	}
	return b
}

// OpenReadOnly opens the store for reading without migrating it.
//
// Returns the store, the schema gap, and an error. A degraded gap is NOT an
// error: the caller gets a working reader plus the obligation to render the
// banner. Treating it as an error would reproduce the blackout.
func OpenReadOnly(path string) (*Store, SchemaGap, error) {
	// mode=ro is enforced by the driver, not by convention — but ONLY via the
	// file: URI form. Without the scheme the driver silently ignores the mode
	// parameter and hands back a writable handle: the first version of this
	// function did exactly that, and its own test caught a successful INSERT
	// through what was labeled read-only. A guarantee the driver does not
	// enforce is a comment.
	dsn := "file:" + path + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, SchemaGap{}, fmt.Errorf("routerstore: open read-only %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&version); err != nil {
		_ = db.Close()
		return nil, SchemaGap{}, fmt.Errorf("routerstore: read user_version: %w", err)
	}

	gap := SchemaGap{
		StoreVersion:  version,
		BinaryVersion: migrations[len(migrations)-1].version,
	}
	if gap.Degraded() {
		// Best-effort: a store that predates provenance recording has no row,
		// and that must not fail the open.
		var by string
		if err := db.QueryRowContext(ctx,
			`SELECT value FROM state WHERE key=?`, provenanceKey).Scan(&by); err == nil {
			gap.MigratedBy = by
		}
	}
	return &Store{db: db}, gap, nil
}
