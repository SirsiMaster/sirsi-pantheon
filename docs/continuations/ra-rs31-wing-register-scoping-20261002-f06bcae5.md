<!-- agent: ra | workstream: rs-31-wing-schema-enforcement | repo: sirsi-pantheon | date: 2026-10-02 | session: thr-f06bcae5b1c0a1e0 -->

# rs-31a scoping notes — wing register verb

Task ledger row: `ra/rs-31-wing-schema-enforcement` (lease held, in-progress,
not released — no retry attempt consumed by this session).

## What already exists (reuse, don't rebuild — Rule 0)

- `internal/stacklab/wing.go` — `WingRecord` struct, `ValidateWing(raw []byte)`
  (hand-rolled, mirrors `contracts/stacklab/v2/wing.schema.json` field-for-field,
  deliberately no JSON-schema library per in-file comment), `ContentSHA256(raw)`.
  This covers schema validation + content digest already — the register verb's
  validation step is close to a direct call into this, not new logic.
- `cmd/sirsi/stacklabcmd.go` — CLI skeleton pattern (`wing-doctor`, `catalog`)
  to model the new `wing register` subcommand on. No `register` verb exists
  today; `grep -rn "wing" cmd/sirsi/` confirms this is net-new.
- Caller-identity resolution (the "don't trust self-declared fields" piece):
  `cmd/sirsi/threadcmd.go:719` `resolveCurrentAgent` + `internal/router/threads.go:513`
  `RegisterThread`, validated via `dispatch.New(...).ValidateAgent` /
  `router.DeclaredOnOrigin`. This is the existing authority-binding mechanism
  rs-31b should bind the caller's project/repo identity through — not a new
  JSON field on the wing record.
- Canonical-path containment precedent (no shared helper, 5 independent copies
  of `filepath.Abs` + `filepath.EvalSymlinks` + prefix-check): `internal/router/router.go:401-410`,
  `internal/routerstore/store.go:916-920`, `internal/routerstore/spoolcheck.go:95`,
  `internal/cleaner/safety.go:86-88`, `internal/router/relayagent.go:87`. rs-31c
  should probably extract one shared `IsWithin`-style helper instead of writing
  a 6th copy.

## Net-new work (the actual rs-31a build)

- No `wings` table/store method exists anywhere (`internal/routerstore`). The
  v21 migration (`schema.sql:272-280`) added `project_id`/`router_namespace`
  columns to `items`/`tasks` but explicitly defers population to this task —
  nothing persists a wing *record* itself yet (no digest column, no idempotency
  key). This is new schema (SQLite + Postgres, matching the existing dual-driver
  pattern from rs-07) + new store methods.
- Idempotency-on-identical-bytes and conflicting-identity rejection: no
  precedent in the codebase. Needs explicit design (e.g. unique index on
  wing id + content hash column; reject when id matches but content hash
  differs, unless an authorized version-bump path — not built yet either).
- Atomic persist-with-receipt (wing id/project_id/router_namespace/digest
  returned to caller) is new service-layer code wiring the above together.

## Found in passing: hash discrepancy (independently verified, not guessed)

`docs/router-service/stacklab/WING.md:18` and `contracts/stacklab/v2/PROVENANCE.md:11`
both cite `a69e0094b8ec...` as the pinned schema hash (from the original vendor,
PR #747). The file actually in the repo today hashes to `e2120a81160d...`
(`shasum -a 256 contracts/stacklab/v2/wing.schema.json`, confirmed locally),
which matches what's actually enforced in code —
`internal/routerstore/wingschema_test.go:17` `pinnedWingSchemaHash` — and that
test passes against the live file.

`git log --oneline -- contracts/stacklab/v2/wing.schema.json` shows PR #900
("fix(stacklab): accept wing naming canon") edited the vendored schema file
*after* the original vendor commit (#747), and the test constant was updated
to match in the same PR — but the two docs above were not. Separately, WING.md
states the schema's authority lives in `sirsi-inference` and is "never forked
here" — if that's still the intended model, PR #900 editing the local vendored
copy directly (rather than syncing a new hash from the SNE repo) is itself
worth a question to the SNE/`sirsi-inference` custodian before rs-31a pins
anything new against `e2120a81...`. I did not edit the docs or re-pin anything
myself — this needs an answer from whoever owns the SNE schema authority, not
a guess from this session.

## Why this wasn't built end-to-end this session

rs-31a alone (schema-validate + atomically persist + idempotency/conflict,
deferring rs-31b authority-binding and rs-31c adversarial path-containment
tests per the task's own sub-build split) is a real multi-file feature: new
dual-driver store schema, new store methods, new CLI verb, and a test matrix
for idempotency/conflict that has no precedent to copy. Shipping a rushed
version of a security-admission path (SNE design review + Ma'at test-matrix
gate named explicitly in the task) risked exactly the "half-finished
implementation" ponytail forbids. Scoped properly, next session can go
straight to implementation using the reuse list above instead of re-deriving
it.
