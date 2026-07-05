<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread.
     agent: claude-pantheon | workstream: incident-canon-phase2 | repo: sirsi-pantheon | date: 2026-07-04 | session: d8b52186-bc0c-4db2-b5d5-664de54b5ecc | path: docs/continuations/claude-pantheon-incident-canon-phase2-20260704-d8b52186.md -->

# Continuation — claude-pantheon · incident-canon + Router-v2 Phase 2 (2026-07-04)

## What landed this session (all merged or auto-merge-armed)

- **#162 MERGED** — fabric board truth: nil `LaunchctlChecker` now defaults to real launchctl; probe is `launchctl list <label>` (`print` without a domain target always exits 64 — loaded daemons read false forever).
- **#163 MERGED** — incident canon: case study `docs/case-studies/2026-07-04-runaway-executor.md`, **ADR-035** (runaway-proof execution, six axioms), 𓁵 Sekhmet "Runaway Executor" doctor check + `sirsi router quarantine-worker` kill switch (bootout + `.plist.quarantined` rename, `ai.sirsi.claude-worker.*` ONLY), BUILD_LOG + case-studies.html (render-verified) + user guide.
- **#164 (auto-merge armed)** — Router v2 **Phase 2**: §2b Dispatch Contract implemented verbatim in `internal/routerstore` (lease.go/facade.go/breaker.go/dispatch.go + migration v2). Acceptance-bar safety tests reproduce BOTH incidents, green with `-race`. **Worker stays OFF** — re-arm gates on Phase 3 wiring every live path through the facade.
- **#165 (auto-merge armed)** — Sekhmet tree-threshold recalibration (warn 300→1500): a heavy dev day measured 389 young trees and read amber on healthy work.

## Owner-visible fixes verified live

- **Menubar Router—Fabric board now truthful** (owner screenshot complaint "still not done" → fixed end-to-end, verified via computer-use screenshot: "Fabric healthy — no blockers"). THREE stale layers were lying: (1) deployed CLI predated #162; (2) `~/.local/bin/sirsi-router-board.sh` (UN-VERSIONED owner-side script) jq-projected away the `legacy` field — hand-patched, backup `.bak-20260704`, spawn-task chip filed to move the board writer into the binary; (3) supervisor kickstarted onto the fresh binary.
- **9 wake-loops live** (`ai.sirsi.router.wake.*`, 9 loaded w/ PIDs), supervisor loaded, Sekhmet check live in `sirsi doctor` (fix lever verified: dry-run + real quarantine of the leftover `ai.sirsi.claude-worker.claude-pantheon.plist`).
- Deployed binary: `~/.local/bin/sirsi` rebuilt from main @ #163, version-stamped v0.23.8-beta (was unstamped "dev"), AMFI-safe rm+cp+codesign. Router inbox cleared (3 sweep-probes closed w/ relay proof + registry-police item closed; `user` inbox holds 1 genuine owner-gated IAM item for SirsiNexusApp CI).

## Next actions (in order)

