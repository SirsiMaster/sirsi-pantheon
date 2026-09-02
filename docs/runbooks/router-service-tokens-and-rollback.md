# Runbook — router service: tokens, revocation, rollback

*ADR-062. For the operator of `sirsi router serve`. Every command here runs
on the service host against the service's own backend (`--store`); none of it
is reachable over the wire.*

## Tokens

| Task | Command |
|---|---|
| Mint a token for a machine | `sirsi router token mint <hostname> --label "<what it is>" --store <backend>` — prints `SIRSI_ROUTER_TOKEN=…` **once**; only its hash is stored |
| List tokens | `sirsi router token list --store <backend>` (ids, hosts, labels, state; never plaintext) |
| Revoke a machine | `sirsi router token revoke <token-id> --store <backend>` — that host's next request fails; every session minted under that host is revoked; no other host notices |
| Rotate a machine | mint the new token, install it on the machine (`SIRSI_ROUTER_TOKEN`), confirm `sirsi router status` works, then revoke the old id |

`<hostname>` must equal what the machine's `hostname` prints: a per-host token
can only mint sessions for its own host (`ErrHostMismatch`).

The **bootstrap token** (`--token-env`, default `SIRSI_ROUTER_SERVE_TOKEN`)
can mint sessions for any host. Keep it in Secret Manager and your keychain
only; never install it on a node. Rotate it by restarting the service with a
new value; nothing on the nodes changes.

## Sessions

Nodes mint their own sessions on first use and cache them at
`~/.sirsi/sessions/<agent>.json` (0600). A session is bound to the machine's
token host and to the SHA-256 of its `sirsi` binary; a new binary mints a new
session automatically. To force a machine to re-enrol, revoke its token and
mint a new one. To inspect: `SELECT host, agent, created, revoked FROM
sessions` on the backend.

## Rollback — a node

Unset `SIRSI_ROUTER_URL` (and `SIRSI_ROUTER_TOKEN`) on the machine. Every verb
returns to the local `~/.sirsi/router.db`. Nothing is copied back: whatever
the node wrote while on the service stays on the service. Rehearse it once per
machine before cut-over and note the time.

## Rollback — the service (self-hosted)

Stop the new binary, start the previous one against the same `--store`. The
schema is versioned (`router.schema_version` on Postgres, `PRAGMA
user_version` on SQLite); a binary that does not understand the stored
version refuses to open it rather than degrade.

## Rollback — Cloud Run (Phase D)

`gcloud run services update-traffic sirsi-router --to-revisions <previous>=100
--project sirsi-nexus-live`. The previous revision's digest is on the deploy's
audit receipt item in the ledger. Rehearse before the first cut-over (rs-17).

## Outage behaviour

If the database is unreachable the service answers **503** and nodes back off
and retry with their existing sessions; nobody re-enrols. A lease held across
an outage stays valid until its TTL; a completion that failed during the
outage is retried by the worker. If a holder never completes, the item
reopens at TTL and another node may take it — that is recovery, not a
duplicate.

## Migration (SQLite → the service backend)

See `docs/user-guides/router-service.md` § "Migrating an existing ledger":
`sirsi router migrate-store --from … --to … [--dry-run] [--scrub-nul]`.
It quiesces the fabric itself and releases the marker only when it exits.
