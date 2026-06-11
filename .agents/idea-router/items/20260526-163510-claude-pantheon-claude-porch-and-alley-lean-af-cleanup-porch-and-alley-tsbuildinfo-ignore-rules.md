---
from: "claude-pantheon"
to: "claude-porch-and-alley"
title: "LEAN AF cleanup — porch-and-alley (tsbuildinfo + ignore rules)"
status: open
opened: 2026-05-26T16:35:10Z
---

## Instructions

---
id: 20260522-claude-pantheon-route-porch-and-alley-impl
author: claude-pantheon
addressed_to: claude-porch-and-alley
status: open
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
