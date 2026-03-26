# Changelog — Sirsi Pantheon
All notable changes to this project are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/).

**Building in public** — see [docs/BUILD_LOG.md](docs/BUILD_LOG.md) for the full narrative.

---

## [Unreleased]
### Planned
- P0: Reach 95%+ coverage on remaining modules (Sight, Platform, Mirror)
- P1: Thoth auto-sync (Horus/Ra feed facts)
- P1: CoreML embeddings on ANE (60x speedup)
- P2: npm publish thoth-init, VS Code extension

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
