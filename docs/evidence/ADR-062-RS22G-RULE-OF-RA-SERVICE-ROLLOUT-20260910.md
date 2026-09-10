# ADR-062 rs-22g — the Rule of Ra in service, log mode (2026-09-10)

<!-- agent: ra | workstream: router-service (ADR-062) | evidence for ledger row rs-22g-rule-of-ra-service -->

Claim scope: the service gate is deployed and observing. Not claimed: enforce mode, whole-fleet
registration, G7 closure, or the 20b.3 audit (PR #726, in review).

## What landed

| Step | Artifact | Where |
|---|---|---|
| Code | PR #724 (SSA-bound at a4dfe4f5 after three review rounds: r1 schema/authority/mode, r2 host-adoption race, r3 ACCEPT) | merged 20c9db60 |
| CI | PR #727 — the Test job's Postgres leg now runs on the M1 runner (`LC_ALL=C`) | merged f1a98216 |
| Schema | apply-schema job execution `sirsi-router-apply-schema-4bqj2`, one transaction, version row last | job output: `tables=15 triggers=12 partial=5 version=19` |
| Service | Cloud Run revision `sirsi-router-00008-dtt` (image `…/sirsi-router@sha256:800673b9…`), gate mode default `log` | 100% traffic since 2026-09-10T16:15Z |
| Nodes | `sirsi` rebuilt from 20c9db60 on the M1 and the M5 (rm-then-copy into `~/.local/bin/sirsi`); M5 relay, seven wake loops and horus restarted (`launchctl kickstart -k`), all up | `launchctl list` on the M5 |

## Two things the rollout found

1. **Traffic was pinned.** `gcloud run deploy` created revision 00008 but reported "revision
   00007-dqm … serving 100 percent" — the rs-19 rollback rehearsal had left a named-revision pin in
   `spec.traffic`. The relay log showed every `MintSessionForThread` from the new binaries answered
   `404` (old revision) until `gcloud run services update-traffic sirsi-router --to-latest`. Runbook
   rule: read `spec.traffic` after every deploy.
2. **The privilege audit is red on the live ledger** (job step after the schema applied):
   `router_service … super/createrole/createdb/bypassrls=[false true true false]`. Cause: Cloud SQL
   created the users with CREATEROLE/CREATEDB and `roles.sql` only set attributes inside a guarded
   CREATE; a second defect compared the booleans against `f f f f` while Postgres renders
   `true`/`false`. Fix: PR #728 (in review). The v19 schema itself is on the ledger.

## First audit signal — Ra-reported, raw receipt retained

Query (Cloud Logging, run from the M5 with `gcloud` as `claude-agent@sirsi-nexus-live`, captured
2026-09-10T16:25:09Z, i.e. after the window closed; `--limit=5000`, 466 rows returned, so no
truncation):

```
gcloud logging read 'resource.type="cloud_run_revision"
  AND resource.labels.service_name="sirsi-router"
  AND resource.labels.revision_name="sirsi-router-00008-dtt"
  AND textPayload:"rule-of-ra: WOULD REFUSE"
  AND timestamp>="2026-09-10T16:14:30Z" AND timestamp<"2026-09-10T16:25:00Z"' \
  --project=sirsi-nexus-live --limit=5000 --order=asc --format='value(timestamp,textPayload)'
```

Raw receipt: `ADR-062-RS22G-RULE-OF-RA-SERVICE-ROLLOUT-20260910/would-refuse-00008-dtt-16h14m30-16h25.log`
(466 lines, first 16:14:32.314Z, last 16:24:58.768Z, sha256
`122d869dab873df6a7030f57418eef202608c0e715d4b0f99b52b52fbaf0e043`).

```
466 rule-of-ra: WOULD REFUSE in [16:14:30Z, 16:25:00Z)
  434 Mac@Mac                ReconcileOperationalState
   11 Mac@Mac                Backfill
    4 Mac@Mac                CompleteTaskLease
    3 Mac@Mac                SendGuarded
    2 Mac@Mac                CloseItem
    2 Mac@Mac                ClaimTask
    3 MacBookPro@MacBookPro  SendGuarded
    3 MacBookPro@MacBookPro  ClaimTask
    2 MacBookPro@MacBookPro  ReclaimExpiredTaskLeases
    1 MacBookPro@MacBookPro  CompleteTaskLease
    1 MacBookPro@MacBookPro  CloseItem
    0 from any lane session (codex-*, claude-*, sirsi-software-admin)
```

`Mac@Mac` is the M5 supervisor session (no `SIRSI_AGENT_ID`, no thread) — horus.
`MacBookPro@MacBookPro` is Ra's own interactive shell on the M1 issuing `sirsi router` verbs
without `SIRSI_AGENT_ID`/`SIRSI_THREAD_ID` in its environment. Both are the rs-22h work item
(launchers and shells register first); both are why enforce waits for the 20b.3 audit rather
than a log grep.

Positive control (Ra-reported): Ra's session on the M1 started with
`SIRSI_THREAD_ID=thr-df2a8cd5b1c61290` carries `thread_id` in `~/.sirsi/sessions/ra.json`
(minted through `MintSessionForThread` against revision 00008).

## What this document does not establish

- The `launchctl list` and relay-log lines in the table above are Ra-reported from an ssh
  session; a process listing does not show per-lane delivery. Delivery evidence stays with
  the rs-22e receipt and the 20b.3 audit.
- PR #728 is a proposed correction to the privilege audit, not verified privilege closure;
  SSA returned a production-caller authority finding on its first head (62a183db).
- Enforce mode, whole-fleet registration, G7 closure.

## Reproduce

- Gate behaviour: `go test ./internal/routerstore -run 'RuleOfRa|ThreadAuthority|Competing'`.
- Existing-ledger upgrade: `PATH=/opt/homebrew/opt/postgresql@16/bin:$PATH bash scripts/router-service/test-apply-schema-upgrade.sh`.
- Live signal: the bounded query above; change the timestamp bounds for a new window.
