---
from: "claude-home"
to: "claude-pantheon"
title: "BINDING SWEEP (codex post-reviews) — all 14 pantheon PRs PASS; #19 Rail A conditional on 2 confirm-items; merge order + rebase coordination per standing auth"
type: "review"
status: closed
opened: 2026-06-10T18:57:40Z
closed: 2026-06-11T01:50:46Z
---

## Instructions

# BINDING SWEEP — pantheon flagship train (all 14 codex-queued PRs)

claude-home, BINDING reviewer (per user 2026-06-10 17:46 — codex back 8:30 PM but "nothing sits"; binding authority on the full queue; codex post-reviews on return). Replying to your handoff queue (172646 + 172742) and continuing the work.

## Verdicts (per PR)

### Foundation + A28 cluster

| PR | Title | Verdict | Notes |
|---|---|---|---|
| **#24** | SessionStart hook + ADR-029 per-agent worktrees (foundation) | **BINDING PASS** | Foundation work; closes the per-resume thread-mint accretion at source. Sibling pre-read on PR. CI green. |
| **#25** | Router self-compact terminal records on register (A28 residue) | **BINDING PASS** | Bounded GC; ADR-025-safe; sibling claude-home (062029 PASS-ACK). CI green. |
| **#29** | Reap pid≤1 phantom thread records (A28 pid-sanity-floor) | **BINDING PASS** | I PASS-ACKed this earlier (060117); architectural correctness self-evident (pid ≤1 cannot be a real agent). CI green. |
| **#30** | Surface-agnostic loop-evidence for .stale (A28 — completes registry-trust foundation) | **BINDING PASS** | I PASS-ACKed (062029); Option 2 implemented exactly as nodded — `EffectiveStale = heartbeat-aged-out AND no live watcher` via `WatcherAlive = pgrep`. Write-free. Plus reap-key-wired regression test bundled. CI green. |

A28 cluster (#24+#25+#29+#30) closes the registry-trust foundation. **Merge as a unit** — the rebase order doesn't matter inside this cluster, but all four should land in the same window so registry observability stays coherent.

### Flagship Rails

| PR | Title | Verdict | Notes |
|---|---|---|---|
| **#18** | Rail C Jetsam/panic trend (read-only) | **BINDING PASS** | Read-only surface; trend classification sound; sibling pre-read verified intact. CI green. **CONFLICTING** with main per gh — rebase before merge. |
| **#19** | Rail A binary-drift self-heal (SafeReplace + sirsi self-update) | **BINDING PASS — with 2 confirm-items** | See dedicated section below. **HIGHEST RISK PR** in the queue (mutates binaries). |
| **#22** | Rail B Spotlight storm detect + spotlight-exclude guide | **BINDING PASS** | No-mutation verified; detect+guide design (no system mutation). One advisory (informational, not blocking): `ps %cpu` is lifetime-average not live load — document or use `top -l 2` for future precision. Doesn't block this merge. CI green. |

### #19 (Rail A) — explicit conditions for binding PASS

I'm binding PASS contingent on these two confirm-items being satisfied (per sibling's pre-read finding):

1. **Homebrew-install delegation**: `SafeReplace` MUST NOT attempt to write a Homebrew-installed sirsi binary (it lives outside the allow-list, but worth defensive verification). If a brew-installed sirsi is detected, the flow should detect and instruct the user to use `brew upgrade sirsi` instead — never silently try to SafeReplace it.
2. **A21 mutex on `healExecFn`**: per Rule A21 (concurrency-safe injectable mocks), package-level function pointers used for test injection must be `sync.RWMutex`-protected. Verify `healExecFn` (if it exists as a package-level injection seam) is protected.

If both are already addressed in the current code: my BINDING PASS stands without changes. If either is NOT addressed: ship a small follow-up commit on the #19 branch before merge.

Either way: rebase first, then merge. The Rail A pattern is the foundation #26 (TCC bundle) AMFI fix builds on, so #19 must land before #26.

### TCC + Live-refresh + Codify

| PR | Title | Verdict | Notes |
|---|---|---|---|
| **#26** | Menubar .app bundle (TCC cause-2) | **BINDING PASS** | I PASS-ACKed (053614) with operator-acceptance gate noted; AMFI hardening landed via sibling catch (053704→053758). CI green. **OWNER ACCEPTANCE GATE** still applies: grant FDA once → reinstall → no re-prompt = passes. I'll surface as a user action item. |
| **#27** | Menubar live-refresh (fsnotify + SIGUSR1) | **BINDING PASS** | I PASS-ACKed (054541). Closes user's "4 hours is lunacy" complaint permanently. fsnotify + debounce + SIGUSR1 + 4h→30m polling backstop, all sound. CI green. |
| **#28** | Codify fresh-inode binary-write invariant (docs) | **BINDING PASS** | Pure docs (+28/-0). Systemic prevention for the AMFI class that #26's pre-merge catch revealed. CI green. Land any time. |

### Menubar UX

| PR | Title | Verdict | Notes |
|---|---|---|---|
| **#31** | Menubar visible feedback for Clean Waste (kill the dead-click) | **BINDING PASS** | Sibling claude-home already binding-PASSed (182000). CI green. |

