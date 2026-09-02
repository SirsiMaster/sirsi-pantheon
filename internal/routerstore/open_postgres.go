package routerstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"
)

// PostgresSchemaSQL is pg/schema.sql, embedded so the migration tool (rs-12)
// and the dual-driver test suite apply exactly the schema this binary was
// built against. Production DDL still runs as router_migrator (pg/README.md).
//
//go:embed pg/schema.sql
var PostgresSchemaSQL string

// PostgresRolesSQL is pg/roles.sql (cluster-level, run once by an admin).
//
//go:embed pg/roles.sql
var PostgresRolesSQL string

// postgresSchemaVersion pairs with SQLite's PRAGMA user_version high-water
// mark that pg/schema.sql was translated from. OpenPostgres refuses any other
// value: a ledger this binary does not understand must never be written.
const postgresSchemaVersion = 17

// OpenPostgres opens the Ra ledger over a Postgres DSN (ADR-062 §1). The
// schema must already exist (applied by router_migrator); this never runs
// DDL. Like OpenPath it is for Resolve() and tests only — the direct-open
// gate (rs-04) forbids production callers.
//
// Pool is capped at the ADR-062 §2 per-node in-flight limit so a slow
// service backpressures instead of piling up connections.
func OpenPostgres(dsn string) (*SQLiteStore, error) {
	if !strings.Contains(dsn, "search_path") {
		// Every statement names bare tables; the schema is router.*.
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "search_path=router"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("routerstore: open postgres: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var version int
	if err := db.QueryRowContext(ctx, `SELECT version FROM router.schema_version`).Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("routerstore: postgres ledger has no router.schema_version (apply pg/schema.sql as router_migrator first): %w", err)
	}
	if version != postgresSchemaVersion {
		_ = db.Close()
		return nil, fmt.Errorf("routerstore: postgres ledger schema_version %d, this binary expects %d", version, postgresSchemaVersion)
	}

	escalationAgent := strings.TrimSpace(os.Getenv("SIRSI_ESCALATION_AGENT"))
	if escalationAgent == "" {
		escalationAgent = "owner"
	}
	return &SQLiteStore{
		db:              &dbHandle{db: db, d: postgresDialect},
		d:               postgresDialect,
		path:            "postgres",
		escalationAgent: escalationAgent,
	}, nil
}
