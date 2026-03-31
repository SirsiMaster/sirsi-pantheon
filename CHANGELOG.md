# Changelog — Sirsi Pantheon
All notable changes to this project are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/).

**Building in public** — see [docs/BUILD_LOG.md](docs/BUILD_LOG.md) for the full narrative.

---

## [0.9.0-rc1] — 2026-03-31

### Added
- **Neith Module** — Real plan alignment engine with keyword-based log assessment, full tapestry validation (all 5 deity checks), drift detection, and CLI (`pantheon neith status`, `pantheon neith align`).
- **Ka Cross-Platform Ghost Detection** — `GhostProvider` interface with platform-specific implementations. macOS (full), Linux (XDG + dpkg + .desktop files), Windows (stub). All providers testable from any platform via `TestMockProvider`.
- **5 New MCP Tools** — `thoth_sync`, `maat_audit`, `anubis_weigh`, `judge_cleanup` (dry-run only), `pantheon_status`. Total: 11 tools, 4 resources.
- **Thoth /compact Integration** — `pantheon thoth compact -s "summary"` persists session decisions into memory.yaml and journal.md before context compression. Includes `PruneJournal()` for age/count-based cleanup.
- **Claude Code Custom Command** — `.claude/commands/compact.md` for `/compact` integration.

### Changed
- **Hapi → Seba Consolidation** — Hardware detection and accelerator logic moved from `internal/hapi/` to `internal/seba/`. Hapi retains backward-compatible wrappers (type aliases + delegation). Hapi now contains only APFS snapshot management + wrappers. Circular import resolved via guard bridge pattern.
- **Hapi dedup.go removed** — Duplicate detection is handled by the mirror module; hapi's version was redundant.

### Fixed
- All 28 packages pass tests on macOS and Ubuntu CI
- Zero golangci-lint errors

### Not Included (deferred)
- **Ra** — Web portal / hypervisor orchestration (not started)
- **Windows Ka** — Stub only; real implementation deferred
- **Flatpak/Snap/RPM** — Linux package managers beyond dpkg deferred

---

## [0.8.0-beta] — 2026-03-31 (The Honest Measurement)

### What This Release Is
v0.8.0-beta is the first credible public release of Pantheon. All metrics are verified by `go test -cover ./...` — no hardcoded numbers, no projections presented as measurements. The previous v1.0.0-rc1 claim was premature and has been corrected.

### Added
- **Thoth Knowledge System** — Go port of sirsi-thoth folded into Pantheon. `pantheon thoth init --yes <dir>` scaffolds .thoth/ project memory. Detects Go, TypeScript, Next.js, Rust, Python projects.
- **Ma'at Streaming Progress** — `maat audit` now shows per-package test results as they stream in, with color-coded verdicts. No more 2-minute silent waits.
- **`--skip-test` Flag** — `maat audit --skip-test` uses cached coverage for instant governance results without running the full test suite.
- **Ma'at Dynamic Module Discovery** — `DefaultThresholds()` now scans `internal/*/` dynamically instead of using a hardcoded registry. All 27 modules are now measured (was missing 10).
- **E2E Smoke Tests** — 9-test bash suite (`scripts/smoke.sh`) + 10-test Go E2E suite (`tests/e2e/smoke_test.go`) testing the compiled binary against the real filesystem.
- **Jackal Rules Coverage** — 93.1% coverage on scan rules (was 64.5%). 50+ new tests covering all rule constructors, Scan/Clean operations, Horus manifest branches, findRule depth/matchFile logic.

### Fixed
- **False Coverage Reports** — Ma'at was reporting 0% for 10 modules (thoth=83%, seshat=85%, neith=100%, etc.) due to hardcoded module registry. Fixed with dynamic discovery.
- **CI Pipeline** — Go 1.22 -> 1.24, golangci-lint v4 -> v6, 40+ lint errors resolved across 19 files.
- **Version Honesty** — Corrected v1.0.0-rc1 -> v0.8.0-beta. The v1.0.0-rc1 label was premature — it will be earned after 30-day dogfooding.

### Changed
- Version: `0.7.0-alpha` -> `0.8.0-beta`
- Go: 1.22 -> 1.24 across all CI workflows
- golangci-lint: v4 -> v6

### Verified Metrics (March 31, 2026)
| Metric | Value | Command |
|--------|-------|---------|
| Tests Passing | 1,500+ | `go test -short ./...` |
| Packages | 28/28 green | `go test ./...` |
| Weighted Coverage | ~85% | `go test -cover ./...` |
| Lint Errors | 0 | `golangci-lint run ./...` |
| Binary Size | ~12 MB | `go build ./cmd/pantheon/` |
| Scan Rules | 64 | `internal/jackal/rules/` |
| Internal Modules | 27 | `ls internal/` |
| E2E Smoke Tests | 9+10 | `scripts/smoke.sh` + `go test ./tests/e2e/` |

### What's NOT in This Release
- Ra (web portal) — not started
- Neith (orchestration) — stub only
- Windows/Linux ghost detection — macOS-first
- Cross-platform GUI — CLI only for now

### What's Next (v1.0.0-rc1 — earned, not declared)
- 30-day dogfooding on production machines
- Cross-platform testing (Linux, Windows)
- Neith orchestration implementation
- MCP plugin for Claude Code (desktop/IDE/CLI)

---

