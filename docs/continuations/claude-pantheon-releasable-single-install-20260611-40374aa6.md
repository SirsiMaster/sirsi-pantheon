<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread.
     agent:      claude-pantheon
     workstream: releasable-single-install (native menubar + clean install/upgrade)
     repo:       sirsi-pantheon
     date:       2026-06-11
     session:    40374aa6-c64e-4613-a12d-cd07f96b3be0
     path:       docs/continuations/claude-pantheon-releasable-single-install-20260611-40374aa6.md
-->

# Continuation Prompt — resume the 2026-06-11 Pantheon "releasable single-install" session

> Read `~/.claude/projects/-Users-thekryptodragon/memory/MEMORY.md` first (auto-loads on
> SessionStart). You are **claude-pantheon** (the SessionStart hook mis-tags you claude-home
> off cwd — ignore it). Source repo `~/Development/sirsi-pantheon` is intact.
>
> **Pantheon is now INSTALLED (local dev build, 2026-06-11).** Built from the rebased #32
> branch: `~/.local/bin/sirsi` (CLI, has `insight`) + `~/Applications/Sirsi Menubar.app` (the
> **native NSPopover/SwiftUI menubar**, adhoc-signed `ai.sirsi.pantheon`), LaunchAgent
> `~/Library/LaunchAgents/ai.sirsi.pantheon.plist`. The user said **we are NOT waiting on Apple**
> — adhoc is fine for their own machine; the Developer-ID/notarization (#42) is only for
> distributing to others. **One-time action for the user: grant Full Disk Access** to the app
> (System Settings → Privacy → Full Disk Access). Since it's adhoc-signed, do NOT rebuild +
> redeploy it casually — that mints a new TCC identity and re-prompts FDA
> (`feedback_dont_churn_menubar_fda`). Build sources for a reinstall live in worktree
> `/tmp/pantheon-build` (branch `feat/adr-030-nspopover-menubar`).

## ⚠️ Operating rules the user set this session (honor them)
- **Do NOT re-arm caffeinate / Monitor / /loop / heartbeat** unless the user explicitly asks.
  They were repeatedly frustrated that the autonomy stack churns tokens while idle, and they
  uninstalled Pantheon. The stack is OFF. Leave it off. (`feedback_autoarm_stack_on_resume` is
  SUSPENDED until they re-ask.)
- **Be active = ship code, not infra theater.** Never report "tasks running but no work."
- **Don't redeploy the menubar app on every change** — each adhoc re-sign mints a new TCC
  identity and clutters Full Disk Access (`feedback_dont_churn_menubar_fda`). Build to verify;
  deploy once, only when delivering.

## The goal: a single, releasable install that upgrades cleanly and creates no mess
The user is enrolling in the **Apple Developer Program (approval PENDING)**. Everything else is
being built to the line so it's "one secret away from shippable."

### In-flight PRs (all mine, all BEHIND main → need rebase before merge)
- **#32** `feat/adr-030-nspopover-menubar` — the **native NSPopover/SwiftUI menubar** (ADR-030).
  All surfaces wired: ✨ Insight panel (`sirsi insight --json` + Ask-Gemma), 🐺 Anubis
  (Clean/Scan/Find-Leftover-Apps), 𓂀 Horus (health), 𓆄 Ma'at (audit), 𓁟 Thoth (status),
  𓇶 Ra (status), ⚠️ FDA grant — via a reusable `CommandView`. Zero dead-ends. **Awaiting codex
  binding review** (codex is back). Worktree `/private/tmp/sirsi-adr030`.
- **#42** `feat/release-signing-notarization` — corrected `scripts/build-dmg.sh` (it was
  notarizing the DMG *before creating it*, used a keychain-profile CI lacks, built the old fyne
  menubar). Now: native-app-aware → inside-out Developer-ID sign + hardened runtime → DMG →
  `notarytool submit --wait` → `stapler staple`. `docs/RELEASE_SIGNING.md` lists the 7 secrets.
  **Dormant until the Apple cert exists.** Worktree `/private/tmp/sirsi-signing`.
- **#41** `feat/sirsi-uninstall` — `sirsi uninstall` (dry-run default; Trash apps; `tccutil reset`
  its own bundle id). Clean self-removal. Worktree `/private/tmp/sirsi-uninstall`. (Verify if merged.)
- Merged this session: #31 (menubar visible/safe-only), #33 (AI caches → caution; caught a 30 GB
  HuggingFace one-click-trash — those weights ARE the local Gemma models), #34 (`sirsi insight`
  AI-optional), #35 (canonical router), #36 (binding-hold gate), #37 (session record).

### Release-readiness scorecard
| Piece | State |
|---|---|
| Single install (`brew install --cask sirsimaster/tools/sirsi-pantheon`) | ✅ exists |
| Clean uninstall w/ FDA reset | ✅ #41 |
| **Developer ID signing + notarization** | ✅ wired (#42) — **blocked on Apple approval** |
| Native menubar shipped as the `.app` | 🟡 #32 in codex review; build-dmg.sh auto-uses it once `macapp/` is on main |
| FDA from the bundle only (never bare CLI) | 🟡 the curl-install path still drops bare binaries — bundle/cask is fine |
| Cask version tracks real release (stuck at 0.18.0) | 🟡 auto-syncs on next tagged release |

### The day Apple approves — exact path
1. Add 7 CI secrets (all in `docs/RELEASE_SIGNING.md`: `MACOS_CERTIFICATE`/`_PWD`, `KEYCHAIN_PWD`,
   `DEVELOPER_ID_APPLICATION`, `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_PASSWORD`).
2. Rebase + merge #32, #41, #42 (codex binds the safety-relevant ones).
3. `git tag vX.Y.Z` → CI builds a signed + notarized `Pantheon.app` DMG and bumps the cask.
4. `brew install --cask …` → no Gatekeeper warning, grant FDA once, `brew upgrade` never re-prompts.
5. Offer to run a real notarized build end-to-end to PROVE it (the user asked for proof).

## Other durable facts from this session
- **gcloud per-project accounts** wired: `~/Development/gcloud-project-switch.sh` (sourced from
  `~/.zshrc`) auto-selects `sirsi` / `finalwishes` / `assiduous` configs by directory.
  admin@sirsi.ai needs a one-time `gcloud auth login` (token expired). Memory:
  `feedback_gcloud_account_per_project`.
- **FDA cleanup truth**: `tccutil reset SystemPolicyAllFiles <bundle-id>` only works for bundle-id'd
  apps; bare-binary *path-keyed* FDA rows are NOT tccutil-targetable and the system TCC.db is
  write-locked → the user must remove those via System Settings (− button). `siriactionsd` is
  Apple Siri, NOT sirsi — never tell the user to remove it. A future `sirsi tcc` cleanup feature
  (read TCC.db → reset bundle ids → guide for the rest) is a good build, detect-and-guide like
  Spotlight Rail B.
- **sirsi-gemma** MCP works (local MLX Gemma 2 27B at `~/.venvs/mlx`); was registered then removed
  during uninstall. The 30 GB `~/.cache/huggingface/hub` = the Gemma weights.

## Resume one-liner
> "Pantheon: rebase + land #32 (native menubar) and #42 (signing) when codex/Apple are ready;
> the goal is the single clean notarized `brew` install that upgrades without FDA churn. Don't
> re-arm any loops/monitors. Apple Developer approval is pending."
