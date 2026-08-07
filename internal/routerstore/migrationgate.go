package routerstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

// The migration gate: a schema migration is a one-way write to shared
// production state, and until now ANY build could perform one.
//
// This happened twice on 2026-08-05. The second time, a binary built from an
// uncommitted worktree migrated the live store to v14 while every installed
// binary understood v7. Each one then fail-closed — correctly, that guard is
// good — and the fleet went dark: the dashboard returned 502, the CLI refused,
// the menubar had nothing to read. Recovery took an hour, most of it spent
// finding WHICH build did it, because the store records the version it is at
// and nothing about who put it there.
//
// Two rules follow, and they are deliberately different in strictness:
//
//  1. A build from a DIRTY tree may not migrate. Source that exists only in
//     someone's working directory cannot be rebuilt by anyone else, so the
//     migration it performs is unrecoverable by construction — there is no
//     commit to check out and compile.
//  2. Every migration records its provenance, so the next refusal NAMES the
//     build instead of posing a forensic puzzle.
//
// Reads are never gated. Refusing to read would convert a recoverable
// coordination problem into an outage, which is the thing being prevented.

// provenanceKey is a row in the existing `state` table, NOT a new column.
// Recording provenance via a migration would itself bump the schema and strand
// every peer binary — the exact failure this gate exists to stop.
const provenanceKey = "schema_migrated_by"

// buildStamp identifies the binary performing a migration.
type buildStamp struct {
	Revision string
	Dirty    bool
	// Known is false when the toolchain stamped no VCS info — `go test`
	// binaries and `go run` typically have none.
	Known bool
}

// currentBuildFn is a var so tests can stub the stamp (Rule A16/A21 injectable
// side effect) without rebuilding this binary under different VCS conditions.
var currentBuildFn = readBuildStamp

func currentBuild() buildStamp { return currentBuildFn() }

func readBuildStamp() buildStamp {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildStamp{}
	}
	var b buildStamp
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision, b.Known = s.Value, true
		case "vcs.modified":
			b.Dirty = s.Value == "true"
		}
	}
	return b
}

func (b buildStamp) String() string {
	if !b.Known {
		return "unstamped build (no VCS info — go test/go run)"
	}
	rev := b.Revision
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if b.Dirty {
		return rev + " (DIRTY — uncommitted changes)"
	}
	return rev
}

// checkMigrationAllowed refuses migrations from builds whose source cannot be
// recovered by anyone else.
//
// Unstamped builds are ALLOWED. `go test` produces no VCS stamp, and a gate
// that blocked them would fail every test that opens a store — a gate nobody
// can run is a gate that gets deleted. The rule is therefore: refuse when we
// can positively prove the tree was dirty, not whenever we cannot prove it was
// clean. That is narrower than ideal and deliberately so; it catches the case
// that actually caused both outages.
func checkMigrationAllowed(from, to int, storePath string) error {
	b := currentBuild()
	if !b.Known || !b.Dirty {
		return nil
	}
	// Scope the refusal to the SHARED store, because that is the entire
	// argument for it: "every peer binary would fail closed with no commit to
	// recover from". No peer binary will ever open a store under t.TempDir(),
	// so migrating one from a dirty build strands nobody.
	//
	// The stale premise this repairs is in the comment on
	// TestUnstampedBuildIsAllowed: "go test produces no VCS stamp". Modern Go
	// stamps test binaries, so a dirty tree made EVERY test that opens a fresh
	// store fail its v0->v1 migration. The Ma'at pre-push gate had to print
	// "Working tree is DIRTY — routerstore then refuses its test migration and
	// cmd/sirsi tests fail for reasons unrelated to your commit", i.e. the gate
	// had become a known-false alarm operators were told to read past. A gate
	// people are instructed to ignore trains them past the real refusal too.
	//
	// Reuses isSharedProductionStore (store.go) rather than adding a second
	// definition of "shared" — it already resolves inodes, symlinked parents and
	// case-folding. The one deliberate difference: an EMPTY path is treated as
	// shared here, because that helper answers "is this the live store" (empty
	// means none) while this gate asks "may I make a one-way write that could
	// strand peers" (unknown must fail toward refusing).
	//
	// This does NOT weaken the shared-store rule: the canonical store still
	// refuses, which is the case that took the fleet down on 2026-08-05.
	if storePath != "" && !isSharedProductionStore(storePath) {
		return nil
	}
	if os.Getenv("SIRSI_ALLOW_DIRTY_MIGRATION") == "1" {
		fmt.Fprintf(os.Stderr,
			"routerstore: WARNING migrating v%d->v%d from a DIRTY build (%s) because SIRSI_ALLOW_DIRTY_MIGRATION=1.\n"+
				"  Every peer binary will fail closed until this source is committed, pushed, and installed everywhere.\n",
			from, to, b)
		return nil
	}
	return fmt.Errorf(
		"routerstore: refusing to migrate v%d->v%d from a build with uncommitted changes (%s).\n"+
			"  A schema migration is a one-way write to the shared store. Source that exists only in a working\n"+
			"  tree cannot be rebuilt by any other agent, so every peer binary would fail closed with no commit\n"+
			"  to recover from. This exact sequence took the fleet down on 2026-08-05.\n"+
			"  Fix: commit and push the migration, then build from the pushed commit.\n"+
			"  Override (accepting fleet-wide breakage): SIRSI_ALLOW_DIRTY_MIGRATION=1",
		from, to, b)
}

// recordMigrationProvenance stamps who migrated, into the existing state table.
// Best-effort: a provenance write must never fail a migration that already
// applied, or the store would be left at the new version with the transaction
// rolled back around it.
func recordMigrationProvenance(ctx context.Context, ex interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, to int) {
	b := currentBuild()
	stamp := fmt.Sprintf("v%d by %s at %s", to, b, time.Now().UTC().Format(time.RFC3339))
	_, _ = ex.ExecContext(ctx,
		`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		provenanceKey, stamp)
}

// readMigrationProvenance returns the recorded stamp, or "" if none.
//
// Deliberately tolerant: it is called on the error path of a store this binary
// cannot fully read, so a missing table or column must degrade to "unknown"
// rather than masking the real version error with a query failure.
func readMigrationProvenance(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) string {
	var v string
	if err := q.QueryRowContext(ctx, `SELECT value FROM state WHERE key=?`, provenanceKey).Scan(&v); err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// tooNewError builds the refusal a peer binary sees, naming the culprit build.
func tooNewError(version, maxVersion int, provenance string) error {
	msg := fmt.Sprintf(
		"routerstore: migrate: database is at schema version %d, newer than this binary understands (max %d) — refusing to touch it",
		version, maxVersion)
	if provenance == "" {
		return fmt.Errorf("%s\n  Migrated by: unknown (no provenance recorded — the migrating build predates the migration gate)", msg)
	}
	return fmt.Errorf("%s\n  Migrated by: %s\n  This binary must be rebuilt from a commit containing that migration.", msg, provenance)
}