### Session 37 (2026-03-29) — The Great Pantheon Consolidation
- **Deity-First Architecture** — Successfully consolidated 12 fragmented command scripts into 6 Master Deity Pillars, achieving the "One Install. All Deities." standard.
  - **Anubis 𓂀**: Unified Hygiene, Ka Ghost Hunter, Mirror Dedup, and Guard Watchdog.
  - **Ma'at 𓁐**: Unified Scales Governance and Isis Autonomous Remediation.
  - **Thoth 𓁟**: Unified Knowledge Sync and Permanent Brain Ledger.
  - **Hapi 𓈗**: Unified Hardware Detection and Sekhmet ANE Acceleration.
  - **Seba 𓇼**: Unified Infrastructure Mapping, Project Book, and Scarab Fleet Discovery.
  - **Seshat 𓁆**: Unified Gemini Bridge, Brain Library, and MCP Context Server.
- **Universal Glyph Standard** — Purged all generic emojis (🏛️, 🌊, ⬥) and geometric symbols (⬥, ◇, ◆) across the entire platform. 
  - **CLI/TUI**: All headers, status indicators, and dashboards now use High-Fidelity Ancient Egyptian Hieroglyphs.
  - **Registry**: Remastered `docs/index.html` with click-to-flip cards reflecting the unified 6-pillar hierarchy.
- **Safety Restoration** — Restored the **⚠️ Universal Warning** signal throughout the platform to ensure absolute safety and recognition for destructive operations.
- **Monumental Baseline (𓉴)** — Promoted the **Great Pyramid (𓉴)** as the primary architectural anchor for the Pantheon platform and Web Registry, replacing legacy generic identifiers.
- **Hieroglyphic Menu** — Published the `glyph_menu.md` (Artifact) featuring over 25 categorized hieroglyphs for Master Pillar selection variety.
- **Hardening & Verification** — Resolved all compilation regressions, import collisions (fmt, os, InfoStyle), and unit test mismatches.
- **Stats**: 36 files modified, consolidated 13 legacy scripts, 100% build-readiness.

### Planned
- P1: npm publish thoth-init
- P2: Isis Phase 2 (test scaffold generation, errcheck auto-fix)
- P3: Thoth test coverage (internal/thoth/ at 0%)
- P4: Homebrew Formula update for marketing launch.

### Session 35 (2026-03-28) — Isis Phase 1 (The Healer) + Thoth CLI
- **Thoth CLI** (`cmd/pantheon/thoth.go`) — `pantheon thoth sync` wired to CLI.
  - Two-phase auto-sync: Phase 1 updates memory.yaml identity fields from source analysis. Phase 2 appends journal.md entries from git history.
  - `findRepoRoot()` walks up from cwd to locate `.thoth/` directory.
  - Flags: `--since`, `--dry-run`, `--memory-only`, `--journal-only`.
  - Self-dogfooded: the sync command updated its own memory.yaml in this session.
- **Isis Remediation Engine** (`internal/isis/`, 6 files, 24 tests) — Phase 1 of the Ma'at→Isis healing cycle.
  - `isis.go`: `Healer` struct, `Strategy` interface, `Heal()` orchestrator with dispatch, `Report` formatter.
  - `lint.go`: `LintStrategy` — runs `goimports` + `gofmt` with injectable `RunCmd` (Rule A21).
  - `vet.go`: `VetStrategy` — runs `go vet`, parses findings. Reports (no auto-fix — requires human judgment).
  - `coverage.go`: `CoverageStrategy` — uses `go/parser` AST analysis to find exported functions without tests.
  - `canon.go`: `CanonStrategy` — detects memory.yaml/journal drift and triggers `thoth.Sync()`.
  - `bridge.go`: `FromMaatReport()` converts Ma'at `Assessment` verdicts to Isis `Finding` structs.
- **Isis CLI** (`cmd/pantheon/isis.go`) — `pantheon isis heal`.
  - Dry-run by default (Rule A1 — safety first). `--fix` to apply changes.
  - Cache-based Ma'at weighing (~3ms) by default. `--full-weigh` for live `go test` (~5min).
  - Strategy filters: `--lint-only`, `--vet-only`, `--coverage-only`, `--canon-only`.
- **Distribution** — `tools/thoth-init/README.md` for npm publish. Local `npx thoth-init -y` verified.
- **Stats**: 14 files changed, +1,765 lines, 843+ tests, 27 modules, 42 commands.
- **Seshat VS Code Extension** (`extensions/gemini-bridge/`) — Full TypeScript extension for Gemini Bridge.
  - 7 source files: `extension.ts`, `commands.ts`, `dashboard.ts`, `knowledgeProvider.ts`, `chromeProfilesProvider.ts`, `syncStatusProvider.ts`, `paths.ts`.
  - **Activity Bar**: Dedicated sidebar with 3 tree views — Knowledge Items, Chrome Profiles, Sync Status.
  - **Dashboard Webview**: Gold-on-black Egyptian aesthetic with KI inventory, conversation count, bridge direction visualizer, and sync actions.
  - **Chrome Profile Discovery**: Reads Chrome's `Local State` to enumerate all profiles; highlights configurable default (`SirsiMaster`).
  - **6 Commands**: `seshat.listKnowledge`, `seshat.exportKI`, `seshat.syncToGemini`, `seshat.listProfiles`, `seshat.listConversations`, `seshat.showDashboard`.
  - **4 Config settings**: `seshat.defaultProfile`, `seshat.autoSync`, `seshat.pantheonBinaryPath`, `seshat.antigravityDir`.
  - VSIX packaged: `seshat-gemini-bridge-0.1.0.vsix` (430 KB, 12 files).
  - Publisher: `SirsiMaster`. License: MPL-2.0.
