# Pantheon — Complete Feature Inventory & Engineering Roadmap

> Status as of 2026-06-18. Scope: the local-node surface that **cleans, hydrates, and protects** sessions (gamers, productivity, devs, and the AI agents themselves). **Ra (fleet-tier orchestration) is out of scope** for this document and is omitted from the roadmap below.
>
> This document is written in the wake of a real failure: Pantheon OOM-crashed a 48 GB M5 Max. The "protect" pillar was advertised but **never built** — Hapi is a stub, the Isis watchdog was not running, and `renice` frees zero RAM. The inventory does not cherry-pick. It states what is real, what is a stub, and what was over-claimed.
>
> **UPDATE 2026-06-19:** the "protect" pillar is now real — **Hapi shipped** (`internal/guard/hapi.go`, `sirsi hapi`, ADR-031-A Layer 4, merged in #63): the live memory governor that samples free RAM + per-process RSS and suspends/kills a *governed* runaway before the kernel Jetsams. The Hapi row below is updated accordingly.
>
> **Platform scope (ADR-032 — Mac-first):** every feature targets **Mac only** today and advances through the same surface ladder in this order: **CLI → Menubar → TUI → Mac desktop GUI** (built FROM the menubar). The "CLI state" / "Menubar state" columns below are stages 1–2 of that ladder; TUI and GUI are the subsequent stages for each feature. Windows/Linux are deferred 3–6mo (demand-gated); CI is Mac-only (PR #64).

---

## 1. Feature Inventory

Runtime legend: **live** = a persistent daemon exists and runs · **on-demand** = works only when a command is invoked · **stub** = code scaffold present, no working surface · **missing** = claimed but not implemented.

| Deity / Feature | Glyph | Role (one line) | CLI state | Menubar state | Runtime |
|---|---|---|---|---|---|
| **Horus** | 𓂀 | Workstation lord: daemon health, repo status, code graph, operator dashboard | working (`scan/outline/symbols/context/stats/supervise`) | partial | on-demand |
| **Isis** | 𓁐 | Health & remediation: doctor, auto-fix, watchdog, CPU/RAM monitor | working (`diagnose/isis network/isis fix/relieve`) | partial | on-demand (watchdog code exists, **not running**) |
| **Hapi** | 🌊 | Memory governor: free-RAM + per-process RSS watch, suspend/kill a governed runaway before Jetsam | **working** (`sirsi hapi` status/`watch --govern`/`protect`/`release`; ADR-031-A Layer 4, #63) | next | on-demand (`watch` loop; launchd daemon = next) |
| **Guard** | 🛡️ | Process protection & CPU-pressure relief (renice) | working (`relieve/monitor`) | partial | on-demand |
| **Sekhmet** | ⚡ | Alerts & escalation (watchdog `FormatAlert` only) | none (no standalone command) | none | **stub** |
| **Vitals** | 📊 | Health dashboard / metric streaming / trend | partial (`status/monitor` are snapshots) | none | on-demand |
| **Anubis** | 𓃣 | Hygiene engine: scan, clean, ghosts, dedup, monitor | working (`scan/clean/ghosts/duplicates/monitor`) | **surfaced** (AnubisView, CleanReviewView) | on-demand |
| ├ Jackal (scan engine) | 𓃣 | Parallel rule execution for waste detection | indirect (via `sirsi scan`) | partial | on-demand |
| ├ Ka (ghost hunter) | 𓃣 | Uninstall remnant detection + uninstall | working (`ghosts`; uninstall hidden) | surfaced | on-demand |
| ├ Mirror (dedup) | 𓃣 | Two-stage hash duplicate finder | working (`duplicates/dedup`, dry-run) | partial | on-demand |
| ├ Guard (resource monitor) | 𓃣 | One-shot RAM/CPU/Jetsam snapshot | working (`monitor`) | partial | on-demand |
| └ Cleaner (safety engine) | 𓃣 | Last-line delete-safety path validation | invoked by `clean --confirm` | partial | on-demand |
| **Ma'at** | 𓆄 | QA governance, quality gates, pre-push hooks | working (`audit/scales/heal/pulse`) | none | on-demand |
| ├ Scales (policy enforce) | ⚖️ | Threshold/drift enforcement, verdicts | **stubbed** (`runMaatScales` is a no-op) | none | on-demand |
| **Net** | 𓁯 | Scope assembly, canon alignment, drift detection | working basic (`net status/align`) | none | on-demand |
| ├ Neith (arch drift) | 𓁯 | Architecture-triad validation | subsumed into `net`; no verb | none | **stub** |
| **Stele** | 𓊖 | Append-only hash-chained event ledger | none (written via `Inscribe()`, no query CLI) | none | **live-daemon** (write path) |
| **agentguard** | 🛡️ | Pre-flight resource/context checks for agent work | working (`agent preflight/run-safe`) | none | on-demand |
| **Thoth** | 𓁟 | Session memory, context compression, journal | working (`thoth init/sync/brain/compact/status`) | **surfaced** | on-demand |
| **Seshat** | 𓁆 | Knowledge bridge: ingest/export, Gemini/NotebookLM | partial (`ingest/export/list/auth/mcp`) | surfaced | on-demand |
| **Seba** | 𓇽 | Infra/hardware: topology, fleet, hardware profile, ANE | working (`scan/hardware/profile/compute/fleet/diagram`) | partial | on-demand |
| **Brain** | 🧠 | Neural weights, semantic classification | working pipeline, **model missing** | none | on-demand |
| **Insight** | ✨ 𓂀 | Cross-deity state-of-union, deterministic + Gemma narration | working (`insight`) | **surfaced** | on-demand |
| **Suggest** | 💡 | Context-aware next-action recommendations | working (consumed by CLI) | partial | on-demand |
| **Gemma / Local AI** | 𓂀 | On-device MLX-Gemma 27B 4-bit, MCP server | MCP-only (no `sirsi gemma <prompt>`) | partial | **stub** (cold reload per call, no warm broker) |
| **MCP Server** | 𓁐 | Pantheon tools to Claude Code / IDE | working (`mcp start/config/list-tools`) | none | on-demand |
| **Menubar Surface** | 𓂀 | Local systray dashboard (fyne/SwiftUI) | binary, no subcommands | **surfaced** | **live-daemon** |
| **TUI** | 𓁟 🖥️ | BubbleTea terminal console | working (`tui` / bare `sirsi`) | partial | on-demand |
| **RTK (output filter)** | ⚙️ | ANSI strip / dedupe / truncate tool output | working (`rtk filter/stats`) | none | on-demand |
| **Vault (context sandbox)** | 🔐 | SQLite FTS5 store + BM25 code search | working (`vault store/search/get/...`) | none | on-demand |
| **Osiris (checkpoint)** | 𓁹 | Uncommitted-work / session-drift risk | working (`osiris risk/status`, `risk`) | partial | on-demand |
| **Sight** | 👻 | Launch Services ghost audit | none (folded into `ghosts`) | surfaced (under Anubis) | on-demand |
| **Stealth** | 🤫 | Privacy-aware process observation | none (internal) | none | **stub** |
| **Yield** | 🌚 | Resource throttling / task scheduling | none (internal) | none | **stub** |
| **Ignore** | 🚫 | Scan exclusion patterns | none (internal) | none | **stub** |
| **Scarab** | 🪲 | Fleet/subnet discovery, container audit | indirect (via `seba scan`) | partial | on-demand |
| **Dashboard (web)** | 𓂀 | Local HTTP workstation monitor + code graph | working (`dashboard`, hidden) | none (separate web app) | on-demand |
| **Self-Update** | 🔄 | Re-sign fresh binary over stale CLI (AMFI-safe) | working (`self-update`) | none | on-demand |
| **Notify** | 🔔 | Persistent action/alert history | working (`notifications`) | partial | on-demand |
| **Setup** | ⚙️ | Install/uninstall/Ma'at-gate | working (`setup install/uninstall/maatgate`) | none | on-demand |
| **Profile** | 💻 | Deep hardware profile JSON | working (`seba profile`) | partial | on-demand |
| **Spotlight-Exclude** | 🔍 | mds_stores storm detection + guidance | working (`spotlight-exclude`) | none | on-demand |
| **Horus Supervisor** | 𓂀 | Router/Horus supervise loop | working (`horus supervise`) | none | **stub** (no daemon) |
| **Horus Code Graph** | 📊 | AST symbol extraction, compact outlines | working (`horus scan/outline/symbols`) | none | on-demand |

---

## 2. The Honest Gaps — Real vs Stub vs Over-Claimed

**The protect pillar is the lie, and it is what crashed the machine.**

- **Hapi (compute/memory guard) does not exist.** It is documented as "absorbed into Seba v2.0.0," but Seba only does *detection* — it reads hardware topology, it does not *act*. There is no process, anywhere, that watches free RAM and intervenes before the OS Jetsam-kills or the machine OOMs. `hapi_bridge.go` is a hardware-query bridge with no guard loop. **This is the single most important missing feature, and it was presented as shipped.**
- **The Isis watchdog is code without a process.** `StartWatch` exists. Nothing runs it. There is no LaunchAgent, no daemon, no `KeepAlive`. "Continuous health monitoring" is a function that is never called continuously. On the night of the crash it was, correctly described, **not running**.
- **`renice` is not memory relief.** `sirsi relieve` / Guard renice a process to lower CPU priority. Renicing frees **zero bytes of RAM**. Selling renice as the answer to memory pressure is a category error — it is the wrong tool for the failure mode that actually occurs.
- **Gemma has no warm broker.** The 27B model is reloaded per invocation (~3–5 s cold start) and — critically — **its launch is not gated on available RAM and its resident size is not capped**. An agent firing Gemma while the working set is already high is exactly how 48 GB gets exhausted. The pre-launch RAM gate (#60) lands the first of three needed guards; the other two (hard MLX runtime cap, live self-governance) are not built.
- **Sekhmet is a string formatter, not an alert system.** It only formats watchdog text. No daemon emits alerts, nothing escalates, nothing reaches the menubar.
- **Ma'at Scales is a no-op.** Full YAML policy parsing exists; `runMaatScales()` enforces nothing. Governance is advertised; enforcement returns empty.
- **Vitals has no trend.** `status`/`monitor` are snapshots. There is no health-over-time store, so "trend tracking" and "health score history" do not exist.
- **Stele is write-only to the operator.** The ledger works, but there is no `sirsi stele query/tail/verify`. Nothing reads it back to a human or a surface, so the audit trail is invisible.
- **Brain ships without a brain.** The download/manifest pipeline is complete; **no trained model exists.** Classification falls back to file-extension heuristics. "Semantic classification" is aspirational.
- **Neith / Stealth / Yield / Ignore are internal stubs** with no surface. Honest, but they should not be counted as user features.
- **Menubar ≠ CLI parity.** Most working CLI features (Ma'at, Net, Seba topology, Stele, self-update, notifications filtering, Osiris drill-down, Gemma prompt) have **no menubar surface**. The menubar honestly surfaces Anubis, Thoth, Insight, threads, and Osiris status — and little else.

**What is genuinely real and good:** Anubis hygiene (scan/clean/ghosts/dedup) end-to-end including a real menubar review-and-trash flow; Thoth memory; Insight's deterministic aggregation; Vault; RTK; Self-Update's AMFI-safe re-sign; Osiris risk; the Stele write path; Seba hardware detection. The *clean* and *hydrate* pillars are substantially built. **The *protect* pillar is not.**

---

## 3. Priority 0 — Tonight: Make "Protect" Real (the OOM fix)

The fix is one feature with one job: **never let a session — human or agent — take the machine down.** Build Hapi for real as a live memory guard, and wrap the local-AI broker in three layers of safety.

### 3.1 Hapi made real — the live memory guard

- A genuine daemon (`ai.sirsi.pantheon.hapi`, LaunchAgent, `KeepAlive=true`, `ProcessType=Background`) that polls `host_statistics64` / `vm_stat` + memory-pressure + Jetsam signals every **2–5 s**.
- Thresholds with **real actions** (not renice):
  - **Yellow (free RAM < 20% or pressure=warn):** emit Sekhmet alert → Stele + menubar; refuse new Gemma launches.
  - **Orange (< 12% or pressure=critical):** drop OS page caches where safe; `sirsi clean --safe` of reclaimable caches; signal the Gemma broker to evict the warm model.
  - **Red (< 6% or Jetsam imminent):** **terminate the largest non-protected, non-agent resident process first** (protected set = Claude/Codex/Gemini/Gemma worker PIDs, system processes), with a 1-line Stele audit per kill. RAM relief means freeing pages — eviction and cache-drop, never renice.
- CLI: `sirsi hapi status | guard | watch`, plus `sirsi hapi --install-daemon / --uninstall-daemon`.
- Menubar: a live RAM/pressure row with a colored dot reflecting the guard state.

### 3.2 The 3-layer broker safety contract (so the AI itself can't OOM the box)

1. **Pre-launch RAM gate (#60 — DONE):** before spawning Gemma, check projected resident size against free RAM + headroom; **refuse** if it would cross the yellow line. Return a clear "insufficient memory, free N GB" instead of launching into a crash.
2. **Hard MLX runtime cap (BUILD TONIGHT):** launch the MLX process under an enforced resident ceiling (e.g. via `MLX`/Metal allocator limit + a watchdog that hard-kills the broker if RSS exceeds the cap). The model physically cannot grow past its budget.
3. **Live self-governance under Hapi (BUILD TONIGHT):** the Gemma broker registers with Hapi; on Orange it evicts the warm model, on Red it is the first thing asked to release. The broker is a *managed tenant* of the guard, not an unbounded process.

### P0 verification gate

- Unit tests: threshold math, protected-PID exclusion, eviction ordering, gate refusal logic.
- **Live render (per "verify in browser, not tests"):** reproduce the failure — drive memory toward exhaustion (allocator stress + repeated Gemma launches) and **observe, on the real menubar**, the dot go yellow → orange → red, Gemma launches get refused, the warm model evict, and a non-protected hog get terminated **before** the machine OOMs. Confirm Claude/Codex/Gemma worker PIDs are never the victim. The machine must survive a run that previously crashed it. No green-test claim counts until this is watched happening.

**Owners:** gemma decomposes thresholds + eviction policy table (zero-token draft) → **claude-pantheon** builds the daemon, gate, MLX cap, broker registration → **codex** SME-reviews the kill/eviction safety and protected-set correctness (this is destructive — A1 safety held for real codex) → **claude-home** binding verdict before merge.

---

## 4. Engineering Roadmap — Every Feature to CLI + Menubar Parity

Owner key: **gemma** = zero-token decomposition/draft · **claude-pantheon** = build · **codex** = SME/security review · **claude-home** = binding review. Every phase gate = tests **plus** a live render (menubar/TUI/dashboard observed actually working).

### Phase 1 — Protect persistence & alerting (immediately after P0)
- **Deliverables:** Isis watchdog promoted to a LaunchAgent that runs `StartWatch` continuously and feeds Hapi; **Sekhmet** becomes a real alert daemon (advisory→warning→critical escalation, snooze/suppress) writing every event to Stele; menubar **AlertsView** streaming Sekhmet events; `sirsi sekhmet history`.
- **Gate:** trigger a synthetic pressure event; watch it escalate through three levels and appear live in the menubar AlertsView with a Stele audit row.

### Phase 2 — Vitals + Stele readability (make protection observable)
- **Deliverables:** Vitals daemon streaming metrics to `~/.config/ra/vitals.db`; health-score trend; `sirsi vitals [--streaming]` with graph output; menubar **VitalsView** mini-graphs (RAM pressure, CPU, Jetsam count). `sirsi stele query|tail|verify|export`; menubar live event ticker reading Stele.
- **Gate:** open the menubar, watch a live RAM mini-graph move and the Stele ticker scroll real deity events; `sirsi stele verify` confirms hash-chain integrity.

### Phase 3 — Gemma warm broker (perf without losing safety)
- **Deliverables:** PR#60 follow-on — `mlx_lm.server` warm broker (`sirsi gemma serve`), `HTTPRunner` replacing subprocess-per-call, token streaming via MCP progress, `sirsi gemma <prompt>` CLI, Stele generation events. **Stays inside the Phase-0 cap + Hapi governance** — eviction-on-pressure is mandatory.
- **Gate:** measure warm latency drop; then re-run the P0 stress test with the warm broker resident and confirm Hapi still evicts it under Red and the machine survives.

### Phase 4 — Governance enforcement (Ma'at Scales + Neith)
- **Deliverables:** implement `scales.Enforce()` against scan/ghost/resource findings → pass/warn/fail verdicts → Stele + notifications; `--fix` for approved remediations; `sirsi neith audit` validating architecture triads; menubar Ma'at card (quality score, violations, feather weight).
- **Gate:** seed a policy violation, watch a fail verdict render in the menubar Ma'at card and block the pre-push gate.

### Phase 5 — Anubis hardening & daemonization (the clean pillar to live-tier)
- **Deliverables:** `sirsi anubis daemon` / `sirsi monitor --daemon` LaunchAgent for continuous waste/trash watch; scan-diff persistence + "waste trend" in menubar; Mirror server via `sirsi duplicates --ui`; Ka uninstall flow surfaced in menubar (Ask→DryRun→Confirm→Result inline); **Stele inscriptions on every clean/uninstall/dedup**; full Cleaner protected-path penetration test + symlink resolution.
- **Gate:** run the destructive paths against a sandbox of known-sensitive macOS paths; confirm all are blocked and every block/clean writes a Stele row; watch the Ka uninstall flow complete inline in the menubar.

### Phase 6 — Horus operator surface & supervisor daemon
- **Deliverables:** `horus supervise --install-daemon` (`ai.sirsi.horus.supervisor`); menubar **HorusView** real-time status; code-graph caching to `~/.cache/pantheon/horus-graph.json`; dashboard "Code Structure" tab; file-watcher surface; full operator dashboard (ADR-017/020) with WebSocket live refresh + "Open Dashboard" menubar button.
- **Gate:** edit a source file, watch the dashboard symbol count update live over WebSocket and the menubar HorusView reflect daemon health.

### Phase 7 — Seshat / Brain / Seba completion
- **Deliverables:** Seshat full MCP stdio relay + Safari/Firefox adapters + scheduled ingestion + Stele events; Brain — train + ship the CoreML classifier, wire confidence scoring into Anubis findings, expose download progress in menubar + CLI; Seba — Mermaid→HTML diagrams, passive fleet discovery, Scarab container audit, ANE→Horus symbol pipeline; convenience aliases `sirsi profile`, `sirsi scarab`, `sirsi sight`.
- **Gate:** ingest a real Chrome+Safari source and see it in Seshat list; classify files with the trained Brain and watch confidence scores appear in a live Anubis scan; render a Seba HTML topology diagram in the browser.

### Phase 8 — Surface parity sweep (close the menubar/TUI gap)
- **Deliverables:** menubar parity for every working CLI feature still surfaced-less — Notifications tab + badge, Self-Update row on drift, Setup/Settings UI, Osiris checkpoints drill-down, Insight trend badge, Suggest "What's Next" toasts; TUI tabs for Anubis/Horus/Ma'at + persisted view stack + live Stele feed; Insight extended to Seshat/Seba/Brain/Ma'at signals with `--watch`.
- **Gate:** walk every menubar row and every TUI tab on the live build and confirm each performs its action inline (no terminal kick-out, no dead ends) — the Mole-quality bar.

---

## 5. Release Path — GitHub Releases **and** the Sirsi Product Page

The deliverable is one signed, notarized artifact downloadable from **both** GitHub Releases and the Sirsi product page, installable via Homebrew cask, that upgrades **without FDA/TCC churn**.

### 5.1 Signing & notarization (gated on Apple Developer Program approval — PENDING)
- Land the corrected `scripts/build-dmg.sh` (PR#42): **Developer-ID Application** signing + **hardened runtime**, then `notarytool submit --wait`, then `stapler staple` the DMG (create DMG → sign → notarize → staple, in that order — the old script notarized before creating it).
- Add the 7 CI signing secrets (per `docs/RELEASE_SIGNING.md`): Developer-ID cert + password, App-Store-Connect API key/id/issuer, team id, keychain password.
- Stable bundle id `ai.sirsi.pantheon` so re-installs **do not mint a new TCC identity** (adhoc re-signing is what churns FDA — Developer-ID + notarization is the fix). FDA requested once, at first-run `sirsi setup`, never mid-use.

### 5.2 GitHub Releases
- `git tag vX.Y.Z` → CI builds + signs + notarizes + staples the DMG and the bare `sirsi` CLI tarball → uploads both to the GitHub Release with SHA-256 checksums.
- Rebase + land #32 (native NSPopover menubar), #41 (`sirsi uninstall`), #42 (signing) in that order once the Apple cert exists.

### 5.3 Homebrew cask
- Bump `sirsimaster/homebrew-tools/Casks/sirsi-pantheon.rb` to the new tag + notarized DMG URL + SHA-256, so `brew install --cask sirsimaster/tools/sirsi-pantheon` and `brew upgrade` work cleanly. Cask points at the GitHub Release asset (single source of truth for the binary).

### 5.4 Sirsi product-page wiring
- On the Pantheon product page (sirsi.ai), wire a **Download for macOS** button to the **same** GitHub Release DMG asset (not a separate upload — one artifact, two front doors), with the `brew install --cask` one-liner shown beneath it and the SHA-256 published for verification.
- Per "verify in browser, not tests": after wiring, **render the live product page in a real browser**, click Download, confirm it fetches the notarized DMG, install on a clean machine, and confirm Gatekeeper passes (notarization stapled) and FDA is requested exactly once at first run.

### Release gate
A release ships only when, on a **clean machine**: the cask installs, the product-page button downloads the identical notarized artifact, Gatekeeper accepts it with no quarantine warning, first-run requests FDA once, an upgrade re-installs without re-prompting FDA — and the **P0 memory guard is live and observed catching a stress run**. Protection is not a footnote of the release; it is the precondition for it.
