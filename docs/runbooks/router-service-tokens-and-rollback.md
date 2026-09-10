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

A cut-over node is marked by `~/.sirsi/router-service.env` (written by
`scripts/router-service/cutover-m5.sh` step 6 and sourced from `~/.zshenv`).
While that file exists, a process without `SIRSI_ROUTER_URL` is **refused** by
`routerstore.Resolve()` — it does not fall back to the local file. That is
deliberate: a GUI-launched process read a frozen `router.db` as a live inbox on
2026-09-10, and a newer binary would have created an empty ledger and split the
fabric. Unsetting the two variables is therefore not a rollback.

Rolling a node back is a deliberate procedure, in this order:

1. Stop or drain the node's wake loops and horus (their plists carry the env).
2. Put the retained local file back at the path FIRST: on the M5 the cut-over
   left `~/.sirsi/router.db.frozen-<date>` and an unopenable directory at
   `~/.sirsi/router.db` (so no process can create a new file there). Remove the
   directory, copy the frozen file into place, `chmod u+w`, and
   `sqlite3 ~/.sirsi/router.db "PRAGMA journal_mode=wal;"`.
3. Restore the pre-cut-over binary if the schema requires it
   (`~/.sirsi/build/sirsi-prev` on the M5; the retained file is schema 16).
4. Only now move the marker aside: `mv ~/.sirsi/router-service.env
   ~/.sirsi/router-service.env.rolled-back-<date>` and remove the source line from
   `~/.zshenv`; confirm the marker is gone. A host is never left with neither a
   service env nor an openable ledger. Open shells keep their variables until
   they `unset SIRSI_ROUTER_URL SIRSI_ROUTER_TOKEN`.
5. Verify: `sirsi router status` from a fresh shell shows the frozen counts.

**Retained-data boundary:** nothing is copied back. The local file holds exactly
what the node had at the freeze; everything written on the service after that
stays on the service (a service→local export does not exist — rs-20b). Rehearsed
on the M5 2026-09-10: 0.03 s out, 0.57 s back, data-level dump hash identical
(`docs/evidence/ADR-062-RS20-CUTOVER-EVIDENCE-20260910.md`). `scripts/router-service/cutover-m5.sh rollback` performs steps 2–4 on both
Macs in that order (restore first, marker last), refuses with exit 2 when the
placeholder directory has no frozen copy to restore, permits an absent marker but
propagates a present marker's rename failure and asserts its absence before
announcing success; `scripts/router-service/test-rollback-restore.sh` rehearses the restore
against disposable paths (restore, refusal, idempotence, marker absent, marker
present, injected rename failure). The live G8 rehearsal of
2026-09-10 predates the placeholder directory and covered the file-only case.

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

## Host env: the `~/.zshenv` source line is conditional
`~/.zshenv` sources `~/.sirsi/router-service.env` only when `SIRSI_ROUTER_URL` is unset. A lane that starts with
`env -u SIRSI_ROUTER_TOKEN SIRSI_ROUTER_URL=spool://…` keeps both choices in every shell it opens (codex runs its
commands through a login shell); otherwise the https URL and the host token come back into a sandbox that has no
DNS and must never hold the token (observed 2026-09-10 on codex-inference). Both Macs carry this form.
