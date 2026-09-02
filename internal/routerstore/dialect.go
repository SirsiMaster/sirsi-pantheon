package routerstore

// ADR-062 Phase B (rs-06): one store implementation, two SQL dialects.
//
// The ledger's SQL was written for SQLite. Rather than duplicate 74 methods
// for Postgres, every statement passes through a dialect rewriter on its way
// to database/sql. The rewrite is a fixed, tested token substitution (see
// pg/README.md for the table) — not a SQL parser — so it must stay in lockstep
// with pg/schema.sql. dbHandle and txHandle wrap *sql.DB / *sql.Tx so the
// rewrite is applied at exactly one seam and no method can forget it.

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	sqlitedriver "modernc.org/sqlite"
)

type dialect struct {
	name string
	// rewrite adapts a SQLite-flavored statement for this engine.
	rewrite func(string) string
	// retryable reports engine-level contention that the whole operation may
	// safely re-run (SQLite READONLY/BUSY; Postgres serialization/deadlock).
	retryable func(error) bool
	// claimLock is appended to the row-selecting SELECT inside a claim
	// transaction. Postgres: FOR UPDATE SKIP LOCKED so two claimants pick two
	// rows instead of racing on one; SQLite: nothing, one writer connection.
	claimLock string
	// hasPragmas is true only for SQLite; migrate()/schema-version reads are
	// skipped on engines whose schema is owned by an external migrator.
	hasPragmas bool
}

var sqliteDialect = &dialect{
	name:       "sqlite",
	rewrite:    func(q string) string { return q },
	retryable:  func(err error) bool { return isReadonlyContention(err) || isBusy(err) },
	claimLock:  "",
	hasPragmas: true,
}

var postgresDialect = &dialect{
	name:       "postgres",
	rewrite:    rewriteForPostgres,
	retryable:  isPostgresRetryable,
	claimLock:  " FOR UPDATE SKIP LOCKED",
	hasPragmas: false,
}

// rewriteForPostgres applies the pg/README.md translation table.
func rewriteForPostgres(q string) string {
	// Function/idiom substitutions first — they contain no placeholders.
	q = strings.ReplaceAll(q, "strftime('%Y-%m-%dT%H:%M:%SZ','now')", "router.now_rfc3339()")
	q = strings.ReplaceAll(q, "lower(hex(randomblob(16)))", "router.rand_hex32()")

	// INSERT OR IGNORE INTO t ... ;  →  INSERT INTO t ... ON CONFLICT DO NOTHING;
	// The statement may or may not end in ';' and may carry trailing whitespace.
	if idx := strings.Index(q, "INSERT OR IGNORE INTO "); idx >= 0 {
		q = strings.Replace(q, "INSERT OR IGNORE INTO ", "INSERT INTO ", 1)
		trimmed := strings.TrimRight(q, " \n\t")
		hadSemi := strings.HasSuffix(trimmed, ";")
		trimmed = strings.TrimSuffix(trimmed, ";")
		q = trimmed + " ON CONFLICT DO NOTHING"
		if hadSemi {
			q += ";"
		}
	}

	// ? → $1..$n, skipping '...' string literals so a literal '?' survives.
	var b strings.Builder
	b.Grow(len(q) + 16)
	n := 0
	inStr := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'':
			inStr = !inStr
			b.WriteByte(c)
		case c == '?' && !inStr:
			n++
			b.WriteByte('$')
			b.WriteString(itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// isPostgresRetryable: 40001 serialization_failure, 40P01 deadlock_detected.
func isPostgresRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}

// keep the sqlite driver import meaningful for the sqlite dialect's callers
// (isBusy lives in store.go and uses sqlitedriver.Error).
var _ = sqlitedriver.Error{}

// ── handles ────────────────────────────────────────────────────────────────

// dbHandle is *sql.DB behind the dialect rewrite. Only the methods the store
// uses are exposed, so a new call site cannot bypass the rewrite by accident.
type dbHandle struct {
	db *sql.DB
	d  *dialect
}

func (h *dbHandle) Exec(q string, args ...any) (sql.Result, error) {
	return h.db.Exec(h.d.rewrite(q), args...)
}
func (h *dbHandle) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return h.db.ExecContext(ctx, h.d.rewrite(q), args...)
}
func (h *dbHandle) Query(q string, args ...any) (*sql.Rows, error) {
	return h.db.Query(h.d.rewrite(q), args...)
}
func (h *dbHandle) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return h.db.QueryContext(ctx, h.d.rewrite(q), args...)
}
func (h *dbHandle) QueryRow(q string, args ...any) *sql.Row {
	return h.db.QueryRow(h.d.rewrite(q), args...)
}
func (h *dbHandle) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return h.db.QueryRowContext(ctx, h.d.rewrite(q), args...)
}
func (h *dbHandle) Begin() (*txHandle, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return nil, err
	}
	return &txHandle{tx: tx, d: h.d}, nil
}
func (h *dbHandle) BeginTx(ctx context.Context, opts *sql.TxOptions) (*txHandle, error) {
	tx, err := h.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &txHandle{tx: tx, d: h.d}, nil
}
func (h *dbHandle) Conn(ctx context.Context) (*sql.Conn, error) { return h.db.Conn(ctx) }
func (h *dbHandle) Close() error                                { return h.db.Close() }
func (h *dbHandle) SetMaxOpenConns(n int)                       { h.db.SetMaxOpenConns(n) }
func (h *dbHandle) PingContext(ctx context.Context) error       { return h.db.PingContext(ctx) }

// txHandle is *sql.Tx behind the same rewrite.
type txHandle struct {
	tx *sql.Tx
	d  *dialect
}

func (t *txHandle) Exec(q string, args ...any) (sql.Result, error) {
	return t.tx.Exec(t.d.rewrite(q), args...)
}
func (t *txHandle) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, t.d.rewrite(q), args...)
}
func (t *txHandle) Query(q string, args ...any) (*sql.Rows, error) {
	return t.tx.Query(t.d.rewrite(q), args...)
}
func (t *txHandle) QueryRow(q string, args ...any) *sql.Row {
	return t.tx.QueryRow(t.d.rewrite(q), args...)
}
func (t *txHandle) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, t.d.rewrite(q), args...)
}
func (t *txHandle) Commit() error   { return t.tx.Commit() }
func (t *txHandle) Rollback() error { return t.tx.Rollback() }

// dialect returns the store's SQL flavor; a zero-value store (tests build
// &SQLiteStore{} literals) is SQLite, the engine the code was written for.
func (s *SQLiteStore) dialect() *dialect {
	if s == nil || s.d == nil {
		return sqliteDialect
	}
	return s.d
}
