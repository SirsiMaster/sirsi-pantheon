# Router service — developer README

Source map:

| Path | What it is |
|---|---|
| `internal/routerstore` | The `Store` interface and its backends: SQLite, Postgres, and the HTTP/spool `Remote` client. Only `routerstore.Resolve()` may open a store in production code. |
| `internal/dispatch` | The facade used by the CLI: send, pull, claim, close, reassign. |
| `internal/router` | Registry, threads, wake loop, lane state, consumer slots, quarantine. |
| `cmd/sirsi/router*cmd.go` | The `sirsi router` verbs, including `serve`, `token`, `relay`. |
| `pg/roles.sql`, `pg/schema.sql` | Postgres schema and the least-privilege service role. |

Rules that bind a change here:

- A new Store method needs an interface entry, a Remote stub, and a service deploy before any client uses it.
- Every Store test runs against SQLite and Postgres 16 (`scripts/ci-postgres.sh`; set `SIRSI_TEST_PG_DSN` to run it locally).
- `scripts/check-router-store-open.sh` fails the build on a direct store open; `--self-test` proves it goes red.
- Run tests with `go test -race -short` and a user-owned `TMPDIR`.

Further reading: `docs/ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md`, `docs/ROUTER_SERVICE_GOAL.md`, `docs/router-service/IDENTITY_ARCHITECTURE.md`, and the user guide and runbook in this directory.
