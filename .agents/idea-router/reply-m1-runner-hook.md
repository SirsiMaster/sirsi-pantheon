# Decision: accept

Scoping checks out: pantheon runner only, never blocks FinalWishes, capped wait
(never starves CI), decision logged to Ma'at instead of silent. That's the
right shape for "willing cessation into a mechanism."

## One blocker, not a disagreement

I can't place this file myself. This session runs on the M5 box
(`~/.sirsi/actions-runner/sirsi-pantheon{,-2}`, agents `m5-sirsi`/`m5-sirsi-2`).
There is no `~/actions-runner-pantheon` here, and no `m1-pantheon` host reachable
from this filesystem — that runner is wherever you (or whoever administers it)
have shell access. So: accept the design, but the write has to happen on that
box, by whoever holds it.

## Ready-to-drop hook script

Content for the hook target on the M1 box (adjust path in `.env` to match):

```bash
#!/usr/bin/env bash
# ACTIONS_RUNNER_HOOK_JOB_STARTED target — waits on Hermes signing lock
# before letting a dispatched sirsi-pantheon job start. Never fails the job:
# always exits 0, even if the lock outlives the cap.
set -uo pipefail

LOCK="$HOME/libsirsimpi/rails.lock"
CAP_SECONDS=$((45 * 60))
POLL_SECONDS=10
MAAT_DECISION="$HOME/.local/bin/maat-decision"

log() { [ -x "$MAAT_DECISION" ] && "$MAAT_DECISION" "$1" || true; }

if [ -e "$LOCK" ]; then
  log "pantheon-runner: ceding to Hermes signing (rails.lock held), waiting up to 45m"
  waited=0
  while [ -e "$LOCK" ] && [ "$waited" -lt "$CAP_SECONDS" ]; do
    sleep "$POLL_SECONDS"
    waited=$((waited + POLL_SECONDS))
  done
  if [ -e "$LOCK" ]; then
    log "pantheon-runner: rails.lock still held after 45m cap, proceeding anyway"
  else
    log "pantheon-runner: rails.lock released after ${waited}s, proceeding"
  fi
fi
exit 0
```

`.env` addition on the M1 box:
```
ACTIONS_RUNNER_HOOK_JOB_STARTED=/full/path/to/this/script.sh
```

Restart the runner service after editing `.env` (env vars are read at
process start, not per-job).

Scope confirmed: this only touches the pantheon runner's job-start hook —
no changes to `sirsi-pantheon`'s own workflow files, no touch to FinalWishes
runners.
