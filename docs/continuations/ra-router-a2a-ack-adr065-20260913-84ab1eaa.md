<!-- agent: ra | workstream: router-service (ADR-062) | repo: sirsi-pantheon | date: 2026-09-13 | session: 84ab1eaa-67f4-4eef-9f48-5d35d1d6ba2a -->

# Ra — continuation: PR #754 rollout complete, rs-38 regression fixed (2026-09-13)

Resume from THIS file only if you are `ra` on this workstream. Identity reuses thread
`thr-df2a8cd5b1c61290` (`--thread`, never mint a new id). The thread record is reaped
("lost lifecycle fence" on `thread register`); gate mode is `log`, so ledger mutations still land.

## State at hand-off (verified unless marked)

- **main `1995046`** = PR #755 (rs-38 fix) over PR #754 `3d535ebc` (body-loss guard, `router acknowledge`,
  schema v22, ADR-065 Proposed). Both merged on an owner-waived bind ("move forward").
- **Rollout (recipe §1):** C7 Cloud SQL v22 · C4/C5 Cloud Run `sirsi-router` `00013-nhn`, min-instances 1 ·
  C3a M1 client `341b7ae0…` (#754 build; does NOT carry #755 — the M1 spool is single-owner so the
  `mkdirTrusted` bug never fired there; roll it at the next M1 change, not urgently) · C3b M5 client
  `0b2fd3da…` (#755 build), 9 running lanes + system relay live-verified 08:41–42Z.
- **rs-38 regression, fixed:** `mkdirTrusted` chmod'd pre-existing dirs; on the M5 `~/.sirsi/relay` →
  `/var/sirsipantheon/relay` with `Mac/` owned by `_sirsipantheon` → EPERM → all 9 lanes crash-looped.
  Rolled back, fixed (stat first, chmod only what MkdirAll created), negative control run, re-rolled.
- **Ledger (ra):** rs-34, rs-35, rs-38 **done** (fenced) · rs-36 pending (SSA sandbox has no network; bundle
  delivery is manual — closes with ADR-065 Decision 4) · rs-37 blocked (ADR-065 needs SHA + SSA design
  verdict; NO informer code exists).
- **Router:** SHA notified of the rollout/regression/fix in `20260913-083606-ra-sirsi-hardware-admin-…`.
  SSA is at `397eb638` with no network; bundles live in `~/.sirsi/handoff/` — cut a fresh one from
  `397eb638..1995046` for anything they must read.

## On resume — in order

1. `sirsi router pull ra` — SHA/SSA replies on #755, ADR-065 verdicts.
2. If SHA + SSA accept ADR-065 → start rs-37 per the ADR's cut-over order (informer up → verified push
   receipt → retire ONE lane's plist, negative control first). Decision 4 closes rs-36.
3. Roll the M1 client to `1995046` at the next M1-touching change (recipe §4 step 4: build → `.new` →
   atomic `mv`; never `cp` over a live binary).
4. Out-of-scope flag, not started: `~/.sirsi/logs/router-relay.log` on the M5 (gui-side, not the daemon's
   log at `/var/sirsipantheon/router-relay.log`) has stale `spool: … is a symlink; refusing` lines from a
   non-daemon invocation; find what writes there before touching anything.

## M5 operating facts that cost time today

- 26 wake lanes are registered, **9 run**; the other 17 are parked by owner decision (2026-08-09). Kickstart
  only lanes already in `state = running`. `launchctl print gui/501/<label>` returns nonzero for a parked
  label — under `set -e` that kills a script at the first one.
- The relay is a **system** daemon: `sudo launchctl kickstart -k system/ai.sirsi.router.relay`; it runs
  `~/.local/bin/sirsi router relay serve --spool /var/sirsipantheon/relay` as `_sirsipantheon`, so every
  client roll must kickstart it too.
- `go` is not on the SSH PATH: `export PATH=/opt/homebrew/bin:$PATH`. Build from the clean worktree
  `~/.sirsi/worktrees/main-3d535ebc` (detach it at the target commit); the host checkout is dirty — never
  stash or reset it.
- `sirsi --version` is not a flag; smoke-test a fresh binary with `sirsi router status ra` over `spool://`.
- Ledger: `task claim-id <agent> <id> --thread … --worker … --json` (two positional args) → lease ~10 min →
  `task complete <agent> <id> --lease <id> --result-ref @file`. Inline router args are shell-evaluated.
