# Router full-state recovery plan

Owner: Ra. Supporting: SSA, SHA. Status: plan plus two source fixes. No backup,
restore, export, credential action or production change was made to write it.
Answers SSA 20261106-164335, -164621, -165520, -171609, -175007 and SHA
20261109-082507.

## 1. What has to come back

| State | Where it lives | In the Cloud SQL backup? |
|---|---|---|
| Items, tasks, receipts, Ma'at reservations (`state` rows), agents, breakers, quotas, counters, identifiers, requirements, wake events | Postgres `router` database on Cloud SQL `sirsi-router` | Yes |
| Threads, sessions, lease sessions | same database | Yes, but stale on arrival (section 4) |
| Host tokens and their lane scopes (`host_tokens`, `host_token_scopes`) | same database | Yes, but authority must be re-checked (section 4) |
| Gate diagnostic logs (`audience_log`, `task_lease_log`) | same database | Yes; diagnostic only, not needed to recover |
| Service bootstrap token, database DSN | Secret Manager (`sirsi-router-bootstrap-token`, `sirsi-router-service-dsn`) | No. Secret Manager keeps its own versions |
| Service code and config | Cloud Run revisions; source on `origin/main` plus the stage branch | No. Rebuilt from git |
| Relay spool and outbox (requests a Mac accepted but has not yet delivered) | Files on each Mac (`~/.sirsi/...` relay directories) | No |
| Per-Mac session files and local registry caches | Files on each Mac | No; re-minted on demand |

## 2. Backups today (metadata readback, 2026-11-10)

Read with `gcloud sql instances describe` and `gcloud sql backups list`. Nothing
was read from the database itself.

- Instance `sirsi-router`, project `sirsi-nexus-live`, `us-central1-a`, POSTGRES_16,
  ENTERPRISE edition, ZONAL, state RUNNABLE, deletion protection on.
- Automated backups: on, daily at 08:00 UTC, 7 kept. Seven successful backups,
  2026-11-04 through 2026-11-10 08:00Z.
- Point-in-time recovery: on, 7 days of transaction logs kept in Cloud Storage.
  The recovery window is therefore the last 7 days, to the second.
- Databases: `postgres`, `router`.

Earlier, SSA's principal got a 403 on the backup list. Ra's deploy principal
can read the metadata. Who may *restore* has not been tested and is not claimed.

## 3. What a "complete" recovery point means

A Postgres point-in-time restore gives one consistent cut of everything in the
first table of section 1. It does **not** include the Mac relay spools. A shared
timestamp does not join them (SSA 175007 point 1). So:

- **Database-only recovery** is always available for the 7-day window. It is
  labelled *incomplete* whenever any Mac held undelivered relay requests at the
  chosen time.
- **Complete recovery** may be claimed only for a point at which every relay
  spool and outbox was empty. The proof is: the fabric quarantine marker was set
  (`sirsi router quarantine`), each Mac's relay reported zero pending and zero
  inflight, and the restore time is after both. Without that proof, no
  complete-recovery claim is made.
- Spool requests created after the restore point are replayed from the Macs only
  if they carry their original idempotency keys. Anything else is reported, not
  replayed.

## 4. Restore procedure (never in place)

1. Clone to a new, inactive instance at the chosen time:
   `gcloud sql instances clone sirsi-router sirsi-router-restore-<date> --point-in-time=<RFC3339>`.
   The live instance is never overwritten.
2. Prove the clone with `CanonicalDump` hashes (`internal/routerstore/migratestore.go`).
   The manifest now includes `host_token_scopes`, and a test fails if any future
   table is left out without a stated reason.
3. **Tokens: carry later revocations forward** (SSA 175007 point 2). For every
   token, compare `revoked` between the clone and the newest readable source
   (live instance, or the newest backup). A token revoked in either is revoked in
   the clone. If no later source can be read, fail closed: revoke every token in
   the clone and reissue each Mac's token through the existing mint job.
4. **Sessions: never revived.** Delete every row from `sessions` and
   `lease_sessions` in the clone before activation. Clients re-mint sessions with
   their tokens, so a session that was revoked after the restore point cannot
   come back.
5. **Scoped tokens:** see section 5. Activation is refused while any token's
   scope is unresolved.
6. Activate by pointing a new Cloud Run revision at the clone's DSN (a new
   Secret Manager version), tag it at 0%, read back with the probe, then move
   traffic. The old revision stays as rollback.

## 5. Token-scope ambiguity plan (no action taken)

Background: no `host_token_scopes` rows means an unscoped token. Two source
gaps could leave a scoped token with no rows:

- (a) The old migration manifest omitted `host_token_scopes`. It is now in the
  manifest.
- (b) `MintScopedHostToken` wrote the token and its scopes in separate
  statements, so a crash or a concurrent backup could catch the token without
  its scopes. The mint is now one transaction.

To classify existing tokens, for each non-revoked token with no scope rows:

| Evidence | Class |
|---|---|
| Host is a fleet Mac in `machines.json` and the label matches the cut-over mint | Intentional unscoped Mac token |
| Label or mint record says cloud/sandbox/lane, or the host is not a fleet Mac | Ambiguous: treat as needing scope |
| No mint record at all | Ambiguous |

For an ambiguous token, the safe order is: name the consumer (from
`audience_log` host and agent), agree the intended scope with that consumer's
owner, mint a correctly scoped replacement, move the consumer to it, then revoke
the old token. The rollback risk is that the consumer loses access, and moving
one consumer at a time bounds it. Nothing is inferred, issued or revoked under
this plan without that per-token agreement.

## 6. Owner board identity

The board acts as `board:<email>` only after Google IAP has verified the
signed-in account, and the service refuses any other owner actor
(`serve.go` ownerTerminals). Historical Firebase UIDs are not proof of the IAP
email mapping. The mapping is checked when the owner signs in to the board and
sees their own account in the actor field. Reading the IAP policy would expose
account emails, so that check stays in the owner's private channel and is not
recorded here.

## 7. Still open

- Who may restore has not been tested (no restore was run).
- Relay spool emptiness has no single readback command yet. Today it is checked
  per Mac.
