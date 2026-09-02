package routerstore

import (
	"database/sql"
	"os"
	"testing"
)

// SIRSI_TEST_PG_DSN switches the shared test helpers to the Postgres
// backend (ADR-062 rs-07: one suite, two drivers). Unset → SQLite, unchanged.
// The scratch server used on this workstation:
//
//	SIRSI_TEST_PG_DSN='postgres://sirsi@127.0.0.1:54329/routerstore_test' go test ./internal/routerstore/
//
// Each test gets a fresh schema: roles (idempotent) → DROP SCHEMA router
// CASCADE → pg/schema.sql, applied through the embedded copy so the test runs
// against exactly the schema this binary carries.
func pgTestDSN() string { return os.Getenv("SIRSI_TEST_PG_DSN") }

func resetPostgres(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("pg reset open: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range []string{PostgresRolesSQL, `DROP SCHEMA IF EXISTS router CASCADE;`, PostgresSchemaSQL} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("pg reset: %v", err)
		}
	}
}

// openBackendStore opens the store for the backend under test. Postgres when
// SIRSI_TEST_PG_DSN is set, else the SQLite path given.
func openBackendStore(t *testing.T, sqlitePath string) *SQLiteStore {
	t.Helper()
	if dsn := pgTestDSN(); dsn != "" {
		resetPostgres(t, dsn)
		s, err := OpenPostgres(dsn)
		if err != nil {
			t.Fatalf("OpenPostgres: %v", err)
		}
		return s
	}
	s, err := OpenPath(sqlitePath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	return s
}
