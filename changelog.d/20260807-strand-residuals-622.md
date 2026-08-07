## [Unreleased] — fix(router): close two residuals from PR #622 (A35 follow-up)

**`newest-by-agent-counts-suspended`**: `NewestNonTerminalByAgent` now excludes
SUSPENDED records in addition to terminal ones. Previously a suspended thread
could win the "newest" slot, causing `newestByAgent[agent]` to never match any
thread that `AgentArmed` evaluates (it skips SUSPENDED), silently denying the
agent-keyed rescue credit to the next-newest active thread.

**`threadarmed-unscoped-reentry`**: Deleted `threadArmed` (was dead code with
zero production callers after PR #622 converted all three call sites to
`threadArmedForNewest`). Retaining it as an unscoped wrapper created a live
re-entry point: a future caller reaching for the shorter name would silently
restore the unscoped probe and re-introduce the A35 blast-radius bug that #622
fixed. The single test that called it is updated to call
`threadArmedForNewest(thr, now, false)` directly — same semantics, explicit
contract.

Refs: PANTHEON_RULES.md §2.33 (A35), docs/ADR-INDEX.md; PR #622 (parent fix)
