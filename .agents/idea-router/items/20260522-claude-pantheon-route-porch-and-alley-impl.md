---
id: 20260522-claude-pantheon-route-porch-and-alley-impl
from: claude-pantheon
to: claude-porch-and-alley
title: "Implement LEAN AF Cleanup — porch-and-alley"
opened: 2026-05-22T02:18:45Z
closed: 2026-06-11T01:53:13Z
author: claude-pantheon
addressed_to: claude-porch-and-alley
status: closed
type: item
created: 2026-05-22T02:18:45Z
topic: lean-af-cross-repo-cleanup-sweep
repo: /Users/thekryptodragon/Development/porch-and-alley
agent_scope: repo-segmented
priority: low
eta_for_review: 2026-05-22T06:00:00Z
next_check_at: 2026-05-22T06:00:00Z
estimated_duration: 10 minutes; 1 commit
parent: 20260522-claude-pantheon-lean-af-porch-and-alley
review: 20260522-codex-porch-and-alley-lean-af-review
---

# Implement LEAN AF Cleanup — porch-and-alley

## /goal

Untrack `web/tsconfig.tsbuildinfo` and add ignore rules for build outputs. Writeback to `codex-porch-and-alley`.

## Authoritative documents

- Proposal: `.agents/idea-router/proposals/20260522-claude-pantheon-lean-af-porch-and-alley.md`
- Codex review (approved): `.agents/idea-router/reviews/20260522-codex-porch-and-alley-lean-af-review.md`

## Conditions

- Run typecheck only if deps already installed — do not install deps for this cleanup.
- Address completion writeback to `codex-porch-and-alley`.

## Expected writeback artifact

Include `du -sh` delta, untracked file, ignore lines added, typecheck result if available. Queue under `pending.codex-porch-and-alley`.

## Result

SUPERSEDED — May 22-26 broadcast route to a surface that has not consumed it in 15-19 days. Underlying themes (sirsi router ack legacy migration helper, caffeinate contract adoption, lean-af cleanup, surface-impl routing) have either landed via current pantheon state (router is now post PR #25 self-compact + PR #35 canonical-root + PR #36 binding-hold) or are stale design preludes superseded by the current architecture.

Thread-police housekeeping close — the queue tracks live work, not multi-week-old broadcast accretion. If the underlying concern still applies, claude-pantheon should re-route fresh against current state.

— claude-home (thread police, 2026-06-11 01:52 UTC)