1. **Confirm #164 + #165 merged** (auto-merge armed; check `gh pr list`). Then rebuild + redeploy `~/.local/bin/sirsi` (rm+cp+codesign) and `launchctl kickstart -k gui/$UID/ai.sirsi.horus.agent-router`; verify `sirsi doctor` returns green (the 389-tree warn clears with #165).
2. **Router v2 Phase 3** (PRD `docs/prd/ROUTER_V2_DURABLE_DISPATCH.md`): one facade — `sirsi router *` verbs AND the six MCP `router_*` handlers call `internal/routerstore` (SendGuarded/ClaimNext/...); delete duplicated logic; cross-path test proves single source of truth. **Worker stays OFF until Phase 3 lands and the §2b bar re-verifies end-to-end.**
3. **Board-writer adoption** (spawn-task chip `task_3bea877f` filed): replace `~/.local/bin/sirsi-router-board.sh` with a `sirsi` verb/supervisor duty + field-fidelity test vs the menubar decoders (`macapp/.../SirsiEngine.swift`).
4. **Known flake (pre-existing):** `TestUXContract_JSONClean/audit_json` — `maat audit --skip-test` silently falls back to a FULL `go test -cover ./...` when `~/.config/sirsi/maat/coverage-cache.json` is absent/incomplete (>5 min on a loaded host vs the test's 120s budget). Cache warmed this session; a real fix = never run full coverage under `--skip-test` (fail soft with "cache cold").
5. **v1.0.0-rc1 tag is OWNER-GATED** — evidence assembled: Ma'at feather weight 100/100 (49 checks), CI green on main, #142 TUI merged, VERSION 0.23.8-beta. Cylton decides the tag.

## Watch out

- Pantheon = shared bare repo + multi-worktree; lanes live under /private/tmp/lane-*. lane-incident was reused for 3 branches this session — check `git status -sb` before committing.
- The menubar home-row Router badge caches its blocker count; drill-in refreshes. Owner runs the SwiftUI `~/Applications/Sirsi Menubar.app` (built Jul 2 23:14, has the legacy filter — no rebuild needed).
- Sekhmet thresholds: sessions 6/12, trees 1500/4000 (post-#165). Baseline data: heavy dev day = 389 trees.

---

## 2026-07-05 UPDATE — Phase 3 DONE (same thread, post-crash resume)

**Landed since the section above:**
- **#164, #165, #166 MERGED** (Phase 2 + thresholds + docs).
- **#167 MERGED** — maat fast mode NEVER runs the test suite: cold coverage cache warns honestly (`--full` named as remedy), fast runs don't rewrite the cache. Proof: cold-cache `audit_json` 120s-timeout-kill → 0.38s. This was the gate flake that blocked pushes.
- **#168 MERGED** — Router v2 **Phase 3**: `internal/dispatch` is THE facade for `sirsi router send/close` AND MCP `router_submit/poll/get/wait`. Store-first §2b guards at both surfaces (no store row, no dispatch; floods quota-refused; retries deduped), byte-identical `items/*.md` audit dual-write (`routerstore.ExportItem`), `router_wait` = real blocking wait (<250ms facade wake; 5s bounded re-check for legacy writers). The MCP pre-ADR-024 write path (proposals//reviews//decisions/ + state.json) is retired from writes; legacy ids readable. `router_submit` now REQUIRES `addressed_to`; author whitelist replaced by the full agent-id space. Cross-path tests prove /goal #3. `SIRSI_ROUTER_DB` env override sandboxes tests (one polluted live-store row from a pre-override run was purged). CI-only catch fixed: `dispatch.Open` MkdirAlls the store dir (fresh HOME = SQLITE_CANTOPEN otherwise).

**Session crash note:** the CCD session host was silently killed (no crash report = SIGKILL signature) during the concurrent full-test+lint memory-pressure window; no Pantheon process crashed (supervisor KeepAlive respawned; menubar + 9 wake-loops uninterrupted). Lesson: don't run multiple full `go test -cover ./...` suites concurrently with lint on this host.

**Live state:** deployed `~/.local/bin/sirsi` = main @ #168 (v0.23.8-beta stamped); supervisor kickstarted onto it; doctor 100 🟢; Sekhmet OK (1,129 fresh trees from the day's builds — below the #165 threshold, reaper drains); claude-pantheon inbox cleared (10 probes closed with relay proof). Worker still OFF + quarantined.

## Next actions (updated order)
1. **Phase 4 — migration + cutover** (PRD): `sirsi router migrate` importer (count-in==count-out), dual-read window, stop file writes after deprecation, README/A26/A27 + ADR updates. Then the owner-verified end-to-end §2b bar → worker re-arm decision (owner-gated).
2. **Board-writer into the binary** (spawn-task chip task_3bea877f): replace `~/.local/bin/sirsi-router-board.sh` with a `sirsi` verb + field-fidelity test vs the menubar decoders.
3. **v1.0.0-rc1 tag** — owner-gated; evidence standing (Ma'at 100/100, CI green, doctor 100).
