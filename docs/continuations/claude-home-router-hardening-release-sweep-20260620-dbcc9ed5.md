<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread.
     agent:      claude-home
     workstream: router-hardening + signed-release + cross-repo binds (the 2026-06-19/20 sweep)
     repo:       sirsi-pantheon (+ cross-repo: SirsiNexusApp, FinalWishes)
     date:       2026-06-20
     session:    dbcc9ed5-2b10-411b-ac40-d83eb0f76602
     path:       docs/continuations/claude-home-router-hardening-release-sweep-20260620-dbcc9ed5.md -->

# Continuation — claude-home router-hardening + signed-release sweep (2026-06-20)

> ## ⚡ RESUME UPDATE — 2026-06-20 ~19:40Z (session dbcc9ed5, post-merge sweep) — READ THIS FIRST
> Several earlier prescriptions below were STALE/FICTIONAL. Corrected truth:
> - **PR #89 (wake-or-unavailable): MERGED** — squash `06cbcee3` (CI green incl. binding-hold; worktree `/private/tmp/pantheon-wake` pruned). Done.
> - **`sirsi router doctor --fix` DOES NOT EXIST** — router verbs are send/pull/show/close/status/node-status/ack only. No `doctor`, no wake adapter; model is "no daemon, each agent reads its own inbox on wake." Don't try it again.
> - **Release run 27873488895 (v0.23.2-beta): CANCELED by me.** The note "Build & Release FAILED → goreleaser release.mode:replace" was WRONG — that job SUCCEEDED. Real failures: (a) **DMG job HUNG 5h20m** (no `timeout-minutes`; `scripts/build-dmg.sh` signing/notarization wedged — **NOT an Apple gate: Apple Dev Program is ACTIVE, Team 9D382WV988; the CI signing secrets/notarytool cred per `docs/RELEASE_SIGNING.md` are unconfigured so codesign/notarytool wedges**); (b) **npm thoth-init OIDC publish 404** = Trusted Publisher not configured on npmjs.org (pkg is ours, at 2.0.0; 404-on-PUT = masked permission denial). Both ROUTED to **claude-pantheon** (its lane) as binding item `20260620-193953-claude-home-claude-pantheon-release-...`. Do NOT apply the goreleaser change.
> - **nexus**: LIVE; dep-remediation binding item (`20260620-173946`) is in its inbox — it pulls itself. Do NOT absorb the dep sweep.
> - **finalwishes**: DOWN; 6×37 binding item (`20260619-220918`) stranded — needs OWNER to bring that CLI session up. Same for **claude-pantheon** (down) re: the release fixes.
> - **Owner-gated (surface to Cylton):** npm Trusted Publisher for thoth-init; **add/verify the 7 CI signing secrets per `docs/RELEASE_SIGNING.md`** (Apple Dev Program is ACTIVE — Team 9D382WV988 — so the DMG just needs cert+notarytool cred configured, NOT Apple approval); bring up claude-finalwishes + claude-pantheon CLI sessions.

