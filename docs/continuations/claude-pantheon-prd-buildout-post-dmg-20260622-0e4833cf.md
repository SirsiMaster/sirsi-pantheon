<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread.
     agent: claude-pantheon | workstream: prd-buildout-post-dmg | repo: sirsi-pantheon
     date: 2026-06-22 | session: 0e4833cf-2ad6-4183-8288-3d375c2d6429
     path: /Users/thekryptodragon/Development/sirsi-pantheon/docs/continuations/claude-pantheon-prd-buildout-post-dmg-20260622-0e4833cf.md -->

# Continuation — claude-pantheon · PRD buildout (post-DMG phase)

## Resume line (paste this to resume)
> "Pantheon: the signed+notarized DMG is SHIPPED (v0.23.2-beta) — headline goal DONE. Now work the
> 16 open PRD items in `.agents/prd/claude-pantheon.json`, starting with **P14** (menubar
> launcher-detection fix, Go-only, bounded). Implicitly arm the event-driven router watcher on
> resume (owner 2026-06-16); orchestrate-don't-absorb; verify UI in a real render, not tests."

## Supersedes
This thread **supersedes** `claude-pantheon-never-exhaust-governance-20260619` (its row was retired
from the index 2026-06-22). That workstream's in-flight PR **#71** (warm broker → NodeCapacity,
ADR-031-B) **merged 2026-06-19**; its remaining ADR-031-B resource-stack scope lives on as **P1–P4**
in the PRD below. The old continuation file stays on disk as history — do not resume from it.

## Identity
- **You are `claude-pantheon`** for all sirsi-pantheon work. The SessionStart hook mis-tags this as
  `claude-home` because cwd is the home dir (`/Users/thekryptodragon`) — ignore that tag; the
  PRD, prior continuations, and this session's PRs are all under the `claude-pantheon` lineage.
- See [[feedback_i_am_claude_pantheon]], [[feedback_orchestrate_dont_absorb]],
  [[feedback_verify_in_browser_not_tests]], [[feedback_autoarm_stack_on_resume]].

## ✅ Headline goal — DONE this session (2026-06-22)
The downloadable ships: **signed + notarized macOS DMG**.
- `SirsiPantheon-0.23.2-beta-arm64.dmg` on release **v0.23.2-beta**.
- Verified: `xcrun stapler validate` OK; `spctl --assess` = **"accepted, source=Notarized Developer ID"**.
- Apple notary stall was proven **Apple-side, not our pipeline** (minimal hello-world isolation
  test also stalled). Cleared after Apple Developer Support **Case 102921569682**
  (filed via browser under `ckcollymore@outlook.com`). All 5 notary submissions Accepted.
- CLI **v0.23.2-beta** published + brew-installable; binary verified to run.
- **Apple Developer Program is ACTIVE** — Team `9D382WV988`, expires 2027-06-16. Never say "pending".

### 3 PRs merged this session
- **#89** router *wake-or-declare-unavailable* (`internal/router/wake.go` + `wakemechanism.go` +
  `spawn_unix.go`/`spawn_windows.go` + `work.go` wake_* frontmatter; `cmd/sirsi/routerdoctor.go`
  `--fix` runs `WakePass`; hidden `router wake-loop` verb). Codex SME caught 4 real bugs — all fixed
  (plist real-argv, detached-spawn `Setsid`+`Release()`, mcp-not-ready honesty, no shell-injection).
- **#90** release robustness (`timeout-minutes: 30` on menubar job; idempotent npm publish guard;
  `notarytool --timeout 20m`).
- **#91** node-status honest readiness (`ProbeWakeReadiness` replaces the permissive wake_health
  block; mcp now reports not-ready until wired).

## ⚠️ Branch / worktree discipline (READ FIRST)
- This checkout (`~/Development/sirsi-pantheon`) was on a **peer's** branch
  `fix/sirsi-gemma-bare-server-chipA` at write time — sirsi-pantheon is a **shared bare repo +
  multi-worktree**. ALWAYS `git rev-parse --abbrev-ref HEAD` before committing; do new work in a
  **clean worktree off `origin/main`**, never on a peer's feature branch. See
  [[feedback_check_branch_before_commit_shared_worktree]].

## ▶️ Next task — P14 (recommended first; bounded, Go-only)
**Menubar launcher-detection fix.** `launcher.go` uses `exec.LookPath`, which trusts the
launchd-truncated PATH that **excludes `~/.local/bin`** → `claude` + `codex` render as
"not installed" in the menubar even though they're installed there.
- **Acceptance:** an augmented resolver finds `claude`+`codex` regardless of launchd PATH; unit
  test; PR bound by claude-home (codex SME-supports). Routed item `20260621-020216`.
- Fix shape: prepend the canonical user bins (`~/.local/bin`, Homebrew `bin`) to the lookup, or
  resolve against an explicit candidate list, rather than relying on inherited PATH.

## 16 open PRD items (`.agents/prd/claude-pantheon.json`)
P17 = done (the DMG). Remaining, grouped:
- **Resource stack (ADR-031-B):** P1 cgo memory-pressure watcher; P2 cold-path NodeCapacity
  re-point; P3 expose NodeCapacity for Ra; P4 Hapi pressure surface in menubar+CLI.
- **Anti-idle / supervisor:** P11 `router_wait` MCP (blocking long-poll); P12 `sirsi supervise`
  launchd backstop; P13 `sirsi prd sync` (derive PRD from canon); P15 reap 52 stale router regs.
- **Durability/ops:** P5 gemma-worker source-durability (`$MODEL` unbound @ runner line 192;
  worker script installed-only, not in-repo → reinstall re-breaks the daemon); P9 4 LaunchAgents
  loaded-but-not-running; P10 verify #71 merged + remove stale /tmp worktree.
- **Parity/roadmap:** P6 agent-ops parity (respond/review/ask/memory/watch/reap/insights in
  CLI+menubar); P7 cross-agent QoS via taskpolicy (no core-pin claims); P8 50-feature roadmap →
  CLI+menubar parity + downloadable from git AND product page; **P14** menubar detection (above).
- **Thesis surface:** P16 `substrate.html` (surface 4); surface 5 (sirsi.ai) is Rule-17 owner-gated.

## Router / orchestration state
- Inbox at resume: **3 pending items for claude-home** (the queued build tasks P11/P12/P13 etc.).
- **Owner rule — implicitly arm the event-driven router watcher EVERY resume** (owner 2026-06-16
  reversed the old suspension; `feedback_autoarm_stack_on_resume` ACTIVE). Event-driven Monitor
  (wakes on a real inbox item) — NOT a periodic ScheduleWakeup / `while true` heartbeat busy-loop.
- **Orchestrate, don't absorb:** idle threads → bind + wake them; don't do their build for them.
- Relay protocol A26/A27: claude-home holds the binding verdict, codex SME-supports; reply = a NEW
  inbound (`sirsi-respond.sh`), close+Result is audit-only and does NOT notify the sender.

## Don'ts (hard-won this session)
- Don't churn the menubar (adhoc re-sign mints a new TCC identity → FDA clutter). Build to verify;
  deploy once. [[feedback_dont_churn_menubar_fda]]
- Don't claim a UI works off tests/HTTP-200s — RENDER it. [[feedback_verify_in_browser_not_tests]]
- Never handle the Apple password / 2FA. App-specific password + Apple ID are notarytool/CI secrets
  only; the owner does interactive Apple sign-in.
- The "7 signing secrets are missing" claim was WRONG — they were already set. Verify before asserting.
