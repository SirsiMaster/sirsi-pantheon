// Package routerstore — writeretry.go
//
// SQLite reports write-lock contention on this store as SQLITE_READONLY(8), not
// SQLITE_BUSY(5). That single fact broke the fleet on 2026-08-06.
//
// Why it happens: the store runs in WAL mode. When a connection needs to run WAL
// recovery (after a crash, or when the -shm must be reinitialized) it must take a
// brief exclusive lock. If another process holds the file, recovery cannot run and
// SQLite returns SQLITE_READONLY_RECOVERY — extended code 8|(1<<8) — whose primary
// code is READONLY. The DSN's busy_timeout(5000) NEVER engages, because the busy
// handler is only consulted for SQLITE_BUSY.
//
// What that looks like in production, and why it wastes hours every time:
//
//   - Reads stay perfectly healthy. `router status`, `show`, `pull`, `dump` all work.
//   - Writes from the losing process fail outright with "attempt to write a readonly
//     database", which reads like a permissions or corruption fault.
//   - The C `sqlite3` CLI writes the same file from the same shell without complaint.
//   - It is intermittent, and it clears the instant the other holder exits.
//
// Every plausible-looking remedy is therefore a dead end, and all of them were walked
// on 2026-08-06: chmod (mode was already 644 and ours), rebuilding the binary,
// checkpointing the WAL (`wal_checkpoint(TRUNCATE)` returned 0|0|0 — nothing to
// checkpoint), changing cwd, and hunting for a read-only open path in this package
// (there is none; the DSN is a plain path plus WAL/busy_timeout/synchronous pragmas).
// None of them address contention, so none of them worked.
//
// On a host running a triage loop, two claude-workers, a gemma worker, thread
// watchers and a board publisher, contention is not exotic — it is constant.
//
// The fix is to treat READONLY as what it actually is here: retryable contention.
package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlitedriver "modernc.org/sqlite"
)

const (
	// sqliteReadonly is SQLITE_READONLY. Extended codes carry it in the low byte
	// (READONLY_RECOVERY = 8|(1<<8) = 264, READONLY_CANTINIT = 8|(5<<8), ...), so
	// contention detection masks to the primary code rather than enumerating them.
	sqliteReadonly = 8

	// writeRetryBudget matches the DSN's busy_timeout(5000). A writer that would
	// have waited 5s for a BUSY lock should wait the same for a READONLY one —
	// the caller's contract is unchanged, only the error code differed.
	writeRetryBudget = 5 * time.Second

	writeRetryInitialDelay = 2 * time.Millisecond
	writeRetryMaxDelay     = 128 * time.Millisecond
)

// isReadonlyContention reports whether err is SQLite's READONLY, which on this
// store means "another process holds the file", not "this file is unwritable".
//
// A genuinely read-only file is a real fault and would keep failing past the
// retry budget, so misclassifying it here costs a bounded delay and then surfaces
// the same error — never silent data loss.
func isReadonlyContention(err error) bool {
	if err == nil {
		return false
	}
	var se *sqlitedriver.Error
	if errors.As(err, &se) {
		return se.Code()&0xFF == sqliteReadonly
	}
	// Fallback for errors that lost their driver type crossing a wrap boundary.
	return strings.Contains(err.Error(), "attempt to write a readonly database")
}

// retryWrite runs op, retrying with exponential backoff while it fails with
// READONLY or BUSY contention ("database is locked"), up to writeRetryBudget.
// Both are the same underlying condition — another process holding the
// store — surfacing under different SQLite codes depending on the contended
// operation (see this file's header for READONLY, and store.go's isBusy for
// plain lock contention on a single statement).
//
// op MUST be safe to re-run. Every caller here either executes a single
// autocommit statement or runs a transaction that failed before commit, so no
// partial work is ever observable by a retry.
func (s *SQLiteStore) retryWrite(op func() error) error {
	// time.Now(), NOT s.clock(): clock() may be an injected fixed test clock,
	// and a deadline built from a frozen clock would never expire.
	deadline := time.Now().Add(writeRetryBudget)
	delay := writeRetryInitialDelay
	attempts := 0

	for {
		err := op()
		attempts++
		if !s.dialect().retryable(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"routerstore: write blocked by another process holding the store for %s "+
					"(%d attempts; SQLITE_READONLY under WAL contention, not a permissions fault): %w",
				writeRetryBudget, attempts, err)
		}
		time.Sleep(delay)
		if delay < writeRetryMaxDelay {
			delay *= 2
		}
	}
}

// exec is the write-path replacement for s.db.Exec. Single autocommit statements
// are trivially safe to re-run under contention.
func (s *SQLiteStore) exec(query string, args ...any) (sql.Result, error) {
	var res sql.Result
	err := s.retryWrite(func() error {
		var execErr error
		res, execErr = s.db.Exec(query, args...)
		return execErr
	})
	return res, err
}
