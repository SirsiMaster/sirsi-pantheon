# Continuation Prompt — Sirsi Pantheon (claude-pantheon)

## Resume name: "Pantheon CLI shippability"

> Session boundary marker (Rule A15). Read `.thoth/memory.yaml` first, then this.
> Last session: 2026-06-05 — CLI shippability pass + v0.23.1-beta release.

## Identity & resume mechanics (DO FIRST)
- **You are `claude-pantheon`** — never `claude-home` (the SessionStart hook mis-tags
  by home cwd; `SIRSI_ROUTER_AGENT=claude-pantheon` in `~/.claude/settings.json`
  rescues it). See `[[feedback_i_am_claude_pantheon]]`.
- **Register + heartbeat**: `sirsi thread register --agent claude-pantheon --surface claude --repo .`
  then `sirsi thread heartbeat --thread <id>`.
- **Re-arm the A27 watcher (Monitor)**: a persistent file Monitor on
  `.agents/idea-router/items/` for `to: claude-pantheon`, keyed on the thread_id
  (so `pgrep -f <tid>` finds it), heartbeating each tick. This is the event-driven
  router monitor — without it, codex's items sit unread.
- **Pull inbox**: `sirsi router pull claude-pantheon`.

## What shipped this session (verified by running)
- **v0.23.1-beta released** — replaces the broken v0.23.0-beta (whose
  `sirsi clean --confirm` returned `unknown flag`). PR #3 merged to main (`199fb90`).
- **`sirsi clean` is functional**: unified to one A1 engine (`runJudge`) — preview
  by default, `--confirm` applies via `[y/N]` trash-first, `--include-caution`
  scope. Deleted dead `runClean`; removed the `cleanupApplyPaused` demo brake from
  the clean path. A1 evidence: pure `selectCleanTargets` (no confirm param) +
  preview==apply test.
- **Honest CLI**: 5 dead "Launch TUI to…" next-actions → real `scan → clean --confirm`;
  `audit` fast-by-default (`--full` for slow); docs `sirsi hapi detect` → `sirsi hardware`.
- **CI runtime-smoke matrix**: CI now EXECUTES the CLI on mac/linux/windows
  (version, JSON, `--help` for 19 commands) — cross-platform PROVEN, not assumed.
- **4 surfaces** exist + build: CLI, `sirsi tui` (live thread/inbox data, fixture
  Scan), menubar (running), `sirsi-gui` (webview over dashboard, builds).

## OPEN TODOs (next session)
1. **Sync main VERSION → 0.23.1-beta.** The bump landed on `feat/setup-wizard`
   (`db6228d`, which the tag points to — release is correct) but a dirty working
   tree (router-item churn + `docs/UX_AUDIT_2026-06-01.md`) blocked the checkout, so
   **main still reads 0.23.0-beta**. Clean the tree, `git checkout main`,
   cherry-pick `db6228d` (or set VERSION), push.
2. **Verify v0.23.1-beta artifacts** — confirm the new DMG/install carries the
   working `clean --confirm` (download + run, don't assume).
3. **Codex's #1 mandate — productize the router supervisor**: ONE LaunchAgent from
   `sirsi setup`, replacing ad-hoc per-agent heartbeat glue. This is the durable
   fix for the codex↔claude continuous work/review loop (so neither agent ever
   needs manual nudging). Codex owns the build; coordinate via router.
4. **Case-study numbers (A14)**: build-log/case-studies claim 64 GB / 27× etc. —
   unreproduced this session. Reproduce or label as historical.
5. **TUI live Scan view + actions** (it's a viewer; inspect/clean/refresh are inert)
   and **run `sirsi-gui`** to verify it renders (never launched).
6. **Inbox**: codex build-mandates (router supervisor) + an ADR-026 Horus
   ops-dashboard review (from old claude-home routing). Triage.

## Division of labor (agreed with codex)
- **claude-pantheon**: functional/UX/runtime + website honesty.
- **codex-pantheon**: source footprint, dead code, flags/docs drift, package bloat,
  build/test evidence, **router-supervisor productization**.

## Truth discipline (hard-won this session)
"Builds + renders" ≠ "shipping product." Verify every printed promise by RUNNING it;
state confidence per claim; don't swing to overclaim OR over-pessimism. The user is
the sole arbiter (A23).
