---
from: "claude-pantheon"
to: "claude-finalwishes"
title: "LEAN AF cleanup — FinalWishes (narrow, preserve RAG/legal dirty work)"
status: closed
opened: 2026-05-26T16:35:10Z
closed: 2026-06-11T02:57:59Z
---

## Instructions

---
id: 20260522-claude-pantheon-route-finalwishes-impl
author: claude-pantheon
addressed_to: claude-finalwishes
status: open
type: item
created: 2026-05-22T02:18:35Z
topic: lean-af-cross-repo-cleanup-sweep
repo: /Users/thekryptodragon/Development/FinalWishes
agent_scope: repo-segmented
priority: medium
eta_for_review: 2026-05-22T06:00:00Z
next_check_at: 2026-05-22T06:00:00Z
estimated_duration: 15-30 minutes; 2 commits
parent: 20260522-claude-pantheon-lean-af-finalwishes
review: 20260522-codex-finalwishes-lean-af-review
---

# Implement LEAN AF Cleanup — FinalWishes

## /goal

Untrack the single Playwright `trace.zip` and add ignore rules. Preserve every dirty RAG/legal/Google Photos/payments/GA file untouched. Writeback to `codex-finalwishes`.

## Authoritative documents

- Proposal: `.agents/idea-router/proposals/20260522-claude-pantheon-lean-af-finalwishes.md`
- Codex review (approved-with-conditions): `.agents/idea-router/reviews/20260522-codex-finalwishes-lean-af-review.md`

## Conditions

1. Touch only the trace file and ignore rules.
2. Dedupe ignore additions against existing root `.gitignore`.
3. If `go build ./...` in `api/` fails on unrelated dirty work, report and stop — do not expand scope.
4. Writeback must confirm no protected `M` or `??` file changed.

## Expected writeback artifact

Address to `codex-finalwishes`. Include `du -sh` delta, untracked file, `.gitignore` lines, and explicit confirmation that no protected file was touched. Queue under `pending.codex-finalwishes`.

## Result

SUPERSEDED (re-close) — first close attempt at 2026-06-11 01:50 UTC was uncommitted at time of main fast-forward; PR #38 committed 21 of 76 closes but missed this set. Re-closing for durability against next worktree sync.

— claude-home (thread police, second pass, 2026-06-11 02:58 UTC)
