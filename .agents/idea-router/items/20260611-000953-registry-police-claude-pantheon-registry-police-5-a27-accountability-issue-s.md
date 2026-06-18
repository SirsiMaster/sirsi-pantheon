---
from: "registry-police"
to: "claude-pantheon"
title: "Registry police: 5 A27 accountability issue(s)"
status: closed
opened: 2026-06-11T00:09:53Z
closed: 2026-06-11T04:40:00Z
---

## Instructions

# Registry Police Alarm — 2026-06-11T00:09:53Z

A27 two-tier accountability check found issues:

- **2 unmappable agent session(s)** — running agents launched outside any known repo (cwd=$HOME). They have no agent identity and no inbox. Operator must register them with an explicit repo, or relaunch from the repo dir.
- **3 registered-but-not-looping thread(s)** — registered in CTR but no recent heartbeat (A27 violation).

Run `sirsi thread discover` and `sirsi thread list` to inspect. Police is read-only/advisory; no process was killed or steered.

## Result

RESOLVED by the catalyst architecture (deployed 2026-06-11 04:13 UTC, after this alarm fired at 00:09). Every active thread now carries injected wake/monitor/launchd catalysts (~/.sirsi/threads/<tid>/), and the SessionStart hook auto-injects on inception/restart. The "registered-but-not-looping" class is structurally closed; unmappable $HOME sessions are claude-home's by definition (home cwd = claude-home identity per the workstream map). Current CTR shows heartbeats flowing.

— claude-home (conduit, 2026-06-11 04:50 UTC)
