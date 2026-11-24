# Report: is the M0 wake-latency slice already on `main`?

**Task:** `ra-m0-fabric-baseline` · **Author:** ra · **Date:** 2026-11-24 · **Checked against:** `origin/main` at `3dea7e29`
**Type:** report only. No code changed. This does not close the task.

## Question

The missing slice was recorded as: expose `LeaseUpdated` on the `Item` struct / `itemCols` so a read-only wake-latency report can be produced. Is that, or an equivalent, now on `main`?

## Answer

**The report exists on `main`, but not by the route the slice described, and that route was deliberately rejected.** `lease_updated` is intentionally **not** exposed on `Item`/`itemCols`. The report instead reads a new column, `items.first_claimed_at` (schema v27), through a dedicated store method.

Why `lease_updated` was the wrong source: `RenewLease` and `StartWork` overwrite it, so any latency computed from it grows with every renew (`lease.go:470`, `lease.go:479`). `first_claimed_at` is written once by the first claim.

## Evidence (file:line on `origin/main`)

| fact | where |
|---|---|
| `itemCols` does not contain any lease column | `internal/routerstore/items.go:15` |
| a test fixes that exclusion (`lease_updated`, `first_claimed_at` on the not-in-Item list) | `internal/routerstore/store_test.go:626` |
| column added, no backfill (rows claimed before v27 stay empty) | `internal/routerstore/store.go:842-845` |
| set once by the first claim, never overwritten (`CASE WHEN first_claimed_at=''`) | `internal/routerstore/lease.go:240` (`ClaimNext`), `internal/routerstore/lease.go:326` (`ClaimItem`) |
| renew and start-work touch only `lease_updated` | `internal/routerstore/lease.go:470`, `:479` |
| sample type and query (`opened` to `first_claimed_at`, excludes empty) | `internal/routerstore/lanestate.go:192-240` |
| on the Store interface, with the note that the columns stay out of `Item`/`itemCols` | `internal/routerstore/store_iface.go:194-199` |
| served to nodes and generated on the remote client | `internal/routerstore/serve.go:136`, `internal/routerstore/remote_gen.go:390` |
| CLI report (`sirsi router wake-latency`, `--agent`, `--limit`, `--json`; aggregates p50/p95/max) | `cmd/sirsi/routerwakelatencycmd.go:31-72` |
| tests: ignores StartWork/Renew, survives requeue/reclaim, excludes pre-v27 rows, `ClaimItem` sets it | `internal/routerstore/wakelatency_firstclaim_test.go:14,43,71,95`; `internal/routerstore/lanestate_test.go:128` |
| changelog narrative (v27, apply Postgres schema before deploy) | `CHANGELOG.md:216-222`, `CHANGELOG.md:372-375` |

## Residual gaps (found while checking, not fixed here)

1. **Postgres has the column but no reader.** `internal/routerstore/pg/schema.sql:288-290` adds `first_claimed_at`; a search of `origin/main` for `WakeLatency` finds no implementation under the Postgres backend and no claim `UPDATE` there that sets the column. If the service runs on Postgres, `wake-latency` served from it needs verification. Not verified here; I did not run the report.
2. **Pre-v27 items have no sample by design**, so the baseline starts at the v27 deploy date, not earlier.
3. **No M0 baseline numbers are recorded in this check.** `docs/router-service/M0_BASELINE.md:4` references estimator scripts; whether the live baseline has been captured with the new report is not established here.

## What this means for `ra-m0-fabric-baseline`

The instrument is present; the task should not be reopened to add `LeaseUpdated` to `Item`. What remains is (1) confirm the Postgres path, and (2) run `sirsi router wake-latency --json` against the live service and record the result. Both are outside this documents-only change.