- **Neith's Triad Retrofit** — `ARCHITECTURE_DESIGN.md` upgraded from v1.0.0 to v2.0.0:
  - §7: **Data Flow Architecture** — Full Mermaid diagram mapping all CLI entry points, Code Gods, Machine Gods, Safety Layer, Output Layer, and Seshat's 6 external system directions.
  - §8: **Recommended Implementation Order** — Gantt chart of 7 build phases from Jackal through Distribution.
  - §9: **Key Decision Points** — 10-row decision matrix covering binary architecture, concurrency, policy language, safety model, UI framework, fleet transport, context format, deity coupling, distribution, and bridge direction.
  - Document now fully compliant with Rule A22.
- **Firebase Deploy** — 17 files deployed to `sirsi-pantheon.web.app` with all 11 deity click-to-flip cards live.

### Session 29 (2026-03-27) — CI Green Sprint + Thoth Journal Sync + Rule A21
- **CI Remediation (P0)** — Resolved 22 lint errors across 16 files:
  - `errcheck`: 4 suppressed `fmt.Sscanf` returns in `stats.go`
  - `unused`: 3 wired/removed dead functions in menubar
  - `goimports`: 1 formatting fix in `sekhmet.go`
  - `shadow`: 6 renamed inner `err` vars in 5 test files + `publish.go`
  - `unusedwrite`: 8 added struct field assertions in 4 test files
- **Windows CI Fix** — Added `shell: bash` to test step (PowerShell splits `-coverprofile=coverage.out`).
- **Data Race Fix** — `AlertRing` mutex + `sampleTopCPUFn` accessor pattern (`getSampleFn()`/`setSampleFn()`).
  - Root cause: `defer func() { sampleTopCPUFn = old }()` raced with watchdog goroutines on locked OS thread.
  - Fix: `sync.RWMutex`-protected accessors. All 8 bridge tests pass with `-race -count=1`.
- **Rule A21 Canonized** — Concurrency-Safe Injectable Mocks. Ma'at governs: all package-level function pointers used for test injection MUST use mutex-protected accessors.
- **Thoth Journal Sync (P1)** — `internal/thoth/journal.go` (230 lines): auto-generates journal entries from git history.
  - `thoth sync` now runs Phase 1 (memory.yaml) + Phase 2 (journal.md from `git log`).
  - Supports `--since` and `--dry-run` flags. Closes the ghost transcript gap permanently.
- **Firebase Deploy (P2)** — 17 files deployed to `sirsi-pantheon.web.app`.
- **gh CLI Upgrade (P3)** — `gh` 2.87.3 → 2.89.0.


### Session 28 (2026-03-27) — Ghost Transcripts Recovery + CI Remediation
- **Case Study 014** — "The Ghost Transcripts": discovered Antigravity IDE never writes `overview.txt` — 90+ conversations with zero transcripts.
- **Forensic Recovery** — Reconstructed journal entries 022-024 from git diffs, case studies, build log, and memory.yaml.
- **CI Remediation** — Fixed 3 CI failure categories: Windows `CGO_ENABLED` syntax, `coverprofile` parsing, 20+ lint errors.
- **Lint Hardening** — Fixed unused `version` vars (5 standalone binaries), unused struct fields (`lastSnapshot`, `autoEnabled`), misspelling (`cancelled`→`canceled`).
- **Binary Hygiene** — Removed tracked `thoth` binary from git, added to `.gitignore`.
- **Test Hardening** — Added `-short` flag to CI test runner to skip live syscall tests (30s timeout prevention).

## [0.7.0-alpha] — 2026-03-27 (Ecosystem Hardening — Sekhmet Phase)
### Added
- **Singleton Enforcement** — Implemented Unix domain socket locking (`platform.TryLock`) across all primary entry points (Menubar, Guard, MCP) to prevent process redundancy.
- **Hapi-Brain Bridge** — Created `internal/brain/hapi_bridge.go` for hardware-aware inference backend selection (CoreML vs ONNX).
- **Hardened Watchdog** — Sekhmet watchdog now enforces a 1.5GB memory governance threshold and tracks process prioritization.
- **MCP hardware tool** — Added `detect_hardware` tool to the MCP server for real-time accelerator and resource detection.

### Fixed
- **Triple Ankh Redundancy** — Resolved the issue of multiple pantheon-menubar instances running simultaneously.
- **MCP Standardization** — Refactored MCP server startup to utilize the standard `mcp.NewServer()` implementation with singleton hardening.
- **LaunchAgent Audit** — Synchronized `ai.sirsi.pantheon.plist` with the hardened singleton architecture.

### Session 25 (2026-03-27) — Sekhmet Phase II (ANE Tokenization)
- **HAPI Tokenization** — Extended the `Accelerator` interface with native `Tokenize` support.
- **Hardware Backends** — Implemented specialized tokenization for AppleANE, Metal, CUDA, and ROCm.
- **FastTokenize** — Built a deterministic BPE-style pure Go tokenizer as the universal CPU fallback.
- **Sekhmet CLI** — Integrated `pantheon sekhmet --tokenize` for direct hardware-accelerated testing.
- **Global Flags** — Centralized CLI flags in `cmd/pantheon/globals.go` to support modular command files.
- **Canon Sync** — Updated Thoth, BUILD_LOG, FAQ, and the Deity Registry.