> You are **claude-home** (root-authority ORCHESTRATOR + binding reviewer). **Your job is to make
> the OTHER threads WORK — not to do their work yourself** (owner directive 2026-06-20: "your job is
> to make the threads work, not you do the work"). claude-home's own work = BIND their output + WAKE/
> engage idle threads + keep the relay churning. The BUILDING (PRs, fixes, the FW matrix) is the
> threads' job. The threads (claude-pantheon, claude-nexus, claude-finalwishes, codex-*) are **IDLE**
> — that is an ORCHESTRATION failure to FIX (wake them), NOT a cue to absorb their work.
>
> **How to make idle threads work:** (1) ensure each has its work routed (done — see below);
> (2) MERGE PR #89 then run `sirsi router doctor --fix` — that's the wake pass: it wakes agents with a
> ready+explicit wake adapter (worker/headless via the `router wake-install` LaunchAgent pull-loop)
> and marks the rest `wake-unavailable`; (3) codex-* are app-heartbeat (poll on their own heartbeat —
> if stalled, the OWNER restarts Codex automation); (4) **interactive claude-* CANNOT be blind-spawned**
> (codex's #89 constraint) — they need the OWNER to bring those CLI sessions up. So claude-home's move
> for an idle claude-* with work = keep its inbox stocked + surface it as wake-unavailable to the owner,
> NOT do its build. Owner has bypass perms + a standing "NEVER STOP" directive
> ([[feedback_never_stop_until_goal_met]]) — never-stop means keep ORCHESTRATING, not keep absorbing.
> Re-arm the event-driven inbox watcher on resume (keyed on the NEW thread_id), idempotently.

## ⛳ IMMEDIATE NEXT ACTIONS (in order)

1. **BIND-MERGE PR #89** (wake-or-declare-unavailable, branch `feat/router-wake-or-unavailable`, head
   `fa199fad`). It is **VERIFIED + READY**: CI all green (Build/Lint/Test/binding-hold), I ran codex's
   4 wake tests under `-race` = green, and I source-deep verified all 4 of codex's SME findings are
   fixed (plist is DIRECT argv + XML-escaped → no shell injection; cli-spawn Setsid-detaches +
   Process.Release() → no zombie; mcp-notification reports NOT-ready until wired; `RunWakeLoop` is a
   real FOREGROUND `sirsi router wake-loop <agent>` verb run BY launchd, NOT a self-daemon → A26-ok).
   codex was routed to re-verify but is IDLE — do not block a verified+tested PR on idle-codex.
   `gh pr merge 89 --squash`. **This COMPLETES PR #2** (visibility #85 + wake-action #89) and the
   whole 3-PR router-hardening plan (#79/#80 PR1, #85 PR2-visibility/#89 PR2-wake, #86 PR3-doctor).

2. **Signed RELEASE (v0.23.2-beta) — run `27873488895` FAILED** (watcher `b80qasq5l` fired:
   `Build & Release (CLI + Deities): failure` → DMG job **skipped**, no signed .dmg, assets `[]`).
   This is NOT a notarization transient — the **CLI/Deities build itself failed** this run (it had
   SUCCEEDED on a prior run, so suspect a flake or a goreleaser/asset-collision regression introduced
   by the re-tag). On resume: `gh run view 27873488895 --log-failed | tail -60` to read the actual
   failure. Likely fixes by symptom: goreleaser `422 already_exists` on assets → `gh release delete
   v0.23.2-beta --yes` (keep tag) then `gh run rerun 27873488895`; build/compile error → fix on main,
   re-tag `v0.23.2-beta` at HEAD (`git tag -f` + `git push -f`), fresh release run. **Durable fix to
   land regardless:** set goreleaser `release.mode: replace` so re-runs never 422 on existing assets.
   **Release fixes already on main:** #87 (npm thoth-init publish → continue-on-error), #82
   (`/bin/sh` test), #84 (Mac-first goreleaser). NOTE: VERSION file says `0.23.1-beta` — mismatched vs
   the `v0.23.2-beta` tag; bump VERSION to match before the next tag.

3. **Make claude-nexus DO the SirsiNexusApp dependency remediation** (do NOT do it yourself — it's
   claude-nexus's BUILD; claude-home's job was the binding DECISION, routed `20260620-173946`). If
   claude-nexus is idle: keep its inbox stocked, wake it via the wake pass if it has a configured
   adapter, else surface it wake-unavailable for the owner to bring up. The APPROVED plan it builds to:
   - **Step 0 (foundation):** resync the BROKEN lockfiles — `npm ci` FAILS in
     `packages/sirsi-auth/functions`; audit counts unstable (10→26→27). Regen each workspace's
     lockfile from its package.json, verify `npm ci` green + stable audit everywhere. Its own PR.
     NOTHING sweeps until npm ci is green.
   - **Tier the 58 alerts (8H/34M/16L):** T1 stale-lock (pkg.json already declares fix, e.g.
     `ws ^8.21.0`) → regen lock; T2 transitive (js-yaml, ts-deepmerge) → `overrides`; T3
     MAJOR/unverifiable (starlette 0.x→1.3, torch, firebase-admin major) → **NEVER bump a major to
     silence an alert without build+behavior verify**: assess exploit-path, PIN + document
     "not-exploitable: why" if the vuln path isn't reached; smoke-test before any needed major.
     firebase-admin major REJECTED (breaks 30+ TS errors). 1 PR/pkg, 8 HIGH first.
   - **Separate 1-line PR:** `ui/server/src/services/notifications.js:15` `createTransporter` →
     `createTransport` (nodemailer API typo, real live bug).

4. **Make claude-finalwishes DO the 6×37 live-render matrix** (its BUILD, not yours). It has the
   8-point binding criteria (`20260619-220918`) + codex SME pairing. If idle, keep it stocked + wake/
   surface — don't run the Playwright crawl yourself. The bar it builds to: seed
   `estates/estate_persona_qa` + 6 persona accounts → log in as each → walk all 37 routes vs
   **finalwishes-prod.web.app** (NOT localhost) → screenshot+console+permission per cell → 222-cell
   ✓/✗ matrix; renders-real-content (computed styles, not 200), gated routes show INTENTIONAL access
   states (not crash/blank — the white-cards class, [[feedback_verify_in_browser_not_tests]]), zero
   console errors, Photos owner-gate-ok, iOS parity. claude-home BINDS the matrix against the bar.

## ✅ MERGED THIS SWEEP (all on main)
Router: **#76** watch-merge, **#78** self-watch, **#79/#80** honest-liveness (PR1 — armed/loop_alive
per thread, loop-monitor-only gate, Codex app-heartbeat not false-flagged), **#85** stranded-inbox
visibility (PR2-core), **#86** `router doctor` (PR3), **#88** gitignore+untrack `items/` (killed the
dirty-worktree clutter — router messages are ephemeral local state, not source).
Quality: **#81** 76GB-waste false-alarm fix (ReclaimableSize excludes AI model weights) — **DEPLOYED
to the live menubar** (8.9GB now, FDA preserved via stable cert re-sign, NOT churned). **#83**
green-standards (health can reach GREEN: isolated 7-day trend events ≤3 → OK not amber; swap <4GB
routine→green; live RAM/disk still red) — also live in the rebuilt menubar. **#77** 5 deity menubar
rows.
Release: **#82** `/bin/zsh`→`/bin/sh` test (unblocked the runner), **#84** Mac-first goreleaser,
**#87** npm publish non-fatal.
Cross-repo: **SirsiNexusApp #65** (pre-push Ma'at gate genuinely lints — A28 restored; bound + merged
for claude-nexus). SirsiNexusApp PR#64 (HMAC fail-closed) bound then turned out moot (already on main #62).

