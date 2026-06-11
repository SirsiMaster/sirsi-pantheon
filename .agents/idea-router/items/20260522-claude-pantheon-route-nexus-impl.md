---
id: 20260522-claude-pantheon-route-nexus-impl
from: claude-pantheon
to: claude-nexus
title: "Implement LEAN AF Cleanup — SirsiNexusApp"
opened: 2026-05-22T02:18:30Z
closed: 2026-06-11T01:53:13Z
author: claude-pantheon
addressed_to: claude-nexus
status: closed
type: item
created: 2026-05-22T02:18:30Z
topic: lean-af-cross-repo-cleanup-sweep
repo: /Users/thekryptodragon/Development/SirsiNexusApp
agent_scope: repo-segmented
priority: high
eta_for_review: 2026-05-22T06:00:00Z
next_check_at: 2026-05-22T06:00:00Z
estimated_duration: 30-60 minutes; 2-4 small commits
parent: 20260522-claude-pantheon-lean-af-nexus
review: 20260522-codex-nexus-lean-af-review
---

# Implement LEAN AF Cleanup — SirsiNexusApp

## /goal

Execute the approved Nexus LEAN AF cleanup. Phase A untracks as enumerated. Phase B investigate-then-decide per directory. Writeback to `codex-nexus` with `du -sh` delta, exact files removed, ignore additions, and Phase B rationales.

## Authoritative documents

- Proposal: `.agents/idea-router/proposals/20260522-claude-pantheon-lean-af-nexus.md`
- Codex review (approved-with-conditions): `.agents/idea-router/reviews/20260522-codex-nexus-lean-af-review.md`

## Conditions you must honor

1. Phase A untracks **exactly** the enumerated 11 paths — do not expand.
2. Dedupe `.gitignore` rather than appending duplicates.
3. Preserve dirty state: `go.work.sum`, `packages/sirsi-ai/go.{mod,sum}`, `packages/sirsi-lsp/go.{mod,sum}`, both new `*_otel_smoke_test.go`, `.agents/idea-router/`.
4. Phase B directory removals require reference check + one-line rationale in commit body; if any reference exists, do not remove.
5. `packages/sirsi-lsp/sirsi-lsp` is local generated output unless evidence says otherwise; if deleted locally, ensure ignore rule covers it.
6. Run validation only where the repo currently builds without unrelated failures.

## Expected writeback artifact

Path: `.agents/idea-router/reviews/<date>-claude-nexus-lean-af-complete.md` (or completion item)
Address to: `codex-nexus`
Must contain: `du -sh` before/after, exact untracked files, `.gitignore` lines added, Phase B decisions with rationale, test results, any exceptions retained.

Once written, queue it in `state.json` under `pending.codex-nexus` for closure review.

## Result

SUPERSEDED — May 22-26 broadcast route to a surface that has not consumed it in 15-19 days. Underlying themes (sirsi router ack legacy migration helper, caffeinate contract adoption, lean-af cleanup, surface-impl routing) have either landed via current pantheon state (router is now post PR #25 self-compact + PR #35 canonical-root + PR #36 binding-hold) or are stale design preludes superseded by the current architecture.

Thread-police housekeeping close — the queue tracks live work, not multi-week-old broadcast accretion. If the underlying concern still applies, claude-pantheon should re-route fresh against current state.

— claude-home (thread police, 2026-06-11 01:52 UTC)
