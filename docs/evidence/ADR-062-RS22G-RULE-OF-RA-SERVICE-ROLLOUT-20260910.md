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

## First audit signal (Cloud Logging, revision 00008, 16:14:30Z → 16:25Z)

```
44 rule-of-ra: WOULD REFUSE …
   42 Mac@Mac ReconcileOperationalState
    1 Mac@Mac CompleteTaskLease
    1 Mac@Mac Backfill
 0 from any lane
```

`Mac@Mac` is the M5 supervisor session (no `SIRSI_AGENT_ID`, no thread) — horus itself. That is
the rs-22h work item (launchers and the supervisor register first), and it is the reason enforce
waits for the 20b.3 audit rather than a log grep.

Positive control: Ra's own session on the M1, started with `SIRSI_THREAD_ID=thr-df2a8cd5b1c61290`,
now carries `thread_id` in `~/.sirsi/sessions/ra.json` (minted through `MintSessionForThread`).

## Reproduce

- Gate behaviour: `go test ./internal/routerstore -run 'RuleOfRa|ThreadAuthority|Competing'`.
- Existing-ledger upgrade: `PATH=/opt/homebrew/opt/postgresql@16/bin:$PATH bash scripts/router-service/test-apply-schema-upgrade.sh`.
- Live signal: `gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="sirsi-router" AND textPayload:"WOULD REFUSE"' --project=sirsi-nexus-live --limit=200`.
