<!--
agent: ra
workstream: router-service (ADR-062) — identity architecture + #761 race + gate parity
repo: sirsi-pantheon
date: 2026-09-16
session: 6509a1af-6205-4a70-b158-8c3a4cb16b23
-->

# Ra continuation — 2026-09-16 ~06:10Z

Owner directive in force: **"fix these issues once and for all; don't report done until verifiably
fixed; if claude-home doesn't think they are clean, continue until fixed."**
**DONE 06:45Z — claude-home verdict: ALL SIX CLEAN at main 4d7abc30** (item 20260916-064531,
read-only live check, own bash negative control). Reported to owner. Remaining work is the
residuals below, none of it part of tonight's directive.

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
- **02:55Z M5 lane CLI rebuilt too (owner asked)**: `thekryptodragon@m5:~/.local/bin/sirsi`
  09-13 04:39 → 4d7abc3 via scp of the M1 build (no Go on M5). 10 wake loops + horus keep the old
  inode until they respawn on their own (PIDs verified unchanged; nothing restarted). Negative
  control: old M5 binary already had #755 — M5 was never exposed to the EPERM class.
- **~03:1xZ owner at M5**: relay daemon rolled to 4d7abc3 (sudo -n, rename not cp, bootout+bootstrap;
  verified). M5 SSH-key card closed. FileVault on M5 ON (verified). M5 cards all closed. M1: HostName already pinned, FileVault on. SIP: claude-io ruled B
  (decision 20260916-174733) — no longer needed for TB4 RDMA (silicon-gated; dext signed). Owner
  enables it at next reboot (Recovery → csrutil enable); no reboot today (mid-build).

## Inbox sweep 2026-09-16 ~18:0xZ (12 items → 0 open)
- SHA gate facts (8,184 B DF ceiling / 64 KiB TCP PASS) recorded → memory; closed. SHA node-profile
  update routed to claude-io (acting SHA to 09-19) item 175345 + merge io-connect PR #197.
- claude-io spool thread: ADR-062 cross-host audit done (both hosts service URL; no SIRSI_ROUTER_DB
  anywhere); closed. Delegation ack'd.
- ADR-065 hardware-seat ACCEPT (2 conditions) recorded in ADR + index (PR #770). SSA + bind pending.
- **ADR-066 Stack Lab wing authority + Rule A37: PR #770** (Proposed). GO sent to claude-home to build
  `sirsi stacklab doctor` under Ra bind. rs-45 = roster + 3 undeclared wings. Six drafts to ratify.
- SIP on M1: claude-io ruled B (not needed); owner enables at next reboot.
- **18:1xZ**: #770 green (after mirroring A37 into CLAUDE.md/GEMINI.md + index fix). **Bind requested from
  SSA** for #767 + #770, bundles in ~/.sirsi/handoff/sirsi-pantheon-*-20260916.bundle. rs-45 on ledger.
  claude-io unblocked (stale GH_TOKEN check + own worktree, item 175915). Awaiting: SSA bind, claude-home
  doctor PR, claude-io #197 merge + node-profile PR.

## Close of day 2026-09-16 (~20:4xZ) — all on main unless noted
- #767 `365ba8fd` + #770 `0016b726` MERGED on claude-home's bind (owner rerouted from SSA, out to 09-20;
  owner cleared claude-home's disclosed ADR-066 conflict). ADR-066 + Rule A37 + ADR-065 hardware verdict live.
- #771 `sirsi stacklab doctor` MERGED `e716392a` on Ra's bind after two blockers fixed (repo-404 → Unknown;
  branch-scan failure → no finding; roster ids validated). First live run: 3 stranded (§6 map unknown),
  1 undeclared = router wing's own pin (self-exclusion → claude-home follow-up), 1 unknown = catalog file
  misfiled under wings/pinned (→ claude-inference). io-connect canonical (#197 merged `1edd0cca`, hash == pin).
- CLIs on M1 + M5 rolled to `e716392a` (old copies kept). M5 relay daemon rolled to 4d7abc3 earlier; M5
  FileVault ON; M5 SSH-key card closed. M1: HostName pinned, FileVault on; SIP-off ruled unnecessary by
  claude-io (B) → owner runs `csrutil enable` at next reboot.
- Owner "clean up your CI": 3 red runs deleted; #772 (M5 claude-io lane) paths-ignore docs + docs-canon-check
  with stub jobs for the required check names — Ra bind pending green. THIS commit is the docs-only live
  proof: it must run only "Canon + ADR-INDEX guard" + the three stubs, not the heavy jobs.
- Two claude-io processes on one lane (M5 wake lane headless + live session): owner "belt and suspenders" —
  M5 gh logged in by owner; rule "deferred to attended session" sent to claude-home for rs-37.
- Ledger: rs-45 added (roster + nexus-experience, finalwishes, apple-accelerator-routes).
- Open on other lanes: claude-home doctor follow-up (§6 map, self-exclusion, --fix, guard, quiet CI wiring on
  the roster path); claude-inference registry fix; claude-io PR #369 (SirsiNexusApp node profile) for SHA 09-19.

## Residuals (open by design)
rs-42/43 authenticated hostname→machine-id migration (shape bridge REJECTED, `TestThreadAuthorityIsHostScoped`);
rs-44 scoped read-only token (claude-home cloud Routine waits on it); rs-41 anchor ADR not started;
owner cards: pin M1 HostName, SIP/FileVault, M5 key. Open ra inbox: 8 items (SHA rails/ADR-065
verdict, claude-io reconciliation/delegation) — read via `sirsi router pull ra`.

Refs: `docs/router-service/IDENTITY_ARCHITECTURE.md`; PANTHEON_RULES A35 rows (2026-09-16);
memory `reference_local_go_test_must_match_ci_flags_race_tmpdir.md`.