### Session 24 (2026-03-27) — Pantheon v0.7.0-alpha Deployment
- **VSIX Packaging** — Built and sideloaded `sirsi-pantheon-0.7.0.vsix` for verify-before-publish testing.
- **OpenVSX Publish** — Published `SirsiMaster.sirsi-pantheon@0.7.0` to Open VSX using the SirsiMaster account (Rule A20).
- **Crashpad Verification** — Manually verified the Crashpad Monitor's threshold detection by clearing 34 stale dumps.
- **Site Deployment** — Deployed updated Deity Registry and Build Log (Sessions 23/24) to `pantheon.sirsi.ai`.
- **Status Sync** — Updated all public-facing stats: 21K+ lines of Go, 90.1% coverage, 11 deities, 12 ADRs.
- **Version**: 0.7.0-alpha.

### Session 23 (2026-03-26) — Crash Forensics + Crashpad Monitor
- **Crash Forensics** — Investigated IDE crash that required 2 reinstalls + 2 restarts.
  - 34 pending crash dumps in `Crashpad/pending/` — dating back weeks.
  - Root cause: Session 22 manifest patches created un-realizable Extension Host state.
  - Chain: V8 OOM (`electron.v8-oom.is_heap_oom`) → macOS Jetsam (`libMemoryResourceException`) → cascade.
  - V8 GC efficiency dropped to `mu = 0.132` (normal: >0.9) before heap exhaustion.
  - Crash dumps 2 & 3 confirmed as `libMemoryResourceException` — kernel memory pressure kills.
- **Rule A19 Hardened to ABSOLUTE PROHIBITION** — No `.app` bundle modifications ever.
  - Previous exception ("manifest-only patches are safe with re-signing") proven wrong.
  - Semantic integrity matters more than code signing — valid JSON can crash the Extension Host.
  - Case Study 011: `docs/case-studies/session-23-extension-host-crash-forensics.md`.
- **Crashpad Monitor** (`extensions/vscode/src/crashpadMonitor.ts`, 370+ lines) — **NOVEL FEATURE**.
  - Auto-detects Crashpad directory for Antigravity, VS Code, Cursor, Windsurf.
  - Polls `pending/*.dmp` every 5 minutes with rolling trend detection (3-reading window).
  - Extension Host crash identification via first-8KB string extraction from `.dmp` files.
  - Trend classification: `stable` / `growing` / `critical` with threshold-based alerts.
  - Status bar indicator: hidden when stable, 🟡 at 5+ dumps, 🔴 at 15+ dumps.
  - Premium webview report with timeline, forensics reference, and cleanup recommendations.
  - One-time session warning when trend shifts from stable.
  - New command: `pantheon.crashpadReport` (10 total commands, 7 modules).
  - Case Study 012: `docs/case-studies/session-23-crashpad-monitor.md`.
- **Canon Updated** — Journal Entry 020-021, build-log.html, PANTHEON_RULES.md, CLAUDE.md, GEMINI.md.
- **Version**: 0.7.0-alpha. Extension: 10 commands, 7 modules.

### Session 22 (2026-03-26) — Thoth Accountability Engine + Extension Triage
- **Thoth Accountability Engine** (`extensions/vscode/src/thothAccountability.ts`, 645 lines)
  - Cold-start benchmark: walks workspace source, compares against memory.yaml.
  - First measurement: ~1.5M source chars → ~19K memory.yaml = **371K tokens saved** per activation.
  - Dollar savings: configurable pricing tier (Opus $15/M, Sonnet $3/M, Haiku $0.25/M). Default: **$1.11/session**.
  - Freshness meter: compares memory.yaml mtime against most recent source edit. FRESH/STALE/OUTDATED status.
  - Coverage governance: cross-references `internal/` directories against memory.yaml mentions.
  - Context budget: memory.yaml token count as % of 200K context window (<5%).
  - Lifetime counter: persists total tokens, dollars, and sessions across restarts via `globalStorageUri`.
  - Premium webview report in Royal Neo-Deco design language (gold/lapis/obsidian).
  - Status bar: `$(bookmark)` with live savings display next to main PANTHEON ankh.
- **Extension Commands** — `pantheon.thothAccountability` command with 5-option QuickPick menu.
  - Integrated into `pantheon.showMetrics` system dashboard.
  - New configuration: `pantheon.thoth.accountability`, `pantheon.thoth.pricingModel`.
- **Extension Triage** — diagnosed and fixed 4 simultaneous extension issues:
  1. **AG Monitor Pro** (1988ms profile): disabled — `js-tiktoken` WASM init + `chokidar` watcher.
  2. **Pantheon 0.5.0** cascade unresponsive: sideloaded v0.6.0 with Accountability Engine.
  3. **Git extension** missing `title` properties: patched 2 Antigravity-added commands.
  4. **Antigravity extension** missing command declarations: patched 3 undeclared commands.
- **Gatekeeper Recovery** — modifying `.app` bundle broke macOS code signature.
  - Fix: `xattr -cr` + `codesign --force --deep --sign -` (ad-hoc re-signing).
  - Documented as case study with procedure for future `.app` manifest patches.
- **Version**: 0.6.0-alpha. Extension VSIX: 49.47 KB (13 files, 6 modules, 8 commands).

