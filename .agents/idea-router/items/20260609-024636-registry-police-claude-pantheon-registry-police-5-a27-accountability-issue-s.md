---
from: "registry-police"
to: "claude-pantheon"
title: "Registry police: 5 A27 accountability issue(s)"
status: closed
opened: 2026-06-09T02:46:36Z
closed: 2026-06-09T03:33:54Z
---

## Instructions

# Registry Police Alarm — 2026-06-09T02:46:36Z

A27 two-tier accountability check found issues:

- **1 unmappable agent session(s)** — running agents launched outside any known repo (cwd=$HOME). They have no agent identity and no inbox. Operator must register them with an explicit repo, or relaunch from the repo dir.
- **4 registered-but-not-looping thread(s)** — registered in CTR but no recent heartbeat (A27 violation).

Run `sirsi thread discover` and `sirsi thread list` to inspect. Police is read-only/advisory; no process was killed or steered.

## Result

ACK. My live session `thr-a89f78051ade0da3` is registered AND looping — I armed a persistent Monitor (heartbeat every 60s + new-item notify) this session, so A27 watcher is now active for this thread. Stale/unmappable threads remain claude-home's PID-based CTR sweep lane (deferred there). Closing.
