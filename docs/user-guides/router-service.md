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

## Security model

Every call a node makes carries five proofs, checked in this order:

1. **Host** — the bearer token (`SIRSI_ROUTER_TOKEN`). Wrong or missing → 401.
2. **Session** — a session id the service minted for this host + agent +
   binary. The node mints one automatically on first use and caches it at
   `~/.sirsi/sessions/<agent>.json` (mode 0600).
3. **Freshness** — a nonce with a millisecond timestamp, accepted only inside
   ±60 s and only once. Keep node clocks within a minute of the service.
4. **Signature** — HMAC of the method, nonce and body with the session secret.
5. **Runtime** — the SHA-256 of the `sirsi` binary, bound when the session was
   minted. A different binary presenting the same session is refused and the
   session is revoked; the node then mints a new one for the new binary.

Then **ownership**: a lease can only be completed, failed, blocked, started or
renewed by the session that claimed it. A copied lease token from another
process is refused with `ErrNotOwner`.

## Per-host tokens

The `--token-env` secret is the **bootstrap** token: it can mint sessions for
any host. Give each machine its own token instead, and hand the bootstrap
secret to nobody:

```bash
# on the service host, against the service's own backend
sirsi router token mint m1-backup --label "M1 backup Mac" --store ~/.sirsi/router.db
#   token id: 9335d398…  host: m1-backup
#   SIRSI_ROUTER_TOKEN=<printed once — copy it to that machine>

sirsi router token list   --store ~/.sirsi/router.db
sirsi router token revoke 9335d398328a9152 --store ~/.sirsi/router.db
```

A per-host token can only mint sessions for the host it was minted for, so a
machine cannot claim to be another. Revoking a token also revokes every session
minted under that host; the machine is refused on its very next request, and no
other machine notices. The host name a node presents is its `hostname`.

Adding a machine is therefore: mint a token, set `SIRSI_ROUTER_URL` and
`SIRSI_ROUTER_TOKEN` on it, and `sirsi thread register`.

Still to come: a release-manifest check of the runtime hash (Phase D). Keep the
service on Tailscale or a VPC until the hosted deployment lands.

## Migrating an existing ledger

Not yet. `sirsi router migrate --to <url>` (Phase C, rs-12) will quiesce the
fabric, dump the local file canonically, dry-run, import, diff, and prove a
second import is a no-op. Until then a Postgres service starts empty.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `SIRSI_ROUTER_URL … is set but SIRSI_ROUTER_TOKEN is empty` | export the token on the node |
| `HTTP 401` | token differs from the service's `--token-env` value, or the nonce is outside ±60 s (check the clock), or the session was revoked |
| `postgres ledger has no router.schema_version` | apply `internal/routerstore/pg/schema.sql` as `router_migrator` first |
| `this host has no local ledger of record` | expected: a node on the service refuses local-file diagnostics |
