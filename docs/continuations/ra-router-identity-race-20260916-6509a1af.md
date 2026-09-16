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
- **Proof done 02:3xZ**: hook's exact line under bash: scrubbed → routerstore ok 114s, router ok
  28s; unscrubbed → FAIL. (Re-run under bash, NOT zsh — zsh won't split `$SCRUB_SIRSI`; a zsh
  re-run reproduced the ambient FAIL list and looked like the fix failed.) #767 green on `b06fdf6`.
- **SENT** verify request to claude-home (router item, type review, 6 claims + residuals).
  **WAITING on verdict.** Pull replies: `SIRSI_AGENT_ID=ra sirsi router pull ra`. Act on any
  NOT CLEAN until CLEAN. Only then report done to the owner.
- **02:37Z claude-home reported a spool gap** (`SIRSI_AGENT_ID=claude-home … chmod relay/claude-home
  EPERM`). Root cause: stale `~/.local/bin/sirsi` (09-13 04:09) predating PR #755 (04:39). Rebuilt
  from main `4d7abc3`, installed (rm+cp; old at ~/.sirsi/tmp/bin/sirsi-old-20260913), verified both
  identities, negative control on old binary. Replied on router (type decision). claude-home is
  running the 6-claim verification via the plain path meanwhile. Relay daemon untouched.

## Residuals (open by design)
rs-42/43 authenticated hostname→machine-id migration (shape bridge REJECTED, `TestThreadAuthorityIsHostScoped`);
rs-44 scoped read-only token (claude-home cloud Routine waits on it); rs-41 anchor ADR not started;
owner cards: pin M1 HostName, SIP/FileVault, M5 key. Open ra inbox: 8 items (SHA rails/ADR-065
verdict, claude-io reconciliation/delegation) — read via `sirsi router pull ra`.

Refs: `docs/router-service/IDENTITY_ARCHITECTURE.md`; PANTHEON_RULES A35 rows (2026-09-16);
memory `reference_local_go_test_must_match_ci_flags_race_tmpdir.md`.
