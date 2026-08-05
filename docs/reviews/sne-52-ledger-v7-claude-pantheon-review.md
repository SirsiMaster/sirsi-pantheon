<!-- REVIEW: claude-pantheon | SNE-52 ledger schema v7 | commit a4eb665d | 2026-08-05 -->
# SNE-52 Ledger Schema v7 — Parallel Review

**Reviewer:** claude-pantheon (parallel, bounded — requested by codex-inference)  
**Target:** `codex/router-unification-store-v7` @ `a4eb665d`  
**Contract:** `docs/ADR-054-CONTRACTS-IDENTITY-AND-LEDGER-V7.md` Part B  
**Date:** 2026-08-05  
**Verdict:** ✅ PASS — no required changes

---

## Checks run

| Check | Result |
|---|---|
| `gofmt -l` (changed files) | ✅ PASS — no formatting issues |
| `go build ./...` | ✅ PASS (only linker `-lobjc` duplicate warning, pre-existing) |
| `go vet ./internal/routerstore/... ./cmd/sirsi/...` | ✅ PASS |
| `go test ./internal/routerstore/... -race -count=1` | ✅ PASS |
| `TestTaskV4V5V6MigrateDirectlyToV7` (v4/v5/v6 → v7) | ✅ PASS |
| `TestTaskV7DefaultsAndDerivedLiveness` | ✅ PASS |
| `TestTaskV7EvidenceLinksAndAccounting` | ✅ PASS |
| `TestTaskV7AccountingConcurrentIncrements` (20-way) | ✅ PASS |
| `TestTaskV7StageRegressionAndCharterGovernance` | ✅ PASS |

---

## Part B contract verification

### B1 — New columns

All 10 columns in the contract are present in the migration, struct, INSERT, SELECT, and scanTask:

| Column | Schema default | Go default | Verified |
|---|---|---|---|
| `charter` | TEXT NULL | `nil` (`*string`) | ✅ |
| `commissioned_at` | NOT NULL DEFAULT '' (backfilled) | `t.Created` | ✅ |
| `commissioned_by` | NOT NULL DEFAULT '' (backfilled) | `t.Agent` | ✅ |
| `outline` | TEXT NULL | `nil` (`*string`) | ✅ |
| `timeline` | NOT NULL DEFAULT '[]' | `[]TimelineEntry{}` | ✅ |
| `links` | NOT NULL DEFAULT '[]' | `[]TaskLink{}` | ✅ |
| `liveness` | *derived, not stored* | derived in GetTask/ListTasks | ✅ |
| `test_state` | NOT NULL DEFAULT 'untested' | `"untested"` | ✅ |
| `stage` | NOT NULL DEFAULT 'spec' | `"spec"` | ✅ |
| `tokens_consumed` | INTEGER NOT NULL DEFAULT 0 | `0` | ✅ |
| `duration_seconds` | INTEGER NOT NULL DEFAULT 0 | `0` | ✅ |

### B2 — Liveness derived, not stored

`liveness` is absent from all INSERT and UPDATE SQL statements.  
Derived via `deriveTaskLiveness(t, now)` at the end of `GetTask` and each `ListTasks` iteration.  
Single exported constant `TaskStalledAfter = 4 * time.Hour` — shared by store and available to CLI/surfaces.  
Liveness logic matches contract exactly: `blocked` → `active` → `stalled` → `unknown`.  
Threshold boundary validated by test at exactly T+4h (expects `stalled`). ✅

### B3 — Accounting semantics (monotonic accumulators)

`tokens_consumed=tokens_consumed+?` and `duration_seconds=duration_seconds+?` in UPDATE SQL — additive at the database layer, safe under concurrent writers. Negative increment guard (`u.AddTokens < 0 || u.AddSeconds < 0`) fires before any accumulation. `commissioned_at` and `commissioned_by` are absent from the UPDATE SET clause — write-once enforced at SQL level. ✅

### B4 — CLI surface

All 8 new flags on `task update` are wired:
`--charter`, `--outline @file`, `--timeline @file`, `--link kind:label:url` (repeatable), `--test-state`, `--stage`, `--add-tokens`, `--add-seconds`.  
`task list --json` marshals `[]Task` which includes all v7 fields plus derived `liveness`. ✅

### B5 — What v7 does NOT do

No table splits, no foreign keys, no per-task history table in this commit. ✅

---

## Notable design choices (no changes required)

- **Charter amendment governance**: the check uses `u.Links` (incoming links in the current update), not `t.Links` (accumulated history). This correctly requires each charter amendment to carry its own `owner-instruction` — a historic instruction cannot authorize a new amendment. Tests explicitly cover this.
- **Stage regression requires subject replacement**: enforced as "incoming subject non-empty AND different from current subject", not just non-empty. Prevents a trivial whitespace-padded subject from passing the gate.
- **Migration version gap (v5/v6)**: the `schemaMigration{version, sql}` struct change is the right approach — the old slice-index scheme would have required fake no-op entries to hold the v5 and v6 positions. The migration loop correctly finds the next pending version by scanning forward from the current `user_version`.
- **`readTaskFile` enforces `@` prefix for `--outline` and `--timeline`**: prevents accidental raw-JSON injection; the `@file` contract from B4 is mechanically enforced by the CLI flag parser.

---

## Scope note

This review covers Part B only (ledger schema v7). Part A (identity enforcement) is out of scope for this bounded review item.

Refs: ADR-054-CONTRACTS-IDENTITY-AND-LEDGER-V7.md (Part B), codex/router-unification-store-v7 @ a4eb665d
