# ADR Index — Sirsi Pantheon (Architecture Decision Records)

This index tracks **all** architectural decisions for the Sirsi Pantheon ecosystem.

**Total ADRs: 57 (incl. ADR-031-A/B/C sub-decisions)** | **Next available: ADR-057**

---

## Master Registry

| ID | Title | Status | Date |
|----|-------|--------|------|
| [ADR-001](ADR-001-FOUNDING-ARCHITECTURE.md) | Founding Architecture — Go, cobra, agent-controller, module codenames | Accepted | 2026-03-20 |
| [ADR-002](ADR-002-KA-GHOST-DETECTION.md) | Ka Ghost Detection — 5-step algorithm, 17 residual locations, bundle ID matching | Accepted | 2026-03-20 |
| [ADR-003](ADR-003-BUILD-IN-PUBLIC.md) | Build-in-Public as Canonical Process — required release artifacts, transparency rules, dual-audience docs | Accepted | 2026-03-22 |
| [ADR-004](ADR-004-MAAT-QA-GOVERNANCE.md) | Ma'at QA/QC Governance Agent — observe/assess/weigh/report, feather weight scoring, agent prototype | Accepted | 2026-03-23 |
| [ADR-005](ADR-005-PANTHEON-UNIFICATION.md) | Pantheon Unified Platform — all deities as sub-systems, single brand, single install | Accepted | 2026-03-23 |
| [ADR-006](ADR-006-SELF-AWARE-RESOURCE-GOVERNANCE.md) | Self-Aware Resource Governance — Guard module + yield-based resource management | Accepted | 2026-03-23 |
| [ADR-007](ADR-007-UNIFIED-FINDINGS-PORTAL.md) | Unified Findings Portal — Horus as canonical aggregator for deity findings | Accepted | 2026-03-24 |
| [ADR-008](ADR-008-SHARED-FILESYSTEM-INDEX.md) | Shared Filesystem Index — Walk once, query everywhere via Horus manifest cache | Accepted | 2026-03-24 |
| [ADR-009](ADR-009-INJECTABLE-SYSTEM-PROVIDERS.md) | Injectable System Providers — standard interface injection for 99% coverage | Accepted | 2026-03-24 |
| [ADR-010](ADR-010-MENUBAR-APPLICATION.md) | Pantheon Menu Bar Application — native macOS status bar + Finder presence | Accepted | 2026-03-25 |
| [ADR-011](ADR-011-DEITY-ALIGNMENT.md) | Deity Alignment & Context Architecture — canonical scopes for all deities | Accepted | 2026-03-25 |
| [ADR-012](ADR-012-VSCODE-EXTENSION.md) | Pantheon VS Code Extension — always-on Guardian, status bar ankh, Thoth context | Accepted | 2026-03-25 |
| [ADR-013](ADR-013-TILED-CONTEXT-RENDERING.md) | Tiled Context Rendering — GPU-inspired relevance scoring, token budgets, deferred manifest | Accepted | 2026-04-05 |
| [ADR-014](ADR-014-STELE-LEDGER.md) | Stele Ledger — append-only hash-chained event log for all deity communications | Accepted | 2026-04-03 |
| [ADR-015](ADR-015-DEITY-HIERARCHY.md) | Deity Hierarchy — Horus as local workstation lord, Ra as fleet lord | Accepted | 2026-04-24 |
| [ADR-016](ADR-016-TUI-PRIMARY-INTERFACE.md) | TUI as Primary Interface — shared suggest engine, streaming, view stack, persistent state | **Superseded by ADR-018** | 2026-05-06 |
| [ADR-017](ADR-017-RA-HORUS-CTR-HYPERVISOR.md) | Ra/Horus CTR Hypervisor — multi-agent orchestration canon, ownership boundary | Accepted | 2026-05-19 |
| [ADR-018](ADR-018-NATIVE-MAC-APP.md) | Native macOS App + CLI as Pantheon's Interactive Surfaces — TUI sunset, standalone SwiftUI + menubar companion | **Partially In Force — Amended By ADR-020** | 2026-05-21 |
| [ADR-019](ADR-019-KNOWLEDGE-SUBSTRATE.md) | Knowledge Substrate — Thoth/Seba/Understand three-tool split, JSON-as-architectural-code, bidirectional sync, Hedera hypergraph direction | Accepted | 2026-05-26 |
| [ADR-020](ADR-020-INTERACTIVE-SURFACE-REOPENED.md) | Interactive Surface Reopened — Multi-Track Evaluation; closed Hybrid C (TUI first cross-platform, Mac native later) | Accepted (Hybrid C) | 2026-05-29 |
| [ADR-021](ADR-021-DEITIES-NOT-SINGLE-REPO.md) | Deities Must Not Assume Single-Repo — Osiris workstation-scoping; scope sourced from CTR registry, not process cwd | **Proposed** | 2026-05-31 |
| [ADR-022](ADR-022-CTR-OS-TRUTH-LIVENESS.md) | CTR Liveness Is OS Truth, Not Heartbeat Recency — terminal `reaped` state, zombie-aware reaper, idempotent registration | **Accepted** | 2026-06-01 |
| [ADR-023](ADR-023-BINARY-VERSION-CONTRACT.md) | One Build-Version Contract + Local Drift Detection — `internal/version` single source, unified ldflags, `internal/selfupdate` D2/D3 scan, `sirsi doctor` binary-drift finding | **Accepted** | 2026-06-01 |
| [ADR-024](ADR-024-ONE-WATCHER-PER-SURFACE.md) | One Watcher Per Surface — Router-Prescribed Heartbeat — register handshake returns surface's canonical watcher; one inbox (`items/`); idempotent re-arm on OS truth | **Accepted** | 2026-06-01 |
| [ADR-025](ADR-025-THOTH-GATED-EXIT.md) | Thoth-Gated Exit + Resumable Thread Suspend — `suspended` (resumable-but-not-live) carrying memory+plans; `thread suspend`/`resume`; SessionEnd hook; SessionStart reconciliation as the authoritative gate (R3) | **Accepted** | 2026-06-01 |
| [ADR-026](ADR-026-HORUS-OPS-DASHBOARD.md) | Horus Ops-Dashboard — one typed read-model (`router.NodeStatus`) over `GET /api/node-status` + `sirsi router node-status` verb; menubar/TUI read-only projections; realizes ADR-015 "dashboard is Horus" (read companion to the frozen action contract) | **Accepted** (steps 1-3 shipped; codex arch-verify pending; surface chrome 4-5 = claude-pantheon) | 2026-06-02 |
| [ADR-027](ADR-027-ROUTER-MENUBAR-SURFACE.md) | Router Menubar Surface — per-mailbox drill-down + operator override-act (open / Reroute / Mark stale) + ⚡ Caffeinate-router full-revival (hidden until dead); extends Horus — Ops; direct-FS reads, 60s shared tick | **Proposed** (routed to codex-pantheon for arch-verify) | 2026-06-08 |
| [ADR-028](ADR-028-OPTIONAL-SQLITE-LEAN-BUILD.md) | Optional SQLite — `nosqlite` lean build variant — keep full Vault/Seshat/notify by default (15 MB); opt-in `-tags nosqlite` for ~10.6 MB with graceful per-package degradation. The one real remaining size lever (Metal gate measured as non-win) | **Proposed** (design-only; codex final review on return; cross-eyes → non-standin claude) | 2026-06-08 |
| [ADR-029](ADR-029-PER-AGENT-SESSION-WORKTREES.md) | Per-Agent Session Worktrees — every agent session edits source in its own `git worktree` under `.claude/worktrees/<agent>-<session>/`, not the shared root checkout; eliminates the shared-`.git` `core.bare` corruption + cross-branch contamination; shared object store, isolated working tree/index/HEAD; pairs with the SessionStart per-resume thread-mint fix | **Accepted** | 2026-06-09 |
| [ADR-030](ADR-030-NATIVE-MENUBAR-POPOVER.md) | Native macOS Menubar Popover — NSStatusItem + NSPopover + SwiftUI NavigationStack (`macapp/`); persistent panel, drill-in/back, manifest+confirm+result inline; Go stays source-of-truth (Swift reads `latest-scan.json` + shells `sirsi`); supersedes the systray GUI on macOS. Phase 1 (Anubis) + Horus health view shipped | **Accepted** (binding review → codex-pantheon) | 2026-06-10 |
| [ADR-031](ADR-031-LOCAL-MODELS-THROUGH-PANTHEON.md) | Local Models Through Pantheon — Pantheon is the on-device inference broker; local LLMs consumed via three surfaces hitting one resolver + RAM gate (`sirsi-gemma` MCP, `gemma` router worker, `sirsi gemma` CLI #57); consumers never bundle their own runtime; single capability boundary (text-only, no tools/verdicts); A11 local-only choke point; networked broker = future (Nexus location-transparency) | **Accepted** | 2026-06-17 |
| [ADR-031-A](ADR-031-A-NEVER-EXHAUST-THE-HOST.md) | Pantheon Must Never Exhaust the Host — defense-in-depth for spawned compute (RAM gate, governor teeth, hard MLX cap) | **Accepted** | 2026-06-18 |
| [ADR-031-B](ADR-031-B-DYNAMIC-PER-NODE-ENFORCEMENT.md) | Dynamic Per-Node Enforcement — the never-exhaust numbers become functions of the measured node (NodeCapacity) + kernel memory-pressure watcher | **Accepted** | 2026-06-19 |
| [ADR-031-C](ADR-031-C-BROKER-ENFORCEMENT-UNIVERSAL.md) | Broker Enforcement Must Be Universal — no bypass paths; closes a coverage gap where a router-triage daemon and the warm-server's own LaunchAgent both invoked raw `mlx_lm.*` directly, skipping every ADR-031-A/B layer; both fixed + verified live, regression-guard audit recommended | **Accepted** | 2026-07-03 |
| [ADR-032](ADR-032-MAC-FIRST-PLATFORM-ROADMAP.md) | Mac-First Platform Roadmap — CLI → Menubar → TUI → GUI on macOS first; Windows/Linux deferred; CI macOS-only | **Accepted** | 2026-06-19 |
| [ADR-033](ADR-033-REMEDIATION-CATALOG.md) | Remediation Catalog — every finding maps to a real macOS lever, never a monitor; three-outcome law (ACTION / GUIDANCE / INFO); governs the monitor→identify→fix loop | **Accepted** | 2026-06-30 |
| [ADR-034](ADR-034-ORCHESTRATION-BRAIN.md) | Orchestration Brain — tiered (T0 dispatch / T1 triage / T2 execution), pluggable, user-navigable LLM spectrum (Level 0–3); deterministic Tier-0 floor; **surfaces + enforces** the Registry/Wake invariant over the EXISTING wake substrate (does not rebuild it); governs A29 | **Accepted** | 2026-07-02 |
| [ADR-035](ADR-035-RUNAWAY-PROOF-EXECUTION.md) | Runaway-Proof Execution — one fenced dispatch authority (the routerstore; claim is the only door to execution); idempotency precedes autonomy; bounded budgets + breakers ("one red number"); 𓁵 Sekhmet host backstop (Runaway Executor doctor check + `sirsi router quarantine-worker` kill switch); worker re-arm gated on the §2b acceptance bar | **Accepted** | 2026-07-04 |
| [ADR-036](ADR-036-ROUTER-V2-DURABLE-DISPATCH.md) | Router v2 — Durable Dispatch: SQLite store as the ONLY dispatch authority (~/.sirsi/router.db, outside git); one `internal/dispatch` facade behind CLI + MCP; real blocking wait; migration importer + dual-read window; file-write cutover explicitly deferred (owner-gated, end of deprecation window) | **Accepted** | 2026-07-07 |
| [ADR-049](ADR-049-ROUTER-OBSERVER-BOUNDARY.md) | Router Observer Boundary — Transport verbs (move items) are Pantheon's; Observer verbs (show state) stay in sirsi-pantheon but contracts + review belong to sirsi-io; I/O review is Advisory (not blocking) | **Accepted** | 2026-07-31 |

---

## Categories

### Core Architecture
- ADR-001: Founding Architecture
- ADR-005: Pantheon Unified Platform
- ADR-006: Self-Aware Resource Governance
- ADR-007: Unified Findings Portal
- ADR-010: Pantheon Menu Bar Application
- ADR-012: Pantheon VS Code Extension
- ADR-014: Stele Ledger
- ADR-015: Deity Hierarchy
- ADR-016: TUI as Primary Interface *(superseded by ADR-018)*
- ADR-017: Ra/Horus CTR Hypervisor
- ADR-018: Native macOS App + CLI *(v0.22 TUI sunset; partially in force — amended by ADR-020)*
- ADR-019: Knowledge Substrate
- ADR-020: Interactive Surface Reopened — closed Hybrid C (TUI first cross-platform, Mac native later)
- ADR-021: Deities Must Not Assume Single-Repo *(proposed — Osiris workstation-scoping)*
- ADR-022: CTR Liveness Is OS Truth, Not Heartbeat Recency *(accepted — reaped-is-terminal, zombie-aware reaper)*
- ADR-023: One Build-Version Contract + Local Drift Detection *(accepted — single `internal/version`, `sirsi doctor` binary-drift)*
- ADR-024: One Watcher Per Surface — Router-Prescribed Heartbeat *(accepted — register handshake returns the surface's canonical watcher; one inbox; one heartbeat per thread)*
- ADR-025: Thoth-Gated Exit + Resumable Thread Suspend *(accepted — `suspended` resumable-but-not-live, `suspend`/`resume` verbs, SessionEnd hook, SessionStart reconciliation; completes R3)*

### Ghost Detection & Indexing
- ADR-002: Ka Ghost Detection
- ADR-008: Shared Filesystem Index

### Quality & Governance
- ADR-004: Ma'at QA/QC Governance
- ADR-009: Injectable System Providers (Testing Architecture)

### Context Management
- ADR-013: Tiled Context Rendering

### Process
- ADR-003: Build-in-Public as Canonical Process

---

## ADR Numbering History

| Range | Status |
|:------|:-------|
| ADR-001 | Active — Founding Architecture |
| ADR-002 | Active — Ka Ghost Detection |
| ADR-003 | Active — Build-in-Public Process |
| ADR-004 | Active — Ma'at QA/QC Governance |
| ADR-005 | Active — Pantheon Unification |
| ADR-006 | Active — Resource Governance |
| ADR-007 | Active — Unified Findings Portal |
| ADR-008 | Active — Shared Filesystem Index |
| ADR-009 | Active — Injectable System Providers |
| ADR-010 | Active — Menu Bar Application |
| ADR-011 | Active — Deity Alignment |
| ADR-012 | Active — VS Code Extension |
| ADR-013 | Active — Tiled Context Rendering |
| ADR-014 | Active — Stele Ledger |
| ADR-015 | Active — Deity Hierarchy |
| ADR-016 | **Superseded** by ADR-018 — TUI as Primary Interface |
| ADR-017 | Active — Ra/Horus CTR Hypervisor |
| ADR-018 | **Partially In Force — Amended By ADR-020** — Native macOS App + CLI (v0.22 TUI sunset) |
| ADR-019 | Active — Knowledge Substrate |
| ADR-020 | Active — Interactive Surface Reopened (closed Hybrid C) |
| ADR-021 | **Proposed** — Deities Must Not Assume Single-Repo (Osiris Workstation-Scoping) |
| ADR-022 | **Accepted** — CTR Liveness Is OS Truth, Not Heartbeat Recency |
| ADR-023 | **Accepted** — One Build-Version Contract + Local Drift Detection |
| ADR-024 | **Accepted** — One Watcher Per Surface — Router-Prescribed Heartbeat |
| ADR-025 | **Accepted** — Thoth-Gated Exit + Resumable Thread Suspend |
| ADR-026 | Active — Horus Ops-Dashboard (one typed read-model) |
| ADR-027 | Active — Router Menubar Surface (operator inbox + caffeinate) |
| ADR-028 | Active — Optional SQLite (`nosqlite` lean build variant) |
| ADR-029 | Active — Per-Agent Session Worktrees for source edits |
| ADR-030 | Active — Native macOS Menubar Popover (NSStatusItem + NSPopover + SwiftUI) |
| ADR-031 | Active — Local Models Through Pantheon (on-device inference broker) |
| ADR-031-A | **Accepted** — Pantheon Must Never Exhaust the Host (defense-in-depth) |
| ADR-031-B | **Accepted** — Dynamic Per-Node Enforcement (numbers become functions of the node) |
| ADR-031-C | **Accepted** — Broker Enforcement Must Be Universal (no bypass paths — closes a coverage gap, not a design gap) |
| ADR-032 | **Accepted** — Mac-First Platform Roadmap (CLI → Menubar → TUI → GUI) |
| ADR-033 | **Accepted** — Remediation Catalog (real macOS levers; three-outcome law) |
| ADR-034 | **Accepted** — Orchestration Brain (tiered/pluggable/user-navigable; deterministic Tier-0 floor; surfaces+enforces the Registry/Wake invariant over the EXISTING wake substrate; governs A29) |
| ADR-035 | **Accepted** — Runaway-Proof Execution (fenced dispatch authority; idempotency-first stopgaps; Sekhmet Runaway Executor check + quarantine-worker kill switch; worker OFF until the §2b acceptance bar) |
| ADR-036 | **Accepted** — Router v2 Durable Dispatch (store authority, one facade, event wake, migration+dual-read; cutover mechanism shipped behind `SIRSI_ROUTER_STORE_WAKE`, flip is a live-verified deploy step) |
| ADR-037 | **Accepted** — Daemon-Owned Fabric (the ship-complete control plane: Tier-0 daemon + store authority; MCP/CLI/hooks are thin adapters; self-healing via `doctor`; the completion-proof — every conversation-exercised capability must become a shipped, deterministic, test-enforced lever) |
| ADR-038 | **Accepted** — Pantheon Brand (Emerald + Gold) & the Universal Surface (`internal/brand` is the sole palette; `sirsi brand tokens --format css\|swift\|json` derives every non-Go surface; one supervisor report producer + thin renderers put the dashboard in CLI/TUI/menubar/Swift/web; green = Sirsi, green + gold = Pantheon; supersedes A10 v1) |
| ADR-039 | **Accepted** — The Continuous Wake-and-Work Surface (Honest-Gate Autonomy). Owner directive 2026-07-14: "models in effort at all times except when there is an honest user gate." Continuity comes from a redundant self-healing trigger mesh (each tick works + heals + reschedules), not a resident daemon; the local model is always in effort at Tier-0; the loop runs full-auto and stops only per-item at a DETERMINISTIC honest gate (`internal/router/gate.go` `ClassifyGate` — safety/founder/irreversible/escalate, hardcoded like safety.go, model may only ADD gates); rides #203 `brain.AutonomousMode()`. P1 (gate classifier) shipped; P2-P6 decomposed. |
| ADR-040 | **Accepted** — Do No Harm To The Running Host (Load-Bearing Recognition). Owner directive 2026-07-14 after an agent nearly killed the 25.8 GB process that WAS the local-model broker (`sirsi gemma serve`, runs as `Python`). An agent/loop/governor MUST NOT kill or starve load-bearing Pantheon infra while working; the response to an oversized Tier-0 model is RIGHT-SIZE, not kill. Recognition by pidfile not name (`internal/guard.IsLoadBearing` — name-matching misses `Python`); `FindRunaway` skips load-bearing PIDs even as top RSS. Canon: PANTHEON_RULES A32. |
| ADR-041 | **Accepted** — Identity-Enforced Bind, Scoped To Authority-Model Paths. Owner decision 2026-07-15 ("A, scoped as C") after #217 — the PR written to stop authority PRs self-merging — merged carrying its own `binding-hold` label. Root cause is identity: every agent authenticates as the single account `SirsiMaster`, so `required_pull_request_reviews` was `null` and any marker an agent can apply, the author can apply. Bind is now pinned to the one primitive an author cannot forge (GitHub forbids self-approval): authority-model paths (`.github/`, `cmd/sirsi/`, `internal/router/`, `PANTHEON_RULES.md`, `docs/ADR-*`) require an APPROVED review from a non-author login on the current head SHA, recorded by the second identity `sirsi-bind` (App key local-only — a key in Secrets restores the circularity). Ordinary product PRs stay autonomous (scope C). `bound`-label clearing abolished. Canon: PANTHEON_RULES A28 (also amended `enforce_admins` false→true). |
| ADR-042 | **Accepted** — Self-Hosted CI on Sirsi Silicon. Owner directive 2026-07-22 ("the transition from cloud to local hosted frames — why was this ever a question?") after GitHub-hosted macOS runners (10x billing multiplier) exhausted the account allowance and a payment lock took all private-repo CI down mid-investor-week. All pantheon macOS CI targets the self-hosted `m5-sirsi` runner (launchd-durable); local Sirsi-owned frames are the DEFAULT for Sirsi compute, cloud rental the justified exception. Public-repo fork PRs never reach the runner (job-level guards + all-external-contributor approval policy). See docs/ADR-042-SELF-HOSTED-CI-ON-SIRSI-SILICON.md. |
| ADR-043 | **Accepted** — Stray-Thread Supersession Reap (one live watcher per surface). Owner directive 2026-07-22 after the registry accreted 520 records for ~11 live processes (145 `claude-home` records for one live watcher). `ReapDeadThreads` enforced OS-truth on actives but never enforced ADR-024 against duplicate suspends/ghosts, which are reaper-immune by design (ADR-025). New `router.ReapStrayThreads` in the read-time integrity pass sweeps a surface's non-live siblings once a live watcher holds it — ADR-025 preserved (only superseded suspends swept; lonely suspends survive), OS-truth preserved (live PID never reaped), and every reap Thoth-checked (salvageable state inscribed to the Stele before sweep). Canon: PANTHEON_RULES A22/A24/A25/A32. |
| ADR-044 | **Accepted** — `sirsi runner`: Self-Hosted CI as a Product Verb. Extends ADR-042 fleet-wide: the hand-run installer seed becomes `sirsi runner install <repo>` (donor clone with both proven gotchas — `.runner_migrated` strip, `runsvc.sh` staging — gh-fetched token, launchd service, and success gated on the runners API reporting **online**, not on "started") plus `sirsi runner status` grading the whole local fleet live with a published `--json` contract for the board producer. `sirsi setup` offers the install default-yes inside any GitHub repo — builds land on owned silicon by default, cloud rental is the deliberate exception. See docs/ADR-044-SIRSI-RUNNER-PRODUCT-VERB.md. |
| ADR-045 | **Accepted** — Reboot-Durable Gemma Broker. Owner directive 2026-07-23 ("local-llm-sovereignty": the local LLM should survive and reboot everyone else) after the nohup-launched broker died at reboot and again mid-day, each time revived by a CLOUD agent. launchd now owns the bounded broker directly: `sirsi gemma serve --foreground` applies the full ADR-031 bounds then `exec`s the capped server in place (the supervised pid IS the server; pid file stays truthful), and `sirsi setup` installs the `ai.sirsi.gemma-broker` KeepAlive LaunchAgent while retiring the one-shot `ai.sirsi.gemma` launcher. Kill-tested live (36715 → revived 36848). See docs/ADR-045-REBOOT-DURABLE-GEMMA-BROKER.md. |
| ADR-046 | **Accepted (design)** — Local Sovereignty: the T0 Floor and API-Offline Takeover. Owner directive 2026-07-23 ("the local LLM should survive and reboot everyone else… when I'm offline the local LLM should take over ALL LLM work") after the kernel-panic reboot proved the inversion (a cloud agent revived the local broker). Three layers: L0 T0-floor (launchd re-ensure, shipped #286, proven 100s unattended revival), L1 deterministic no-LLM supervision (shipped #287: kickstart duty, heal collector, owner-facing run report, API probe), L2 offline takeover (this design): a 3-strike/2-recovery reachability state machine flips the gemma-worker to consume ALL lanes — mechanical work completes locally, judgment work parks in a draft-attached queue (the local model NEVER binds; A30 amended with the offline clause), clean drain on cloud return. Four phased PRs; acceptance = a 30-min firewall kill test's run-report transcript. See docs/ADR-046-LOCAL-SOVEREIGNTY-T0-FLOOR-AND-OFFLINE-TAKEOVER.md. |
| ADR-047 | (file exists — unindexed; pending index entry) |
| ADR-048 | **Accepted** — MLX Go Bindings: Verdict and Work Path (Stay on Python+mlx_lm; Go bindings not worth the cost at current scale) |
| ADR-049 | **Accepted** — Router Observer Boundary (Transport = Pantheon; Observer contracts = I/O; I/O review = Advisory) |
| ADR-050 | **Proposed for independent review** — Router Universal Task Ledger. Joins durable router items, per-agent task commitments, and thread heartbeat/current-item evidence into one typed model; adds item age/staleness, dependency chains, blocked vs unblocked/unpicked counts, `router ledger`, `router task add\|update\|list`, and CTR summary projection. Owner-directed two-agent quiet-regime work; Claude Nexus reviewed the implementation; Claude Pantheon ratification remains pending. |
| ADR-051 | **Accepted** — Anubis/Ra Product Split: SNE Supervisor Configuration (Pantheon sole authority for SNE profile selection) |
| ADR-052 | **Accepted** — A2A/Router-Conduit Operating Rules — Pantheon Adoption (seven A2A properties; task ledger as authoritative fleet projection) |
| ADR-053 | **Accepted** — Horus as Per-Node Conduit (renumbered from ADR-051 draft; #450). Owner-canon directive: router items + observability = one flow through Horus; one Horus per node; Anubis = SNE + local Horus; Ra = aggregator of Horus ConduitReports. `internal/horus/conduit.go` implements `NodeConduit` + `ConduitReport`. |
| ADR-054 | Reserved — PR #451 (owner-gated): SNE Licensing / Anubis-Ra seam |
| ADR-055 | **Accepted** — Extract Thoth as Standalone Package (formerly mislabeled ADR-016) |
| ADR-056 | **Proposed** — Go Owns the Serving Path; Python becomes a called extension (formerly mislabeled ADR-046) |
| ADR-057 | Next available |

> **Last updated:** July 7, 2026 — indexed ADR-036 (Router v2 Durable Dispatch, accepted 2026-07-07; Phases 1-4 shipped #144/#164/#168/#174-era; cutover deliberately deferred to an owner-gated step); next available advanced to ADR-037. Earlier — July 4, 2026 — indexed ADR-035 (Runaway-Proof Execution, accepted 2026-07-04; canonizes the codex-APPROVED Phase-2 Dispatch Contract axioms at the architecture level and adds Sekhmet's independent host backstop — the "Runaway Executor" doctor finding + `sirsi router quarantine-worker`; provenance: the 2026-07-03/04 runaway-executor incident, case study `docs/case-studies/2026-07-04-runaway-executor.md`); next available advanced to ADR-036. Earlier — July 3, 2026 — indexed ADR-031-C (Broker Enforcement Must Be Universal, accepted 2026-07-03; a router-triage daemon and the warm-server's own LaunchAgent both bypassed the ADR-031-A/B broker by invoking `mlx_lm.*` directly — neither layer failed, neither was in the call path; both fixed and verified live; case study `docs/case-studies/2026-06-18-pantheon-did-not-prevent-oom.md` §6 updated same commit). Earlier — July 2, 2026 — indexed ADR-034 (Orchestration Brain, accepted 2026-07-02; governs PANTHEON_RULES A29; codifies + surfaces the existing `internal/router` wake substrate rather than rebuilding it); next available advanced to ADR-035. Earlier — July 1, 2026 — version-truth sweep: indexed ADR-033 (Remediation Catalog, accepted 2026-06-30, PR #125); next available advanced to ADR-034. Earlier — June 30, 2026: registry reconciled with disk (full-repo audit): added the previously-unregistered ADR-026 through ADR-032 (Horus ops-dashboard, router menubar, optional-SQLite, per-agent worktrees, native menubar popover, local-models-through-Pantheon + ADR-031-A/B resource governance, Mac-first roadmap); corrected the count and next-available pointer (ADR-032 was already taken on disk). Earlier milestone — ADR-023 **accepted**: one build-version contract (`internal/version`) replaces seven scattered `var version` literals; ldflags unified across all binaries; `internal/selfupdate` detects sibling (D2) and PATH (D3) drift locally with no network; `sirsi doctor` emits a `binary-drift` finding (`docs/ADR-023-BINARY-VERSION-CONTRACT.md`).
