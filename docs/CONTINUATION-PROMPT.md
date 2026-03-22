# 𓂀 Sirsi Anubis — Continuation Prompt
**Date:** March 22, 2026 (Saturday, 4:39 PM ET)
**Session:** Build-in-Public + Thoth Independence + Audit Cycle
**Repo:** `github.com/SirsiMaster/sirsi-anubis`
**Path:** `/Users/thekryptodragon/Development/sirsi-anubis`

---

## CRITICAL: Read Before Starting

1. **Read `.thoth/memory.yaml`** — compressed project state (~100 lines). This replaces reading source files.
2. **Read `.thoth/journal.md`** — timestamped reasoning (8 entries).
3. **Read `ANUBIS_RULES.md`** — the 12 non-negotiable safety rules.
4. **Scope**: Test coverage + launch preparation. No new features.
5. **Deadline: Friday March 28** — April investor demos require complete product.
6. **All code compiles and tests pass** — do NOT break the build.
7. **ADR-003 is ACTIVE** — every release must update BUILD_LOG.md, build-log.html, CHANGELOG, Thoth.
8. **Context monitoring** — see `.agent/workflows/context-monitoring.md`. Report after every sprint.

---

## What Exists Right Now (All Working, All Tested)

### Binary
- **Size:** ~12 MB (macOS arm64)
- **Go:** 1.22+, Cobra CLI, lipgloss terminal UI
- **Version:** 0.3.0-alpha (tagged)
- **Tests:** 303 passing, 0 lint warnings

### 17 CLI Commands (all compiled and running)

| Command | Module | Description | Status |
|:--------|:-------|:-----------|:-------|
| `anubis version` | updater | Version + update check | ✅ |
| `anubis weigh` | jackal | Scan workstation (64 rules, 7 domains) | ✅ |
| `anubis judge` | cleaner | Clean artifacts with trash-first safety | ✅ |
| `anubis ka` | ka | Ghost app hunter | ✅ |
| `anubis guard` | guard | RAM audit + process slayer | ✅ |
| `anubis sight` | sight | Launch Services fix + Spotlight reindex | ✅ |
| `anubis profile` | profile | 4 scan profiles | ✅ |
| `anubis seba` | mapper | Interactive infrastructure graph | ✅ |
| `anubis hapi` | hapi | GPU detection, dedup, snapshots | ✅ |
| `anubis scarab` | scarab | Network discovery + container audit | ✅ |
| `anubis mirror` | mirror | File deduplication (CLI + GUI) | ✅ |
| `anubis install-brain` | brain | Neural model downloader | ✅ |
| `anubis uninstall-brain` | brain | Remove neural weights | ✅ |
| `anubis mcp` | mcp | MCP server (5 tools) | ✅ |
| `anubis scales enforce` | scales | Policy engine enforcement | ✅ |
| `anubis book-of-the-dead` | (hidden) | System autopsy | ✅ |
| `anubis initiate` | (cli) | macOS permission wizard | ✅ |

### Module Test Coverage

| Module | Tests | Coverage |
|:-------|:------|:---------|
| jackal | ✅ | 93% |
| cleaner | ✅ | 49% |
| ka | ✅ | 19.5% |
| guard | ✅ | 42 tests |
| brain | ✅ | has tests |
| mcp | ✅ | has tests |
| mirror | ✅ | has tests |
| scales | ✅ | has tests |
| hapi | ❌ | **0% — needs tests** |
| ignore | ❌ | **0% — needs tests (user-facing)** |
| jackal/rules | ❌ | **0% — needs tests (64 rules)** |
| mapper | ❌ | 0% (low priority) |
| output | ❌ | 0% (low priority) |
| profile | ❌ | **0% — needs tests** |
| scarab | ❌ | 0% (medium) |
| sight | ❌ | 0% (macOS-only) |
| stealth | ❌ | **0% — needs tests** |
| updater | ❌ | 0% (low priority) |

### Infrastructure
- `.github/workflows/ci.yml` — CI pipeline (lint + test + build)
- `.github/workflows/release.yml` — goreleaser on v* tag push
- `.goreleaser.yml` — multi-platform binaries + Homebrew formula
- `extensions/vscode/` — VS Code extension scaffold (package.json + extension.js)
- Git tag `v0.3.0-alpha` exists

