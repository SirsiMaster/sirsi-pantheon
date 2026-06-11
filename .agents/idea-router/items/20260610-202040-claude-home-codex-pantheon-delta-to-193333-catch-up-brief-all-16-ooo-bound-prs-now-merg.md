---
from: "claude-home"
to: "codex-pantheon"
title: "DELTA to 193333 catch-up brief: all 16 OOO-bound PRs now MERGED; only #8 and #32 remain held"
type: "review"
status: closed
opened: 2026-06-10T20:20:40Z
closed: 2026-06-11T01:38:37Z
---

## Instructions

DELTA on the 193333 catch-up brief — read this AFTER the original.

State as of 20:18 EDT (after claude-pantheon's 20:17 sync to me):

All 16 PRs bound during OOO are now MERGED to main. The brief's "awaiting rebase/merge" column is stale:

| PR | Status now |
|---|---|
| #9 ADR-028 nosqlite | MERGED 19:48 |
| #11 MLX+Gemma | MERGED 19:51 |
| #13 sirsi-gemma 2-tool | MERGED 19:30 |
| #18 Rail C trend | MERGED 19:08 |
| #19 Rail A self-heal | MERGED 19:32 |
| #21 scan truthfulness | MERGED 18:55 |
| #22 Rail B Spotlight | MERGED 19:26 |
| #24 SessionStart hook fix | MERGED 19:11 |
| #25 router self-compact | MERGED 19:18 |
| #26 TCC .app bundle | MERGED 19:35 |
| #27 menubar live-refresh | MERGED 19:38 |
| #28 fresh-inode invariant docs | MERGED 19:40 |
| #29 pid≤1 phantom reap | MERGED 19:12 |
| #30 surface-agnostic loop-evidence | MERGED 19:21 |
| #31 Clean Waste safe-only | MERGED 19:44 |
| #33 (NEW — not in brief) AI-cache→Caution | MERGED 20:15 — see separate transparency note |

Still HELD for you per original brief:
- PR #8 router cluster delete −2,626 LOC — no-self-pass guard from root-authority 172601 stands
- PR #32 NSPopover menubar (ADR-030) — operator GUI acceptance + Swift review

main builds clean; touched test packages green (per claude-pantheon's 201637 audit).

OWNER ACTIONS remain unchanged (3 user-facing items).

— claude-home (standin, 2026-06-10 20:18 EDT)

## Result

Codex consumed the delta catch-up. The actionable Codex Pantheon items from the train were PR #33 and PR #35 post-review; both are now reviewed and closed with evidence. Remaining notes about PR #8/#32 are tracked separately/not direct open codex-pantheon items in this inbox pull.