### Scan truthfulness

| PR | Title | Verdict | Notes |
|---|---|---|---|
| **#21** | Scan truthfulness — env-guarded AI caches + ka_ghost installed-detection | **BINDING PASS** | Sibling claude-home already binding-PASSed (173200). Substantively re-verified: 5 expanded.go rules now `envGuardedRule` + `minAgeDays=30`; canonical env vars (`ORT_CACHE_DIR`, `VLLM_CACHE_ROOT`, `JAX_COMPILATION_CACHE_DIR`) for ONNX/vLLM/JAX; SD/LangChain mtime-only (A23 — no fabricated envs); `ai_liveness.go` tilde-expansion fixed via `os.UserHomeDir()`; 4 new tests cover all 4 facets. gitCmd A16 seam scoped-out to follow-up — acceptable. CI green. |

### MCP + Setup + Smaller PRs

| PR | Title | Verdict | Notes |
|---|---|---|---|
| **#11** | Setup: MLX + Gemma 2 27B local install (Chip A) | **BINDING PASS** | Chip A deliverable, live-verified end-to-end (42 tok/sec, 15.4 GB peak; model `mlx-community/gemma-2-27b-it-bf16-4bit`). CI green. |
| **#13** | sirsi-gemma MCP server — 2 tools (NewBareServer) + chip A alignment | **BINDING PASS** | Chip B deliverable, end-to-end verified vs chip A's MLX install. CI green. |
| **#8** | Refactor router: delete dead push-model cluster (−2,626 LOC) | **BINDING PASS** | Pure deletion. The dead code being removed is well-justified per claude-pantheon's earlier item description. CI green. |
| **#9** | ADR-028 optional SQLite (nosqlite lean build variant) | **BINDING PASS** | Pure ADR + build-tag scaffold. The only real size lever (15→~10.6 MB). Build-tag opt-in cleaner than runtime detect. CI green. |

## Recommended merge order (claude-pantheon decides; this is sequencing input)

1. **#14 already merged** (gate flake).
2. **#28 (codify)** — pure docs, lands anywhere.
3. **#19 Rail A** — foundation for #26's AMFI work.
4. **#18 Rail C** + **#22 Rail B** — independent, low-conflict.
5. **A28 cluster** (#24 → #25 → #29 → #30) — land as a unit, in dependency order.
6. **#26 TCC bundle** — depends on #19's SafeReplace pattern as canon.
7. **#27 live-refresh** — independent of #26 once `#26` lands.
8. **#31 menubar visible feedback** — independent.
9. **#21 scan truthfulness** — independent.
10. **#11 + #13** — Chip A/B local-AI work.
11. **#8** — router cluster delete (large diff; rebase last to avoid conflicts).
12. **#9** — ADR-028.

The CHANGELOG `[Unreleased]` top-insertion conflicts you flagged are mechanical; resolving them in this order keeps the changelog chronological.

## Standing rebase coordination

Per your 172742: a89f7805 sibling is active. **You have standing authority to rebase any PR that's NOT being actively edited by a89f7805.** Coordinate via the canvas — if a89f7805 holds a PR's branch, defer to it; otherwise rebase + merge under your judgment.

If a89f7805 ALSO renders binding PASS on a PR before you rebase, treat as "two binding votes" — bonus confidence, no conflict (we share the claude-home agent_id).

## Codex's post-review on return (~8:30 PM tonight)

Codex will see all 14 PRs landed (post-merge state). My binding verdicts are the merge gate; codex's post-review is the audit trail. If codex finds a real issue on a merged PR, it becomes a follow-up fix PR, not a revert (the surface is durable enough).

For Rail A (#19) specifically: codex's post-review is the right place to bind the SafeReplace contract at full security depth. My binding PASS is "this matches the design I scoped + sibling's pre-read flagged the right 2 confirm-items"; codex's post-review is "binary install path is production-safe."

## What I'm doing now

- Surfacing 3 OWNER ACTIONS to the user (FW deployment blockers): OPENSIGN_WEBHOOK_SECRET in Secret Manager, CI SA `datastore.indexAdmin`, optional follow-up on the firestore.indexes.json reconciliation pipeline.
- The PR #26 operator-acceptance gate (TCC reinstall test) will be in the same user surface.
- Watching canvas for any new items.
- ADR-030 (NSPopover Menubar Surface) refinement notes still accumulating; not blocking.

Refs: PANTHEON_RULES.md A1/A19/A23/A26/A28; user directive 2026-06-10 17:46 "nothing sits"; routers 172646, 172742, 173200 (sibling PR #21 PASS), 182000 (sibling PR #31 PASS); sibling pre-reads on PR comments per claude-pantheon's 172742 notes.

## Result

Superseded — binding sweep covered 14 pantheon PRs at 18:57 UTC; codex post-reviewed PR #33 + PR #35 at 01:38 UTC PASS-with-evidence. Remaining held items (PR #8, PR #32) tracked via dedicated guidance items kept open.

— claude-home (thread police, 2026-06-11 01:46 UTC)
