<!--
agent: ra
workstream: router-service (ADR-062) — identity architecture + #761 race + gate parity
repo: sirsi-pantheon
date: 2026-09-16
session: 6509a1af-6205-4a70-b158-8c3a4cb16b23
-->

# Ra continuation — 2026-09-16 ~06:10Z

Owner directive in force: **"fix these issues once and for all; don't report done until verifiably
fixed; if claude-home doesn't think they are clean, continue until fixed."** Not done yet.

## Merged tonight (verified via `gh pr view --json mergeCommit`)
#766 `e7448c2f` CI TMPDIR · #761 `122bb02b` stall gate/spool/node-status · #763 `e47f4ee3` cwd ·
#765 `b014437b` machineid opt-in · #764 `4f913766` doctor split-brain · #768 `b178d557` race fix +
`-race` in Makefile + pre-push. rs-40 marked done on ledger. claude-home 052742 (rs-44 wait) closed.

## The defect chain (record, do not re-derive)
#761 shipped a data race (`terminateConsumer` goroutine read pkg var `consumerKillGrace`; test
mutates+defer-restores). Missed because: local `go test` had no `-race`; Makefile/pre-push had no
`-race`; pre-push hook was **disarmed** (`core.hooksPath` unset). Fixed: #768 + `git config
core.hooksPath .githooks` on M1 (verified: hook fired and BLOCKED a push).

## In flight at hand-off
- **Ground truth** `env -i ... TMPDIR=~/.sirsi/tmp/race-main/ go test -race -short ./...` on `main`
  (bg bn73v6po7). **Negative control** same pkgs WITH ambient `SIRSI_ROUTER_URL`/`SIRSI_RELAY_TRUST_GROUP`
  (bg baj1sjf53) — hypothesis: that env is why the armed hook blocked the docs push. Do NOT use
  `~/actions-runner-pantheon/_work/_temp/` for local TMPDIR — the runner wipes it (bit me twice).
- **Docs PR #767** branch `docs/router-identity-architecture-and-a35-entry` (worktree
  `~/.sirsi/worktrees/docs-identity-architecture`): local merge commit `aa07127` (main post-#768)
  NOT pushed — hook blocked. Push with `env -u SIRSI_ROUTER_URL -u SIRSI_RELAY_TRUST_GROUP git push`
  (never `--no-verify`), then watch CI → must be green (its Test was failing only on the #761 race).
- **Control CONFIRMED (06:1xZ)**: same `main` head — clean env: 0 failures; with the shell's
  `SIRSI_ROUTER_URL`/`SIRSI_RELAY_TRUST_GROUP`: dozens of router/routerstore FAILs. Hook block was
  env, not code. Fix authored + pushed on branch `fix/prepush-scrub-sirsi-env` (hook runs
  `env $SCRUB_SIRSI go test ...`); **PR #769 MERGED `4d7abc30`**. This commit's push is the
  proof: made from the exporting shell with NO manual `env -u` — hook must pass on its own.
- **Then** send `scratchpad/claude-home-verify-request.md` (drafted, 6 claims + residuals) to
  claude-home via `SIRSI_AGENT_ID=ra sirsi router send --to claude-home ... @file`; act on any
  NOT CLEAN until CLEAN. Only then report done.

## Residuals (open by design)
rs-42/43 authenticated hostname→machine-id migration (shape bridge REJECTED, `TestThreadAuthorityIsHostScoped`);
rs-44 scoped read-only token (claude-home cloud Routine waits on it); rs-41 anchor ADR not started;
owner cards: pin M1 HostName, SIP/FileVault, M5 key. Open ra inbox: 8 items (SHA rails/ADR-065
verdict, claude-io reconciliation/delegation) — read via `sirsi router pull ra`.

Refs: `docs/router-service/IDENTITY_ARCHITECTURE.md`; PANTHEON_RULES A35 rows (2026-09-16);
memory `reference_local_go_test_must_match_ci_flags_race_tmpdir.md`.
