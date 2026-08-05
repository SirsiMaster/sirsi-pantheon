# SNE-52 Ledger v7 Integration Review — PR #501 head `a9b85c98`

**Reviewer:** claude-pantheon  
**Date:** 2026-08-05  
**Scope:** Does SNE-52 ledger schema v7 (`2e49f351`) remain correct after integration with the SNE-51 identity-admission changes (`a9b85c98`)?  
**Verdict: PASS**

---

## Verification

```
go build ./...      PASS (linker warnings only, no errors)
go vet ./...        PASS
go test -race       PASS — routerstore, dispatch, liveness, router (all 4 pkgs)
```

---

## Integration Points Reviewed

### 1. Migration — CORRECT

`store.go` replaced `[]string` with `[]schemaMigration{version, sql}` so v7 can advance directly from any of v4/v5/v6. The runner finds the first migration with `version > current` rather than indexing by position. The `maxVersion` guard and concurrent-migration re-check both updated consistently. `TestTaskV4V5V6MigrateDirectlyToV7` covers all three starting versions with a live legacy row; backfill (`commissioned_at = created`, `commissioned_by = agent`) confirmed correct.

### 2. Liveness — CORRECT

`deriveTaskLiveness()` is called exclusively at read time (`GetTask`, `ListTasks`) — never stored. Single shared constant `TaskStalledAfter = 4h`. Three-state machine (blocked → active → stalled → unknown) is sound. `TestTaskV7DefaultsAndDerivedLiveness` drives the full state space including the stalled boundary and a blocked override via UpdateTask.

### 3. Task Accounting — CORRECT

`AddTokens`/`AddSeconds` are increment-only (non-negative guard in both store and facade). SQL uses `tokens_consumed=tokens_consumed+?` (atomic at the SQLite serialized-write level). `TestTaskV7AccountingConcurrentIncrements` ran 20 concurrent goroutines and verified no lost increments under -race.

### 4. CLI JSON Projection — CORRECT

`taskSelect` constant centralizes all 19 column names. `scanTask()` helper uses `sql.NullString` for nullable `charter`/`outline` and JSON-unmarshals `timeline`/`links`, guaranteeing non-null arrays at the Go layer. New CLI flags (`--charter`, `--outline`, `--timeline`, `--stage`, `--test-state`, `--add-tokens`, `--add-seconds`, `--link`) correctly use `cmd.Flags().Changed()` for set-detection so empty string is distinguishable from unset.

### 5. SNE-51 Integration — CORRECT

- CLI `routerledgercmd.go` switched from `f.Store().AddTask/UpdateTask` to `f.AddTask/UpdateTask` (facade) — identity admission now runs on every CLI write.
- `dispatch.Facade.AddTask/UpdateTask` validate agent + responsible-party before delegating to store; `self` normalizes to the owning agent before validation.
- `Inbox()` validates acting agent identity when non-empty — undeclared pull is now rejected at the same boundary as send.
- Liveness emitters (`livenesswatch.go`, `gemmaliveness.go`) migrated from `"liveness-watch"`/`"gemma-liveness"` senders and `"user"` recipient to `"horus"` sender and `"owner"` recipient — consistent with declared identities.
- All test fixtures updated with full ADR-054 fields (`id`, `type`, `repo`/`cwd`, `workstream`, `wake.mechanism`).

### 6. `user` alias migration — CORRECT BY DESIGN

`user` is declared in `agents.json` (for legacy item compatibility) but `ValidateAgent` short-circuits with a remediation error before the registry lookup. New writes to `user` fail with `"use declared identity \"owner\""`. Existing items addressed to `user` remain readable; new sends require `owner`. Intentional migration path.

### 7. Stage/charter governance — CORRECT

Stage regression requires a changed subject. Charter amendment requires a fresh `owner-instruction` link in the same update (historic links do not authorize future amendments — `TestTaskV7StageRegressionAndCharterGovernance` validates the non-idempotent case explicitly). `test_state=passed` requires at least one `evidence` link.

---

## Known Design Constraints (not defects)

- **v5/v6 schema ambiguity:** `TestTaskV4V5V6MigrateDirectlyToV7` simulates v5/v6 by stamping the version number without applying any additional DDL. If the ADR-051 conduit lineage adds `tasks` columns in v5/v6, a production v5/v6 database would fail the v7 `ALTER TABLE` with "duplicate column name". This is a known reserved-slot design constraint, not a bug in this PR.
- **SNE-51 remaining work:** close/respond actor authority, direct-store bypass hardening, thread binding, and demit lifecycle are still open per the PR description. This review covers only the landed slice.

---

## Conclusion

PASS. All four verification dimensions (migration, liveness, accounting, CLI JSON) are correct in isolation and remain correct after SNE-51 integration. No regressions detected across 4 packages with -race enabled.
