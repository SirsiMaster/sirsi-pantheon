<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread.
     agent: claude-pantheon | workstream: flaky-router-ack-gitdir | repo: sirsi-pantheon | date: 2026-06-29 | session: e008ddba-975f-4432-b28f-ee08c80b042b | path: docs/continuations/claude-pantheon-flaky-router-ack-gitdir-20260629-e008ddba.md -->

# claude-pantheon — Flaky TestRouterAckLegacyPending → GIT_* env isolation

**Owner:** `claude-pantheon` (work originated in a session the SessionStart hook mis-tagged
`claude-home` because cwd was `~`; per canon [I Am Claude Pantheon], pantheon work is
claude-pantheon's).

## Status: DONE pending merge

- **PR #99** — https://github.com/SirsiMaster/sirsi-pantheon/pull/99
- Branch: `fix/flaky-router-ack-gitdir-isolation` (off `main`), pushed; **Ma'at pre-push gate PASSED** on push.
- Scope: **test-only**, `cmd/sirsi/integration_test.go` (+40/-2).

## What was wrong

`TestRouterAckLegacyPending` was green under a bare `go test` but failed **deterministically**
under the Ma'at pre-push gate with `integration_test.go:233: unexpected claude pending:
[]interface {}{"item-a", "item-b"}` (state.json wholly unchanged though `ack` printed "Acked 2").

**Root cause:** the gate runs inside `git push`, which exports `GIT_DIR` (+ `GIT_INDEX_FILE`,
`GIT_PREFIX`, `GIT_COMMON_DIR`, …) into the env. The test helpers passed `cmd.Env = os.Environ()`,
leaking those into every `sirsi` subprocess. Post-ADR-029, `router.FindRepoRoot()` resolves the
router root by shelling `git rev-parse --git-common-dir`, which **honors `GIT_DIR` over the process
cwd**. So `router ack`, launched in the test's `t.TempDir()`, resolved the **real repo's** router
root, wrote `state.json` there, and left the temp copy untouched → assertion read the unchanged
temp state.

- No `GIT_*` (bare `go test`) → `git rev-parse` fails → `FindRepoRoot` falls back to cwd walk-up →
  temp root → passes. That asymmetry is why it looked "flaky."
- The git-based resolver lives on `main`/PR-#98-merge; the older `fix/sirsi-gemma-bare-server-chipA`
  branch still has the pre-ADR-029 cwd-walk-up `FindRepoRoot`, so the bug is invisible there.

## Fix

New `sirsiTestEnv(dir, extra...)` helper strips all `GIT_*` vars (and pins `PWD`) and is used by
both `runSirsiWithEnv` and `runSirsiInDir`. Closes the class for every router integration test
(`ack`, `pull`, ADR-024), not just the one that surfaced it.

## Verification (all green)

- Reproduced the exact `:233` failure by injecting `GIT_DIR` at a throwaway **decoy** repo; with the
  fix it passes and the decoy `state.json` is never mutated (100+ ack runs).
- `go test ./cmd/sirsi/ -run TestRouterAck -count=20` green **with `GIT_DIR` set**.
- Full `-short` package green with `GIT_DIR` set (ack + pull + all ADR-024 register tests).
- No regression without `GIT_DIR`. `gofmt`/`go vet`/`golangci-lint` clean.
- **Actual Ma'at pre-push gate passed** when pushing the branch.

## Next action for claude-pantheon

1. Get CI green on PR #99 and **merge** it (auto-merge may land it ~2 min after green — see
   [Auto-merge Overrides Hold]).
2. Optional: cherry-pick / forward-port the same `sirsiTestEnv` guard onto any active branch that
   still carries the older `integration_test.go` helper without GIT_* stripping (e.g.
   `fix/sirsi-gemma-bare-server-chipA`) so its router tests don't reintroduce the gate flake there.

## Reusable lesson

Any Pantheon test that runs a binary which shells out to `git` will be hijacked to the gate's repo
under the pre-push hook unless GIT_* is stripped. Recorded in auto-memory
`reference_git_env_leak_test_isolation.md`.