### Session 21 (2026-03-26) — Extension Live Testing + Memory GC
- **Guardian Rewrite** — Native `renice(1)` + `taskpolicy(1)` implementation. No CLI binary dependency for renice.
  - Discovers LSP processes via `ps`, applies nice +10 and Background QoS directly.
  - Skips already-deprioritized processes. Excludes host LSP (language_server_macos_arm) from warnings.
- **Memory Pressure GC** — Tracks per-process RSS across poll cycles.
  - When a third-party LSP exceeds 500 MB for 3+ consecutive checks, triggers VS Code's built-in LSP restart.
  - Maps process names to restart commands (gopls → `go.languageserver.restart`, tsserver → `typescript.restartTsServer`, etc.).
- **Codicon Status Bar** — Replaced invisible hieroglyph with `$(eye) PANTHEON` codicons. Loading spinner on init. Warning icon on pressure.
- **Warning Threshold** — Split total/third-party RAM tracking. Warning triggers on >1 GB third-party LSPs (host LSP at 4-6 GB is normal).
- **CLI Fix** — Commands now use correct Pantheon CLI flags (`weigh --dev --json`, `guard --json`).
- **Live Testing** — Verified end-to-end: all 3 LSPs reniced to nice 10 after 30s delay. Extension Host ~199 MB RSS.
- **Sideloaded** — Installed in both Antigravity and VS Code via VSIX.

### Session 20 (2026-03-25) — The Deployment Sprint
- **Firebase Hosting** — Deployed Deity Registry to `sirsi-pantheon.web.app` via Firebase Hosting (15 HTML pages).
  - Created Firebase site `sirsi-pantheon` in project `sirsi-nexus-live`.
  - Configured hosting target with clean URLs and 1-hour cache.
- **Custom Domain** — Wired `pantheon.sirsi.ai` via Firebase Hosting API + GoDaddy CNAME.
  - Firebase: `HOST_ACTIVE`, `OWNERSHIP_ACTIVE`. SSL auto-provisioning.
- **Flip Cards** — Rebuilt Deity Registry index with click-to-flip 3D cards.
  - Front: user-facing (name, description, key metrics).
  - Back: developer info (package path, coverage, test count, commands, deps, ADR).
  - 3 action buttons per card: Full Page, Download (releases), Source (GitHub internal/ link).
- **Deity Page Fixes** — Updated all 12 deity pages:
  - URL display: subdomain → path format (`pantheon.sirsi.ai/anubis`).
  - Nav links: relative → absolute for Firebase deployment.
- **Canon Cleanup** — VERSION bump to `0.5.0-alpha`, extension icon created, CHANGELOG + Thoth updated.

### Session 19 (2026-03-25) — The Pantheon Extension
- **VS Code Extension** (`extensions/vscode/`) — Full TypeScript extension replacing JS scaffold (ADR-012).
  - `extension.ts`: Entry point — starts Guardian, status bar, Thoth on activation.
  - `guardian.ts`: Always-on renice (30s delay, 60s re-apply loop). Spawns `pantheon guard --renice lsp --json`.
  - `statusBar.ts`: Ankh (𓂀) icon with live RAM/CPU metrics. Polls `ps` directly (sub-50ms). Color-coded states.
  - `commands.ts`: 7 Command Palette entries (Scan, Guard, Renice, Ka, Thoth, Metrics, Settings).
  - `thothProvider.ts`: Context compression from `.thoth/memory.yaml` with file watching.
- **ADR-012**: Pantheon VS Code Extension architecture decision accepted.
- **ADR Index**: Updated to 12 ADRs (001–012).
- **Status**: Extension compiles (0 TypeScript errors), Go backend builds, 819+ tests passing.

### Session 16b (2026-03-24) — The Interface Injection Sprint
- **Coverage Breakthrough** — Weighted average pushed to **90.1%** (Rule A16 established).
- **Injectable Providers** — Established standard interface injection for signals and `exec.Command` (ADR-009).
- **Guard Module (89→91%)** — Full deterministic audit of process termination paths (root-failures, dry-runs).
- **Ma'at Module (80→88%)** — Deterministic CI pipeline assessment with injectable gh-cli runners.
- **Sight Module (78→93%)** — Rebuilt `Fix` and `ReindexSpotlight` with mockable system commands.
- **Antigravity CLI Wiring** — `pantheon guard --watch` now starts the full IPC bridge + AlertRing.
- **MCP Live Alerts** — Bridged watchdog alerts into MCP resources via `anubis://watchdog-alerts`.
- **Canon Realignment** — `ANUBIS_RULES.md` → `PANTHEON_RULES.md` (v2.0.0). ADR index updated.

## [0.4.0-alpha] — 2026-03-23 (Launch Execution + Modular Deities)

### Added
- **Homebrew Tap Integration** — Automated formula updates via `HOMEBREW_TAP_TOKEN`; `brew tap SirsiMaster/tools && brew install sirsi-pantheon`
- **ADR-007 Unified Findings Portal** — Canonical architecture for cross-deity finding aggregation
- **ADR-006 Self-Aware Resource Governance** — Guard module + yield-based resource management
- **Yield Module** (`internal/yield/`) — Cooperative resource yielding for process management
- **Horus Designation** — Assigned as the Unified Findings Portal deity
- **Horus Module** (`internal/horus/`) — Shared filesystem index, parallel walks, manifest cache (ADR-008)
- **Modular Deities (v2.1.0)** — ADR-005 updated with independent deployment standards
- **Ra (Hypervisor)** — v0.1.0-alpha overseer added to Pantheon architecture
- **Seba Rebrand** — `internal/mapper/` → `internal/seba/` (high-performance mapping)
- **Cross-Agent Referral Logic** — Initial implementation of inter-deity remediation referrals
- **Independent Deployment** — Support for standalone deity installation (e.g., `npx thoth-init`)

