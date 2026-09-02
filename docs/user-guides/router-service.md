# The router service — one ledger for every machine

*ADR-062. Status: implemented for a single host or a self-hosted service; the
hosted Ra deployment (Cloud Run + Cloud SQL) is Phase D and needs its own owner
gate. Nothing in this guide changes how a single Mac works by default.*

## What it is

Today every `sirsi router` verb opens `~/.sirsi/router.db` on the machine it
runs on. That is fine for one Mac (the **Anubis** shape). It cannot be shared:
two machines cannot safely write one SQLite file, and copying the file between
them loses claims.

The router service turns the ledger into something every machine talks to over
HTTPS. One process, `sirsi router serve`, owns the database; every other node
points at it with one environment variable and one token. Agents on every host,
Claude or Codex, then claim, work and close items on the **same** ledger at the
same time. That is the **Ra** shape.

## Running the service

```bash
# a SQLite-backed service on one always-on Mac
export SIRSI_ROUTER_SERVE_TOKEN='<a long random secret>'
sirsi router serve --store ~/.sirsi/router.db --listen 0.0.0.0:8080

# a Postgres-backed service (the Ra ledger); schema applied first as router_migrator
export SIRSI_ROUTER_SERVE_TOKEN='<secret>'
sirsi router serve --store 'postgres://router_service@db-host:5432/router' --listen :8080
```

| Flag | Meaning |
|---|---|
| `--store` | `postgres://…` DSN, or a SQLite path. Required. |
| `--listen` | address to bind (default `:8080`); `$PORT` overrides it on Cloud Run |
| `--token-env` | which env var holds the bearer token (default `SIRSI_ROUTER_SERVE_TOKEN`) |
| `--tls-cert` / `--tls-key` | serve TLS yourself; leave unset behind Cloud Run |
| `--max-wait` | ceiling for a `router wait` long-poll (default 60s) |

The service refuses to start without a token. `GET /healthz` answers `ok`.

## Pointing a node at it

```bash
export SIRSI_ROUTER_URL='https://router.example.net'
export SIRSI_ROUTER_TOKEN='<the same secret>'
sirsi router status        # now reads the shared ledger
sirsi router pull ra       # claims come from the shared ledger
```

Every existing verb works unchanged — `send`, `pull`, `wait`, `claim`, `close`,
`respond`, `ledger`, `task`, `board`. Under the hood `routerstore.Resolve()`
returns an HTTP client that implements the same `Store` interface the local
file did, so nothing above it can tell the difference.

To take a node back to its local file, unset `SIRSI_ROUTER_URL`. That is the
whole rollback.

## What changes on a node once the URL is set

- **It has no local ledger of record.** `sirsi schema-check` and the self-update
  backup refuse rather than touch a file that is no longer the truth. A set URL
  with no token is an error, never a silent fallback to the local file.
- **`router wait` is a long-poll.** The node holds an HTTPS request open up to
  `--max-wait` and returns when work lands; it does not poll every second.
- **Every call is bounded** (5 s by default, longer only for `wait`) and retried
  with backoff on engine contention, so a slow service backpressures the wake
  loop instead of piling up claims.

## Security floor and what is still to come

Today: one shared bearer token, compared in constant time, required on every
call; TLS from Cloud Run or your own certificate. Coming in Phase C (ADR-062 §3):
per-host tokens that can be revoked individually, service-minted sessions bound
to the host, the binary's signature and the agent id, a signed 60-second nonce
on every mutation, and ownership checks on every lease and write. Do not expose
the service to the public internet before that lands; keep it on Tailscale or a
VPC.

## Migrating an existing ledger

Not yet. `sirsi router migrate --to <url>` (Phase C, rs-12) will quiesce the
fabric, dump the local file canonically, dry-run, import, diff, and prove a
second import is a no-op. Until then a Postgres service starts empty.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `SIRSI_ROUTER_URL … is set but SIRSI_ROUTER_TOKEN is empty` | export the token on the node |
| `HTTP 401` | token on the node differs from the service's `--token-env` value |
| `postgres ledger has no router.schema_version` | apply `internal/routerstore/pg/schema.sql` as `router_migrator` first |
| `this host has no local ledger of record` | expected: a node on the service refuses local-file diagnostics |
