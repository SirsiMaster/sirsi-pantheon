# Continuation Prompt — Resume the 2026-06-08 Session

> Read `~/.claude/projects/-Users-thekryptodragon/memory/MEMORY.md` first.
> The Pantheon line + `[Codex-Standin / No-Self-Review]` feedback have the
> durable rules. This file holds the in-flight state.

## Identity (READ THIS FIRST)

- **You are `claude-home`** when wearing the unified Horus-ops + codex-standin
  hat during codex's OOO window (~returns 2026-06-10). Register as `claude-home`;
  close any spurious `claude-pantheon` thread the SessionStart hook auto-mints on
  each wakeup (known bug — hook's override gate excludes cwd-resolved cases).
- **Canonical `claude-pantheon`** is a different thread (source-edit lane). Do
  NOT impersonate it. Address its work to it.
- **No-self-review rule**: standin renders **binding PASS-ACK only on cross-repo
  / non-pantheon-authored items**. Pantheon's own PRs → advisory only.
  Safety/A1 → held entirely for real codex.

## In-flight chips (parallel, spawned 2026-06-08)

1. **Chip A — Install MLX + Gemma 2 27B 4-bit on M5 Max** (`task_02957437`).
   PR `feat/mlx-gemma-setup`. Outputs `docs/setup/MLX_GEMMA_LOCAL.md` +
   `scripts/gemma-smoke.sh` with measured tok/sec.
2. **Chip B — `cmd/sirsi-gemma/` MCP server** (`task_a95056a4`). PR
   `feat/sirsi-gemma-mcp-server`. Exposes `gemma_chat` + `gemma_complete`. After
   merge: paste config snippet into `~/.claude/mcp.json` → restart Claude Code
   → I gain Gemma tools in any session ("when tokens run out / offline" story).

## PRs OPEN (don't merge with standin verdict alone)

- **PR #8** `chore/remove-dead-pushmodel-router` — refactor(router): −2,626 LOC.
  Codex pre-approved scope (item 044213); surgical extraction of live Wake* +
  binary-resolver symbols into `wakemechanism.go`. **Holding for non-standin
  cross-eyes** (claude-finalwishes OR real codex on return).
- **PR #9** `docs/adr-sqlite-lean-build` — ADR-028 Optional SQLite (`nosqlite`).
  Holding for real codex.

## ADRs Proposed

- **ADR-027 Router Menubar Surface** — per-mailbox drill-down + override-act +
  `⚡ Caffeinate-router` (hidden until dead). 3 slices A/B/C. Standin-approved
  with 2 notes + 1 naming nit (keep `SIRSI_CAFFEINATE_DISABLE=1`). Codex final.
- **ADR-028 Optional SQLite** — only real size lever (15→~10.6 MB). Metal cgo
  gate measured as non-win. Vault FTS5: **graceful-degrade**, not fail-loud.
  Build-tag `-tags nosqlite` opt-in cleaner than runtime detect.

## Shipped THIS session

- `969edc0` fix(setup): `SirsiBinaryPath` — sibling-of-`os.Executable()`
  resolver fixes menubar's `exec: "sirsi": executable file not found in $PATH`.
  3 call sites consolidated. Net −24 LOC. Menubar reinstalled live (pid 61815).
- `gh repo edit SirsiMaster/FinalWishes --visibility public` — owner-authorized;
  branch protection now wireable on free plan.
- FinalWishes Soul Log + hardening PASS-ACKed with 3 follow-up flags.
- Identity unified: closed `claude-codex-standin`; registered as `claude-home`;
  closed duplicate claude-pantheon threads.

## Open inbox at session end

- `claude-home`: 0 open (verify `sirsi router pull claude-home`).
- `claude-codex-standin` (retired alias): 0 open (verify; watcher canvases this
  too during transition).
- `claude-pantheon`: 1 open (Monday-ready audit `20260605-191735`) — NOT mine.

## Auto-registration confirmed

All claude / codex / resident-surface threads self-register via SessionStart
hooks + codex's `ctr-thread-wake` + menubar/supervisor's native runloop.
**No manual register needed** for those — only for the unified-identity claude-home
override (which the hook gate currently misses).

## Resume command

```sh
# 1. Re-register as claude-home (the hook will auto-mint claude-pantheon — close it).
sirsi thread register --agent claude-home --surface claude --repo ~/Development/sirsi-pantheon

# 2. Arm full-canvas watcher — see ~/.claude/projects/-Users-thekryptodragon/memory/reference_monitor_verification.md
#    Watches BOTH claude-home AND claude-codex-standin during transition.

# 3. Drain both inboxes.
sirsi router pull claude-home
sirsi router pull claude-codex-standin

# 4. Check chip status.
mcp__ccd_session_mgmt__list_sessions   # see if A/B finished
gh pr list --state open                # PRs #8, #9, plus chip A/B if pushed
```

If codex has returned (~2026-06-10):

```sh
# Standin role retires.
# 1. Stop addressing standin work to claude-home/claude-codex-standin —
#    route directly to codex-pantheon.
# 2. Re-route any held PRs to real codex for binding review.
```

Refs: ADR-INDEX (ADR-027, ADR-028), MEMORY.md (Pantheon line),
`feedback_codex_standin_no_self_review.md`, `reference_monitor_verification.md`.
