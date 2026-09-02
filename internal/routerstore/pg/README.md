# routerstore/pg — the Postgres ledger schema (ADR-062)

**What this is.** The router ledger's Postgres schema: SQLite schema v16
(`../store.go` migrations 1..16) translated once, plus the ADR-062 §3 identity
columns. It is the `Ra` backend; `SQLiteStore` stays the `Anubis` backend.
Nothing here runs on a Mac — it is applied to Cloud SQL by the migrator role.

| File | Role | Run as |
|---|---|---|
| `roles.sql` | creates `router_migrator` (DDL owner) and `router_service` (DML only) | cluster admin, once |
| `schema.sql` | schema `router`, 11 tables + `schema_version`, indexes, 12 wake triggers, grants | `router_migrator` |

**Why one baseline and no migration chain.** A Postgres ledger is only ever
created by `sirsi router migrate` (rs-12) from a quiesced SQLite dump, so it
starts at the SQLite high-water mark. `router.schema_version` holds `16` so the
migration tool can compare it with SQLite's `PRAGMA user_version` directly.
Future schema changes add a Postgres migration alongside the SQLite one.

**Translation rules** (the dialect layer in rs-06 must agree with these):

| SQLite | Postgres |
|---|---|
| `INSERT OR IGNORE` | `INSERT … ON CONFLICT DO NOTHING` |
| `lower(hex(randomblob(16)))` | `router.rand_hex32()` |
| `strftime('%Y-%m-%dT%H:%M:%SZ','now')` | `router.now_rfc3339()` |
| `BLOB` | `BYTEA` |
| trigger `WHEN (… EXISTS(…))` | `EXISTS` moved into the PL/pgSQL body (Postgres forbids subqueries in trigger `WHEN`) |
| `?` placeholders | `$1..$n` (rewritten by the dialect layer, not in SQL text) |

Timestamps stay `TEXT` RFC3339 UTC on purpose: the store compares them as
strings and the migration diff (rs-12) is byte-for-byte.

**Identity columns (ADR-062 §3).** `items`, `tasks`: `host`, `user_id`,
`session`. `threads`: those plus `runtime_hash`, with a partial unique index on
`session`. They default to `''` so a migrated SQLite row is valid; the service
fills them from the authenticated session on every write (rs-10).

**Verification.** `scripts/check-pg-schema.sh` creates a throwaway database,
applies both files as the roles they will run as in production, and asserts:
12 tables, 12 distinct triggers, ≥5 partial indexes, version 16; an item insert
emits exactly one wake event and a duplicate `event_key` is ignored; a claim
acks the leased wake event; `router_service` cannot `CREATE TABLE`. Negative
control (2026-09-02): deleting one trigger from `schema.sql` makes it fail with
"expected 12 triggers, got 11". Local scratch server: PG 14 on `127.0.0.1:54329`,
user `sirsi`, see `docs/ROUTER_SERVICE_GOAL.md`.

**Dialect layer (rs-06).** `../dialect.go` applies the translation table above at
one seam (`dbHandle`/`txHandle`); `../open_postgres.go` opens a ledger over pgx.
Run the whole routerstore suite on Postgres with:

```bash
SIRSI_TEST_PG_DSN='postgres://sirsi@127.0.0.1:54329/routerstore_test' go test ./internal/routerstore/
```

**Known limitations.** There is no separate `PostgresStore` type — `SQLiteStore`
carries a dialect (the name predates rs-06; a rename is cosmetic and deferred). `SKIP LOCKED` claims are
a store-code concern, not schema. `rand_hex32()` is `md5`-based and not
cryptographic; it only needs uniqueness, as SQLite's `randomblob` did.