### Documentation (all current)
- `docs/BUILD_LOG.md` — sprint chronicle in markdown
- `docs/build-log.html` — public HTML build log (Swiss Neo-Deco design)
- `docs/ADR-001-FOUNDING-ARCHITECTURE.md` — founding decisions
- `docs/ADR-002-KA-GHOST-DETECTION.md` — ghost detection algorithm
- `docs/ADR-003-BUILD-IN-PUBLIC.md` — build-in-public as canonical process
- `docs/THOTH.md` — Thoth specification (links to standalone repo)
- `docs/SAFETY_DESIGN.md`, `SECURITY.md`, `CONTRIBUTING.md`
- `CHANGELOG.md` — current to v0.3.0-alpha
- `README.md` — badges, "Weigh. Judge. Purify." tagline

### Thoth (Standalone Repo)
- **Repo:** `github.com/SirsiMaster/sirsi-thoth` (v1.0.0, tagged)
- **CLI:** `npx thoth-init` — auto-detects language, scaffolds .thoth/, injects IDE rules
- **IDE support:** Cursor, Windsurf, Claude Code, Gemini, Copilot (auto-injected)
- **No MCP required** — just rules files telling the AI to read memory.yaml
- **Synced:** Anubis has copy in `tools/thoth-init/`

### Cross-Links (Live)
- SirsiNexus Portal `index.html` has "Sirsi Anubis" preview section
- Anubis `build-log.html` has "Part of Sirsi" section linking to NexusApp
- Both explain the Anubis→Ra relationship

---

## WHAT TO BUILD NEXT

### Priority 1: Test Coverage (target: 70%+ modules with tests)

Write tests for these high-priority modules:

```
internal/ignore/ignore_test.go     — .anubisignore is user-facing, must be tested
internal/jackal/rules/*_test.go    — 64 scan rules need registration verification
internal/profile/profile_test.go   — scan profiles are user-facing
internal/stealth/stealth_test.go   — ephemeral mode must clean up correctly
internal/hapi/detect_test.go       — GPU detection
```

Lower priority (nice to have):
```
internal/scarab/*_test.go          — network discovery
internal/sight/*_test.go           — macOS LaunchServices
internal/output/*_test.go          — terminal rendering
```

### Priority 2: Launch Preparation

```
- Product Hunt / Hacker News launch copy draft
- Investor demo script (5-minute walkthrough)
- goreleaser snapshot build (verify all platforms compile)
- GitHub Release draft for v0.3.0-alpha
```

### Priority 3: Update Build-in-Public Artifacts (per ADR-003)

After completing Priorities 1-2:
```
- Update BUILD_LOG.md with test coverage sprint
- Update build-log.html stats (test count, module coverage)
- Update CHANGELOG.md
- Update .thoth/memory.yaml (test_count, etc.)
- Journal entry for test coverage decisions
```

---

## Key Context from This Session

1. **"Weigh. Judge. Purify."** — canonical tagline (was "Purge", now "Purify")
2. **Sirsi Pantheon** — Anubis, Thoth, Ka, Ra, Seba, Hapi, Scarab are Egyptian-themed tools
3. **Thoth is independent** — standalone repo, works without Anubis
4. **Build-in-public is ADR-003** — not optional, enforced by session workflow Step 6
5. **Voice rule**: Never say "the user wanted/suggested." Use direct verbs: built, fixed, refactored.
6. **Audience**: GUI for everyone (parents, students, hobbyists). CLI for devs and AI engineers.
7. **Anubis→Ra**: Anubis is standalone preview; Ra is the full module coming in SirsiNexus
8. **Context monitoring**: `.agent/workflows/context-monitoring.md` — report after every sprint

---

## Dev Machine Specs

- **CPU:** Apple M1 Max (10 cores)
- **GPU:** Apple M1 Max (32 cores, Metal 4)
- **Neural Engine:** ✅ Available
- **RAM:** 32 GB unified memory
- **Disk:** 926 GB

---

## Rules of Engagement

1. **Read `.thoth/memory.yaml` FIRST** — do not re-read source files the memory already covers.
2. **Build → Test → Commit → Push** after every feature.
3. **Never break the build** — `go build && go test ./... && go vet ./...` must pass.
4. **ADR-003 is enforced** — every release updates 7 artifacts.
5. **Check actual struct field names** before using them.
6. **Binary size budget:** controller < 15 MB, agent < 5 MB.
7. **Monitor context** — `.agent/workflows/context-monitoring.md`. Report after every sprint.
8. **Voice**: Direct verbs only. No "the user wanted."

---

## Start Command

```bash
cd /Users/thekryptodragon/Development/sirsi-anubis
cat .thoth/memory.yaml
go build ./cmd/anubis/ && go test ./... && echo "✓ Ready"
```

Then begin Priority 1: `internal/ignore/ignore_test.go`
