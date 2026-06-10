# ADR-029: Per-Agent Session Worktrees for Source Edits

## Status
**Accepted** — 2026-06-09

## Context

sirsi-pantheon is worked on by multiple AI agents concurrently (claude-pantheon,
codex-pantheon, claude-home/standin, menubar/TUI surfaces) plus the human
founder. Historically they all operated on **one shared working tree** at
`/Users/thekryptodragon/Development/sirsi-pantheon` against a single `.git`.

This shared-`.git` model has repeatedly corrupted under concurrent access:

- **`core.bare` flips to `true`** mid-session. When two agents run git operations
  against the same `.git` concurrently (or a `git stash -u` with a mixed
  tracked/untracked pathspec races another op), the repo's `core.bare` config
  gets flipped to `true`. Every subsequent work-tree command then fails with
  `fatal: this operation must be run in a work tree` until someone runs
  `git config core.bare false`. Observed **twice in a single session** (2026-06-09)
  and it previously ate a PR push attempt (the #14 routed fix).
- **Cross-branch contamination.** With one working tree, an agent that commits on
  branch A while another is mid-edit on branch B can land changes on the wrong
  branch, or carry another session's uncommitted WIP across a checkout. Observed:
  a routed fix nearly committed onto a peer's `fix/menubar-*` branch; menubar WIP
  repeatedly trailing across branch switches.
- **Stale-diagnostic confusion.** Tooling that caches file state by absolute path
  sees one tree mutate under it, producing phantom compile errors.

These are not incidental bugs; they are structural consequences of N writers on
one `.git`. The flagship health work also showed the cost is not just developer
friction — the write-churn from concurrent edits + the registry accretion it
accompanies feeds the Spotlight `mds_stores` → Jetsam loop the health surface
measures (see ADR / Rail C, Rail B).

`git worktree` already provides the isolation primitive, and the repo is *already*
partly structured for it: ephemeral agent worktrees appear under
`.claude/worktrees/`. This ADR makes that the **default, codified** model.

## Decision

**Every agent session that intends to edit pantheon source MUST work in its own
git worktree, not the shared root checkout.**

1. **One worktree per session.** Create it under a session-scoped path:
   `.claude/worktrees/<agent>-<session>/` (the harness `EnterWorktree` does this;
   `git worktree add` is the manual equivalent). The worktree gets its own branch
   off `origin/<default>`.
2. **The shared root checkout is for read/coordination only** — router I/O, `git
   log`, status inspection. Agents do not commit source there.
3. **Commits + pushes happen from the worktree.** Each worktree has an isolated
   working tree and index, so concurrent agents never share `core.bare`-flipping
   git operations or contaminate each other's branches.
4. **Branch hygiene unchanged.** PRs still target `main`; the Ma'at pre-push gate
   and CI run from the worktree exactly as from the root.
5. **Cleanup.** Remove the worktree when the session ends (`ExitWorktree`, or
   `git worktree remove`); unchanged worktrees are auto-pruned.

### Tooling caveat (recorded so it doesn't bite)

After entering a worktree, an absolute path that still points at the **root**
checkout edits the *root*, not the worktree. Editors/agents must target the
worktree path explicitly (or `cp` files in). This bit the flagship work once and
is the single sharp edge of the model.

## Alternatives Considered

- **Keep the shared tree, add a lock.** A cross-agent advisory lock around git
  operations would serialize writers, but it does not solve cross-branch
  contamination (still one working tree, one HEAD) and adds a coordination
  point that fails open under crash. Rejected — worktrees solve both problems
  structurally with no lock.
- **One clone per agent (separate `.git` each).** Full isolation, but heavy:
  N full object stores, N fetches, and cross-agent `git log` visibility is lost.
  Worktrees share one object store (cheap) while isolating the working tree +
  index + HEAD — strictly better for this use case.
- **Do nothing / fix `core.bare` reactively.** What we had. The flip recurs and
  silently breaks pushes and watchers. Rejected — treating the symptom, not the
  cause.

## Consequences

**Positive**
- The `core.bare` corruption class is eliminated — isolated working trees don't
  share the work-tree-mutating operations that flip it.
- No cross-branch contamination — each session's edits live on its own branch in
  its own tree.
- Concurrent agents scale without coordination — N worktrees, one object store.
- Feeds the flagship: less shared-tree write-churn → measurably calmer
  Spotlight/Jetsam, which Rail C's thermometer can show.

**Negative / cost**
- The absolute-path tooling caveat (above) — must edit the worktree path.
- Slight disk for each worktree's working copy (object store is shared, so this
  is just the checked-out files).
- Worktree lifecycle to manage (create on session start, remove on end).

**Follow-ups**
- The SessionStart supervisor could mint a session worktree automatically when a
  source-edit session is detected (pairs with the per-resume thread-mint fix —
  the registry-accretion root — landed alongside this ADR).

## Amendment 1 — Canonical Router (2026-06-10)

**Problem found in practice.** The worktree model introduced a regression the
original ADR missed: `.agents/idea-router/` is git-tracked, so every per-agent
worktree carries its **own snapshot copy** of the router. `sirsi router`/`thread`
resolved the router by walking up from `os.Getwd()` (`router.FindRepoRoot`), so a
command run from a worktree cwd read/wrote that **stale local copy**, invisible to
the live root router and to the agents watching it. A binding-review request routed
from a worktree was silently dropped — a broken review gate, the worst failure mode
for the relay (audit 2026-06-10 found exactly one lost item: a #33 safety-review
request that never reached the binding reviewer).

**Ruling (claude-home, root-authority).** The worktree model **REQUIRES a single
canonical router**. `FindRepoRoot` now resolves the **main worktree root** first —
the shared `.git`'s parent via `git rev-parse --path-format=absolute
--git-common-dir` — and only trusts it when the live `.agents/idea-router/` actually
lives there; otherwise it falls back to the original cwd walk-up (non-git checkouts,
tests). cwd no longer determines which router you hit. Verified: the fixed binary,
run from a worktree cwd, resolves `Router home` to the repo root, not the worktree
copy.

**Follow-up (optional hardening).** The per-worktree copy still exists on disk
(git-tracked); the resolve-to-root fix neutralizes its harm but a future cleanup
could `.gitignore` the volatile router state (`items/`, `state.json`) so worktrees
don't carry a snapshot at all.

## References
- ADR-022 (CTR OS-truth liveness), ADR-024 (one-watcher-per-surface) — the
  registry identity model the shared-tree churn corrupted.
- Router 20260609-050500 (claude-home steer: foundation = per-resume mint fix +
  per-agent-worktree ADR).
- `.claude/hooks/router_inbox_check.py` — `claude_session_pid()` ancestry-walk
  fix (the per-resume mint root), shipped with this ADR.
- PANTHEON_RULES.md A12 (additive), A22 (architecture decisions), A28 (CI gate).