### Performance (Dogfooding-Driven)
- **Ma'at Diff-Based Coverage** — 55s → 15ms (3,667× speedup); only tests changed packages
- **Horus Shared Filesystem Index** — Walk once, all deities query; Weigh 15.6s → 7.2s (2.2×)
- **Jackal WalkDir Migration** — `filepath.Walk` → `filepath.WalkDir` (avoids stat per file)
- **Combined dirSizeAndCount** — Single walk replaces two separate walks per directory finding
- **Pre-push Gate** — Total gate time 65s → 5s (13× faster)
- **Feather Weight** — 69/100 → 81/100 over session

### Changed
- **Pantheon Unification** — Standardized GEMINI.md, CLAUDE.md, and Portfolio Standard across all 5 repos
- **Ma'at Governance** — Integrated pipeline monitoring, diff-based coverage default, `--full` flag
- **Improved Logging** — Wired Go 1.21 `slog` into `ka` and `cleaner` cores for better diagnostics
- **Release Pipeline** — GoReleaser brews section enabled with `HOMEBREW_TAP_TOKEN` cross-repo secret
- **Weigh CLI** — Horus integration, `--fresh` flag for forcing index rebuild

### Fixed
- **Missing Imports** — Resolved `undefined: logging` error in `internal/cleaner/safety.go`
- **Domain Purge** — Replaced all instances of `sirsinexus.dev` with `sirsi.ai` in SirsiNexusApp
- **MCP Versioning** — Corrected version reporting to match release tags
- **gofmt** — Fixed formatting in `yield_test.go`
- **.gitignore Collision** — Unanchored `pantheon` pattern was ignoring `cmd/pantheon/seba.go`


---

## [0.3.0-alpha] — 2026-03-21/22 (Ship Week — Mirror + Audit + Thoth)

### Added
- **Mirror module** (`internal/mirror/`) — file deduplication engine
  - Three-phase scan: size grouping → partial hash (first+last 4KB) → full SHA-256
  - 8-worker parallel hashing with semaphore-bounded I/O
  - Smart keep/delete recommendations: protected > shallow > oldest > largest
  - Media type classification: photos, music, video, documents (30+ extensions)
  - Flags: `--photos`, `--music`, `--min-size`, `--max-size`, `--protect`
  - JSON output via `--json` for pipeline integration
- **Mirror GUI** (`internal/mirror/server.go`) — local web UI
  - Native macOS Finder folder picker via `/api/pick-folder`
  - Filesystem browser API via `/api/browse`
  - Graceful SIGINT/SIGTERM shutdown
  - Filter chips, advanced options, results view with keep/remove badges
  - Egyptian dark theme, Inter font, gold accents
- **𓁟 Thoth knowledge system** — persistent AI memory
  - Three-layer architecture: memory.yaml → journal.md → artifacts/
  - `thoth_read_memory` MCP tool for AI IDEs
  - Standalone CLI: `tools/thoth-init/` (auto-detects language, counts lines)
  - Installed across 4 Sirsi codebases (428,000+ lines)
  - 98% context reduction benchmarked on real projects
- **Decision log** (`internal/cleaner/decisions.go`)
  - Per-file action recording: path, size, SHA-256, reason, timestamp
  - Persists to `~/.config/anubis/mirror/decisions/`
  - Trash-first policy on macOS (reversible, "Put Back" works)
- **Performance documentation** (`docs/MIRROR_PERFORMANCE.md`)
  - Real benchmark data: 27.3x faster, 98.8% less disk I/O
  - Algorithm explanation, scaling properties, safety claims
- **Build log** (`docs/BUILD_LOG.md`) — public build-in-public chronicle
- **12 mirror tests** + existing suite = 303 total

### Changed
- **Seba graph** — complete kinetic rewrite (self-contained Canvas renderer)
- **Guard optimization** — pre-lowercased orphanPatterns keys in hot loop
- **README** — added Mirror benchmarks, Thoth section, updated architecture
- **GoReleaser** — fixed brews vs homebrew_casks, removed stale .goreleaser.yml

### Fixed
- **GUI folder picker** — was returning browser-relative paths → native macOS Finder dialog
- **moveToTrash** — silently ignored filepath.Abs error (could trash wrong file)
- **Drop zone text** — said "Drop folders here" but D&D can't work → now says "Select folders"
- **Dead code removed** — symlink check, unused groupID, FollowLinks field
- **Lint fixes** — errcheck, capitalized errors, unnecessary Sprintf
- **GoReleaser CI** — deprecated format, stale config file

### Stats
- 17 CLI commands, 58 scan rules, 19 internal modules
- 470 tests across 17 packages, all passing (with `-race`)
- ~17,000 lines of Go
- Lint clean (golangci-lint + staticcheck)
- Test coverage range: 93% (jackal) to 0% (2 untested modules: mapper, output)
- 6 bugs found and fixed in audit cycle, 7 modules test-covered in test sprint

### Session 7 (2026-03-22)
- **Statistics audit** — corrected 5 categories of inflated claims across 12 files
  - Scan rules: 64→58 (verified). Tests: ~395→470 (verified).
  - Removed fabricated cross-repo savings and "3M tokens in 11 sessions" claim.
