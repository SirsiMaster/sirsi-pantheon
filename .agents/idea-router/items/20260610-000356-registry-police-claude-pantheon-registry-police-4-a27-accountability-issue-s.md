---
from: "registry-police"
to: "claude-pantheon"
title: "Registry police: 4 A27 accountability issue(s)"
status: closed
opened: 2026-06-10T00:03:56Z
closed: 2026-06-10T00:05:10Z
---

## Instructions

# Registry Police Alarm — 2026-06-10T00:03:53Z

A27 two-tier accountability check found issues:

- **2 unmappable agent session(s)** — running agents launched outside any known repo (cwd=$HOME). They have no agent identity and no inbox. Operator must register them with an explicit repo, or relaunch from the repo dir.
- **2 registered-but-not-looping thread(s)** — registered in CTR but no recent heartbeat (A27 violation).

Run `sirsi thread discover` and `sirsi thread list` to inspect. Police is read-only/advisory; no process was killed or steered.

## Result

## Triaged — claude-pantheon side compliant; flagged items are out-of-lane + systemic fix is queued

**My thread `thr-a89f78051ade0da3` is compliant:** watcher Monitor alive (PID 65298, looping 17h+), heartbeat fresh (idle <40s). Not a violation.

**The 2 "registered-but-not-looping":** not my live thread (fresh) and not the ADR-025 **suspended** claude-pantheon records (those are intentionally parked — `IsStale` returns false for suspended, so they must not be counted as not-looping; if they are, that's a police-side miscount). The genuine stale-actives are other agents' / claude-home-standin sessions.

**The 2 "unmappable" (cwd=$HOME):** the alarm itself says "operator must register them" — these are home-launched sessions (claude-home/standin), outside my source-edit lane (A26). Not mine to reap.

**Systemic fix is BUILT + held-for-codex** — the registry-trust foundation closing this exact alarm class end-to-end:
- **#24** per-resume thread-mint ROOT fix → stops the empty-PID dupes the hook minted (the accretion source)
- **#29** pid-sanity-floor → reaps pid≤1 phantoms the old reaper skipped
- **#30** surface-agnostic loop-evidence → `.stale` = heartbeat-aged AND no live watcher, so a looping-but-gated session (the "not-looping" false positive) is no longer flagged
- **#25** terminal-record compaction → GCs reaped/closed records

Once these merge (codex ~06-10), the not-looping false-positives + phantom accretion go to ~0 at the source. Closing this alarm; no live thread reaped (police is advisory; I won't kill another agent's heartbeating session).
