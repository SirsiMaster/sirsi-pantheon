<!--
agent:      claude-home
workstream: fabric truth — surfaces that report states they never measured
repo:       sirsi-pantheon (+ Assiduous, FinalWishes, sirsi-inference as binding agent)
date:       2026-08-07
session:    d8b52186-bc0c-4db2-b5d5-664de54b5ecc
-->

# Continuation — claude-home, 2026-08-07 evening

## The through-line

Every defect found tonight is one class: **a surface reporting a state it never
measured.** Say this out loud when triaging — it predicted five of them.

  1. `wake-install` said "installed" for a plist launchd never loaded  -> PR #659
  2. `StoreWake()` read ANY stat error as "not cut over" -> legacy 2nd registry -> #654 MERGED
  3. `sirsi-bind.sh` read ANY gh api failure as "App not installed" -> PR #658
  4. `ErrNoWork` said "no open ITEM" from the TASK path -> #652 MERGED
  5. gemma-pantheon's `--version` health check stays green after the worker dies
  6. MY OWN spawn probe: `pgrep -f 'claude --print'` cannot match `codex exec`

## Open PRs (all need an independent bind — I authored them, no self-review)

  #639  spawn ceiling for #636. Routed to claude-pantheon (item 20260807-222331)
        deliberately: codex-home is the lane LEAKING, so it must not review its
        own throttle.
  #649  claude-home resident consumer + health_check = `sirsi thread attended`
        (NOT `--version` — that is defect 5 above)
  #651  breaker operator verbs. codex-home approved 0576f7d8; head moved to
        ec508f17 (11 lines .gitignore). Delta ack requested, never answered.
  #658  bind script: 404 -> not-installed, everything else -> retryable
  #659  wake-install loads + verifies via launchd
  #662  dirty-build migration gate scoped to the SHARED store only

## Measured, unresolved

  codex-home: 53 dispatches today, 0 errors, 3 items undrained, 0 live consumers.
  Dispatch every 60s with no progress requirement. That IS #636.
  Fleet-wide 13 dispatches/10min — 11 of them codex-home, so the 5->23 arming did
  NOT cause a surge. One lane accounts for it.

  `lifecycle-fence-lost`: STILL LIVE after #619. 109 occurrences in
  wake-claude-finalwishes.log, most recent AFTER the install carrying the fix.
  #619 bounded the retry; it did not fix the race.

## Fleet state

  23/26 lanes have a LOADED watcher (was 5). Three deliberately not:
    claude-home     — this session is its watcher
    gemma-pantheon  — resident external worker, a lane would duplicate it
    horus           — plist is `.OFF-owner-20260807`. OWNER TURNED IT OFF.
                      DO NOT restore. A control that reverses an owner
                      decision is the defect.

## Traps that cost real time tonight — do not re-learn these

  - `cp` over the live ~/.local/bin/sirsi makes every later exec SIGKILL (137),
    silently, and `codesign -v` still says VALID. `rm` first. Rolling back with
    another `cp` reproduces it.
  - Exit 137 reads as "the command did nothing". Suppressing output in a loop
    turns it into a silent no-op: I reported "closed 21" when nothing closed.
    ALWAYS verify the store, never the exit code.
  - `sirsi-bind.sh` defaults to `--repo SirsiMaster/sirsi-pantheon`. PR numbers
    collide across repos, so an omitted flag mints a real bind on an unrelated
    PR and prints success. I did it twice. Ledger: bind-script-silent-wrong-repo.
  - DO NOT do uncommitted work in /private/tmp/claude-501/wt-esc — other live
    sessions manipulate that worktree. I lost ~40 lines to a stash/pop collision.
    Commit immediately or use a private clone.

## Owner posture

  - SNE is claude-nexus's IN WHOLE. Do not raise SNE to the owner. Do not bind
    for it without nexus ruling on ownership first.
  - "Every lane gets a watcher" — owner ruling, 2026-08-07. That is what drove
    the 5->23 arming.
  - Owner board is at 2: the codex-deck decisions I re-raised after a bulk
    transfer SWEPT five owner cards at 20:34Z. claude-pantheon fixed the cause
    (Facade.CloseItem had no owner-recipient guard, commit a043433f).
    I had reported that sweep as "lanes healthily draining" — it was not.

## Next

  1. Chase binds on the six PRs; #639 first (it bounds the leak).
  2. `lifecycle-fence-lost` needs a re-diagnosis at setThreadConsumerCapable,
     not more retries.
  3. 35 pending ledger rows. Reconcile against verified reality — several are
     terminal-in-fact. Do NOT close on an idle-host pass
     (`nonhermetic-tests-fail-on-loaded-host` is load-dependent; it passes idle).