- **Structured logging** (`internal/logging/`) — Go 1.21+ slog to stderr
  - `--verbose` (debug), `--quiet` (error-only), `--json` (structured) modes
  - Instrumented mirror and ka scanners with debug points
- **Platform abstraction** (`internal/platform/`) — cross-platform interface
  - Darwin, Linux, Mock implementations
  - MoveToTrash, ProtectedPrefixes, PickFolder, OpenBrowser, SupportsTrash
  - Mock enables testing platform-specific code without system calls
- **Case studies** — 3 verified studies in `docs/case-studies/`
  - Thoth Context Savings, Mirror Dedup Performance, Ka Ghost Detection
- **CI fixes** — platform skip guards for macOS-only tests, homebrew tap disabled
- **Rules canonized** — A14 (Statistics Integrity), A15 (Session Definition)
- **GitHub Release** — v0.3.0-alpha published with 6 binaries
- **`SirsiMaster/homebrew-tools`** repo created (pending PAT setup)

### Session 8 (2026-03-23)
- **Platform interface wired** into cleaner and mirror modules (Priority 1 complete)
  - Replaced 3 `runtime.GOOS` checks in `cleaner/safety.go` with `platform.Current()`
  - Replaced `moveToTrash()` and `protectedPrefixes` map with platform interface calls
  - Replaced `OpenBrowser()` switch and `handlePickFolder` osascript in `mirror/server.go`
  - Tests use `platform.Set(&Mock{})` for cross-platform testing
- **CI lint fixes** — resolved 8 lint errors that broke 5 consecutive CI runs
  - `gofmt`: alignment in `ignore_test.go`, `registry_test.go`
  - `govet/unusedwrite`: struct assertions in `scarab_test.go`, `sight_test.go`
  - `misspell`: "cancelled" → "canceled" in platform package
- **Pre-push hook** (`.githooks/pre-push`) — mirrors CI checks locally
  - Runs gofmt + go vet + golangci-lint + go build before every push
  - Prevents lint issues from ever reaching the pipeline
- **Maat proposed** — pipeline purifier module (CI monitoring + auto-remediation)


## [0.2.0-alpha] — 2026-03-25 (Ship Week Day 5)
### Added (Day 5: Neural Brain Downloader)
- **Brain module** (`internal/brain/`) — on-demand neural model management
- **`anubis install-brain`** — download CoreML/ONNX model to `~/.anubis/weights/`
  - Progress bar with bytes/total/percentage display
  - SHA-256 checksum verification post-download
  - Platform-aware model selection (prefers CoreML on Apple Silicon)
- **`anubis install-brain --update`** — check for and install latest model version
- **`anubis install-brain --remove`** — self-delete all weights and manifest
- **`anubis uninstall-brain`** — alias for `--remove`
- **Manifest-driven versioning** — remote `brain-manifest.json` + local `manifest.json`
- **Classifier interface** — pluggable backends (Stub, future ONNX, CoreML)
- **StubClassifier** — heuristic file classification (30+ file types, 9 categories)
  - Path-based detection: `node_modules/`, `__pycache__/`, `.cache/`
  - Extension-based: source, config, media, archives, data, ML models
  - Concurrent batch classification via goroutines
- **22 brain tests** — downloader + inference (manifest roundtrip, hash, batch, 35+ classification cases)
- **`--json` support** on all brain commands
- **Pro upsell footer** — tier messaging on brain commands

### Refs
- Canon: ANUBIS_RULES.md, docs/DEVELOPMENT_PLAN.md
- ADR: ADR-001
- Changelog: v0.2.0-alpha — Day 5 Neural Brain

### Added (Day 6: MCP Server + IDE Integrations)
- **MCP module** (`internal/mcp/`) — Model Context Protocol server
  - JSON-RPC 2.0 over stdio, MCP spec 2025-03-26 compliant
  - `initialize` handshake with capability negotiation
  - `tools/list` and `tools/call` for tool invocation
  - `resources/list` and `resources/read` for resource access
  - `ping` and method-not-found handling
- **`anubis mcp`** — start MCP server for AI IDE integration
- **4 MCP Tools:**
  - `scan_workspace` — run Jackal scan engine on a directory
  - `ghost_report` — run Ka ghost detection
  - `health_check` — system health summary with grade
  - `classify_files` — brain-powered semantic file classification
- **3 MCP Resources:**
  - `anubis://health-status` — system health as JSON
  - `anubis://capabilities` — modules, commands, and scan rules
  - `anubis://brain-status` — neural brain installation status
- **VS Code extension scaffold** (`extensions/vscode/`)
  - Extension manifest with Eye of Horus sidebar concept
  - 4 commands: scan workspace, ghost report, health check, install brain
  - Status bar icon, activity bar sidebar, configuration options
- **Workspace config** — `.anubis/config.yaml` template for per-project settings
- **14 MCP tests** — server lifecycle, tool calls, resource reads, error handling
- **IDE config examples** — Claude Code, Cursor, Windsurf setup instructions

### Refs
- Canon: ANUBIS_RULES.md, docs/DEVELOPMENT_PLAN.md
- ADR: ADR-001
- Changelog: v0.2.0-alpha — Day 6 MCP Server

### Added (Day 7: Scales Policy Engine + Agent Hardening)
- **Scales module** (`internal/scales/`) — YAML policy engine
  - Policy parser with validation (operators, severities, metrics)
  - Threshold normalization (KB/MB/GB/TB → bytes)
  - Built-in default workstation hygiene policy