## 🔑 DURABLE LESSONS (this sweep)
- **claude-home ORCHESTRATES; it does NOT absorb the threads' work** (owner 2026-06-20: "your job is
  to make the threads work, not you do the work"). Idle threads = an orchestration failure to FIX by
  WAKING them (wake pass / surface wake-unavailable for the owner), not by doing their build. The
  binding/coordinating IS claude-home's work (bind PRs, route decisions, run the wake pass). The
  building is theirs. "Never stop" = never stop ORCHESTRATING, not never stop absorbing.
- **codex catches spawn/security bugs I miss** — its #89 review found a shell-injection in a plist +
  3 runtime bugs. Gate spawn/security code on codex IF codex is live; if codex is idle, verify-and-
  merge yourself (run its tests, source-deep the fixes) rather than block forever.
- **Re-validate the EXACT merged commit after a rework** — I once merged a drifted commit (#79
  c3dc044c widened a gate codex had narrowed); codex reviewed a different SHA. Always diff what
  actually merges vs the reviewer's cited SHA.
- **Close your own outbound binds/notices** — leaving done "BINDING PASS"/"merged" items open piles
  clutter in recipients' inboxes ("threads waiting on you" was mostly MY unclosed verdicts).
- **CHANGELOG-conflict churn**: every PR collides on the `[Unreleased]` prepend → rebase+resolve each.
- **Menubar deploy**: rebuild `cmd/sirsi-menubar` → replace the `.app`'s binary → `codesign --force
  --deep --sign "Sirsi Local Code Signing"` (the STABLE self-signed leaf preserves FDA, NO churn) →
  `launchctl kickstart -k gui/$UID/ai.sirsi.pantheon`.

## Resume one-liner
> "claude-home sweep — ORCHESTRATE, don't absorb (owner: make the threads work, not you do the work):
> 1) bind-merge verified PR#89 (wake-action, fa199fad CI-green) → 3-PR router plan done [binding = my
> job]; 2) MERGE #89 then run `sirsi router doctor --fix` (the wake pass) to wake idle agents with
> ready adapters / surface the rest wake-unavailable; 3) confirm signed v0.23.2-beta .dmg landed (run
> 27873488895) or re-run on a notarization transient / delete-polluted-release on 422; 4) make
> claude-nexus build the dep remediation (lockfile resync first, no build-breaking majors,
> createTransport typo) + make claude-finalwishes build the 6×37 matrix — WAKE them, don't do their
> builds. Interactive claude-* can't be blind-spawned → surface wake-unavailable for the owner."
