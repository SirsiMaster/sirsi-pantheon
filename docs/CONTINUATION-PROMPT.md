# Continuation Prompt — Resume the 2026-06-09 Pantheon Session

> **Read `~/.claude/projects/-Users-thekryptodragon/memory/MEMORY.md` first** — the
> Pantheon line (auto-loads on SessionStart) holds the dense durable state. This
> file holds the in-flight structure. Then `sirsi router pull claude-pantheon`.

## Identity & routing (READ FIRST)
- **You are `claude-pantheon`** — the source-edit lane (never `claude-home`). The
  SessionStart hook mis-tags by cwd; `SIRSI_ROUTER_AGENT=claude-pantheon` rescues it.
- **Codex is OOO → ~2026-06-10.** Route ALL review/arch-verify/PASS-ACK to the
  **standin = `claude-home`** (it absorbed the retired `claude-codex-standin`).
  Carve-outs (root-authority, non-negotiable): standin gives **advisory-only** on
  pantheon's OWN PRs (same-pid = no self-review); **A1/safety waits for real codex**;
  binding cross-eyes for my PRs go to a **non-standin claude** (claude-finalwishes).
- **Re-arm the watcher** keyed on your new thread_id (persistent Monitor over
  `.agents/idea-router/items/`, self-send + `*sweep-probe*` suppressed, 60s heartbeat).
- **Isolated worktrees for edits**: `git worktree add /tmp/<name> -b <branch> origin/main`.
  NEVER edit on the shared `~/Development/sirsi-pantheon` tree (concurrent agents +
  shared `.git` `core.bare` corrupt it; `git clean`/rebase there ate worktrees this week).

## Shipped this session (MERGED to main)
- **PR #17** — menubar **TCC stable-sign** (`InstallMenubar` re-signs `sirsi-menubar`
  with stable `ai.sirsi.pantheon` id). **This is the FDA-spam fix**: `go build`
  ad-hoc-signs with a content-hash id (`sirsi-menubar-<hash>`), so every rebuild was
  a new TCC identity → re-prompted FDA. Root of the "duplicate menubars asking FDA."
- **PR #16** — de-flake `TestStaleLockFiles` (drop `t.Parallel` / bare `.git`) + Ma'at
  gate new-branch diff-base scoping (`merge-base origin/main HEAD`). **Fleet push unblocker.**
- **PR #14** (diagnose), **#10** (sirsi-gemma MCP), **#7** (−14MB deity binaries).

## Landing (auto-merging on green) — verify they merged
- **PR #12** — **every command visibly resolves**: `rootCmd.SilenceUsage/SilenceErrors`
  + `output.Error` in `main()` (a failed command no longer dumps cobra Usage → looks
  broken). Fixed dead-ends: `risk` (graceful "needs a git repo"), `duplicates`
  (defaults to CWD, not all of `$HOME` → no more hang).
- **PR #15** — `permissions` + `quickstart` end with a real `output.CommandResult`
  (stacked on #12; GitHub auto-retargets after #12 merges).

## Flagship — IN FLIGHT via a concurrent claude-pantheon thread (DO NOT duplicate)
**Health → diagnosed cause → one-click remediation.** Order **C→A→B** (thermometer
first, so remediations are measurable). Killer dogfood: **`sirsi` is its own #1
crasher (21/61) via binary-drift** (the AMFI `cp`→SIGKILL-137 die-off).
- **PR #18 — Rail C** (Jetsam/panic trend-vs-transient surfacing, read-only): green,
  **held for codex**.
- **Rail A** (binary-drift remediation): running. Rails: **CLI-paths-only/A19** (never
  touch `/Applications/*.app` — bundle is guidance-only), **confirm-gated/A1** (never
  silent, never `--yes`), auth-gated, **read-only surfacing ships first**.

## Held for real codex (~06-10) — do NOT merge with standin verdict
- PR **#8** (router −2,626 LOC dead push-model cluster), **#9** (ADR-028 optional
  sqlite `nosqlite` lean build), **#13** (gemma 2-tool restriction).
- **A1/safety**: orphan-narrowing (`KillTrueOrphans`), diagnose→fix, menubar `--yes`
  funnels. Flagship Rail A/B merges.

## NEXT — highest-leverage, unowned, non-colliding (the real foundation)
**Fix the SessionStart per-resume thread-mint.** The hook mints a NEW
`claude-pantheon` thread every wakeup — it calls `sirsi thread register` with an
empty/zero PID, which **bypasses the `PID >= minAgentPID` idempotent-register
fast-path that already exists** (`ca6e343` + `cc14ae0`, with tests; Agent B
confirmed the register code is correct). Result: ~130 phantom records, duplicate
watchers, daily false A27 "not-looping" alarms, Spotlight→Jetsam churn. **The hook
is the driver, not the registry code.** Pair with **codifying per-agent
session-scoped worktrees** (kill the shared-`.git` corruption class). A fleet tool
whose own node registry can't be trusted undercuts the whole story — claude-home's #1.

## The dormant duplicate (A19 — operator's call)
`/Applications/Pantheon.app/.../sirsi-menubar` is a stale 2nd menubar binary (DMG
install, dormant — nothing launches it; now TCC-unified via `ai.sirsi.pantheon`).
A19 forbids deleting it. Either Trash `Pantheon.app` (keep the dev binary canonical)
or repoint the LaunchAgent at the bundle.

## Resume one-liner
> "Pantheon: fix the SessionStart per-resume thread-mint (root of registry
> accretion) + per-agent worktrees; confirm the #12/#15 auto-merge train landed;
> flagship Rail A/B + held PRs (#8/#9/#13) await codex's return ~06-10."