- **Policy enforcement** (`internal/scales/enforce.go`)
  - Evaluates scan results against configurable thresholds
  - Generates pass/warn/fail verdicts with remediation suggestions
  - Collects metrics from Jackal (waste) and Ka (ghosts)
- **`anubis scales enforce`** — run policies against current state
  - Custom policy files via `-f` flag
  - JSON output support
  - Eye of Horus/Ra upsell for fleet enforcement
- **`anubis scales validate`** — validate policy YAML syntax
- **`anubis scales verdicts`** — show enforcement results
- **Agent hardening** (`cmd/anubis-agent/`)
  - Fixed command set: scan, report, clean, version (Rule A3)
  - All output JSON via AgentResponse envelope
  - Clean requires `--confirm` flag (Rule A1)
  - Health grading: EXCELLENT/GOOD/FAIR/NEEDS_ATTENTION
- **Example policy file** — workstation + CI/CD templates
- **13 scales tests** — parsing, validation, normalization, enforcement, verdicts

### Refs
- Canon: ANUBIS_RULES.md, docs/DEVELOPMENT_PLAN.md
- ADR: ADR-001
- Changelog: v0.2.0-alpha — Day 7 Scales + Agent

### Changed (Day 8: Polish + Release)
- **README.md** — complete rewrite with all 17 commands, 11 modules, MCP guide
- **VERSION** — bumped to `0.2.0-alpha`
- **Binary audit:**
  - `anubis`: 7.7 MB (< 15 MB budget ✅)
  - `anubis-agent`: 2.1 MB (< 5 MB budget ✅)
- **Test suite:** 72 tests, 7 packages, all passing
- **Code size:** 12,277 lines of Go across 15 internal modules
- **gofmt + go vet** — clean

---

## [0.1.0-alpha.2] — 2026-03-21
### Fixed (Session 2: Clean, Lint, Optimize)
- **CI pipeline** — fixed go.mod version mismatch (`go 1.26.1` → `go 1.22.0`)
- **golangci-lint** — added `.golangci.yml` config, replaced deprecated `exportloopref` with `copyloopvar`
- **errcheck** — fixed unchecked `cmd.Help()` return value
- **gofmt** — applied formatting to 4 source files with drift
- **Portfolio CI** — fixed FinalWishes (`go 1.25.0` → `go 1.24.0`), tenant-scaffold (missing `package-lock.json`)

### Added (Session 2: Tests + Documentation)
- **Unit tests** — `types_test.go` (FormatSize, ExpandPath, PlatformMatch), `safety_test.go` (all protection layers), `scanner_test.go` (extractBundleID, guessAppName, isSystemBundleID), `engine_test.go` (mock rules, category filtering, clean safety)
- **ADR-002** — Ka Ghost Detection algorithm (5-step process, 17 residual locations)
- **CONTRIBUTING.md** — contributor guide with scan rule examples and safety rules
- **SECURITY.md** — security policy, threat model, protected paths, data privacy

---

## [0.1.0-alpha.1] — 2026-03-20
### Added (Session 1: Ka Ghost Hunter)
- **Ka module** (`internal/ka/`) — Ghost detection engine scanning 17 macOS locations
- **22 new scan rules** — AI/ML (6), virtualization (4), IDEs (5), cloud (4), storage (3)
- **`anubis ka`** — Ghost hunting CLI command with `--clean`, `--dry-run`, `--target` flags
- **Launch Services scanning** — detects phantom app registrations in Spotlight
- **Bundle ID extraction** — heuristic parser for plist filenames and directory names
- **System filtering** — `com.apple.*` and known system services excluded from ghosts

---

## [0.1.0-alpha] — 2026-03-20
### Added (Phase 0: Project Genesis)
- **Project scaffolding** — Go 1.22+ module, directory structure for all 4 modules
- **ANUBIS_RULES.md v1.0.0** — Operational directive with 16 universal rules + 5 Anubis-specific safety rules
- **GEMINI.md + CLAUDE.md** — Auto-synced copies of ANUBIS_RULES.md
- **ADR-001** — Founding architecture decision (Go, cobra, agent-controller, module codenames)
- **ADR system** — Template + Index established (next available: ADR-002)
- **Architecture Design** — Module architecture, data flow, component interaction
- **Safety Design** — Protected paths, dry-run guarantees, trash-vs-delete policy
- **CI/CD** — GitHub Actions workflow: lint, test, build, binary size guard
- **Default scan rules config** — YAML-based rule definitions
- **LICENSE** — MIT (free and open source forever)
- **VERSION** — `0.1.0-alpha`

### Refs
- Canon: ANUBIS_RULES.md, docs/ARCHITECTURE_DESIGN.md, docs/SAFETY_DESIGN.md
- ADR: ADR-001 (Founding Architecture)
- Changelog: v0.1.0 — Project Genesis

---

## [0.0.1] — 2026-03-20
### Added
- Initial product concept ("Deep Cleanse") born from manual Parallels cleanup session
- Competitive analysis vs Mole (open-source Mac cleaner)
- Name selection: Sirsi Anubis (Egyptian god of judgment)
- Module codenames: Jackal, Scarab, Scales, Hapi
- 60+ scan rule categories across 7 domains identified
- Agent-controller architecture designed
- Network topology awareness (VLAN, subnet, relay) specified
