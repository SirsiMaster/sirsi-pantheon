# 𓂀 Building Anubis in Public

> A transparent record of how Sirsi Anubis was designed, built, tested, broken, fixed, and shipped. No cherry-picking — the mistakes stay in.

[![Version](https://img.shields.io/badge/version-0.3.0--alpha-C8A951?style=flat)](CHANGELOG.md)
[![Tests](https://img.shields.io/badge/tests-~395%20passing-brightgreen?style=flat)](.github/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat)](LICENSE)

---

## Why Build in Public?

Most developer tools ship a polished website with marketing claims. You never see the messy middle — the bugs that got shipped, the benchmarks that didn't hold up, the architecture decisions that were wrong the first time.

We're showing all of it. Every cycle of **build → test → find problems → fix → test again** is documented here. If we claim "27.3x faster," we show you the real benchmark data. If a module has zero test coverage, we say so.

This document is the high-level narrative. The [CHANGELOG](CHANGELOG.md) has every detail. The [engineering journal](.thoth/journal.md) has the reasoning behind every decision.

---

## The Build Cadence

### Sprint 0 — Genesis (March 20, 2026)

**What happened**: The idea emerged from a manual Parallels cleanup that took 3 hours and recovered 47GB. "Why am I doing this by hand?"

**Built**: Project skeleton, CI/CD pipeline, founding architecture document, safety design, MIT license, 16 governance rules.

**Tested**: Build pipeline verified. Zero application code — just scaffolding.

```
Commits: 3  |  Lines: ~400  |  Tests: 0  |  Version: 0.0.1
```

### Sprint 1 — Core Scanner (March 20, Day 1-2)

**What happened**: Built the Jackal scan engine (64 rules across 7 domains), Ka ghost hunter (17 macOS locations), Guard RAM auditor, and Sight LaunchServices scanner. Broke CI twice — go.mod version mismatch and golangci-lint deprecations.

**Built**: 5 internal modules, 22 scan rules expanded to 64, ghost detection engine, RAM process grouping.

**Tested**: First test suite — unit tests for types, safety validation, scanner functions, engine lifecycle. CI went green after 2 hotfixes.

**Broke**: CI failed on `go 1.26.1` in go.mod (doesn't exist). Fixed to `1.22.0`. Linter config used deprecated `exportloopref`.

```
Commits: 15  |  Lines: ~5,000  |  Tests: 45  |  Version: 0.1.0-alpha
```

### Sprint 2 — Brain + MCP + Policy (March 21, Day 5-7)

**What happened**: Added neural model downloader, MCP server for AI IDEs, and Scales policy engine. All built in one session with heavy testing. The agent binary gained hardened command-set restrictions.

**Built**: Brain module (model download + classify), MCP server (4 tools, 3 resources), Scales policy engine (YAML enforcement), agent hardening.

**Tested**: 72 tests across 7 packages. All passing with `-race`. Binary sizes under budget (7.7M + 2.1M).

```
Commits: 8  |  Lines: ~12,000  |  Tests: 72  |  Version: 0.2.0-alpha
```

### Sprint 3 — Mirror Engine (March 21, Day 8)

**What happened**: Built the file deduplication engine — the product's revenue feature. Started with naive full-file hashing. Testing revealed it was too slow for large files.

**Built**: Three-phase dedup scanner, 8-worker parallel hashing, smart keep/delete recommender, GUI web interface.

**Tested**: 12 Mirror-specific tests. Ran benchmark on real ~/Downloads directory.

**Broke**: GUI folder picker returned relative paths (browser security sandbox). Couldn't actually scan real folders. Fixed with native macOS Finder dialog via osascript.

```
Commits: 5  |  Lines: ~14,500  |  Tests: 84  |  Version: 0.3.0-alpha
```

### Sprint 4 — Performance + Safety Audit (March 21-22, Audit Cycle 1)

**What happened**: Deep audit of every module. Key insight: "measure twice, cut once" — hash the first 4KB then the last 4KB. Implemented two-phase partial hashing. Testing exposed the broken GUI folder picker. Added trash-first deletion with full audit trail.

**Built**: Partial hash pre-filter, DecisionLog system, `/api/browse` filesystem browser, `/api/pick-folder` native picker, graceful SIGINT shutdown.

**Tested**: Benchmark verified — 27.3x faster, 98.8% less disk I/O. 303 tests all passing.

**Fixed**: 6 bugs including a safety-critical one — `moveToTrash()` was silently ignoring a path resolution error that could have trashed the wrong file.

**Performance benchmark** (real numbers from ~/Downloads):
| Metric | Before | After |
|:-------|:-------|:------|
| Files hashed | 56 full | 56 partial → 44 full |
| Bytes read | 97.8 MB | < 2 MB |
| Time | 84 ms | 3 ms |
| Accuracy | 25 dupes | 25 dupes (identical) |

```
Commits: 7  |  Lines: ~14,800  |  Tests: 303  |  Bugs fixed: 6
```

### Sprint 5 — Thoth Knowledge System (March 22, Session)

**What happened**: Identified a core problem with AI-assisted development — LLMs have no persistent memory across sessions. Built a three-layer persistent knowledge system. Named it **Thoth** after the Egyptian god of knowledge.

**Built**: Thoth specification, project template, standalone CLI tool (`thoth-init`), MCP tool (`thoth_read_memory`), global AI skill, session workflow. Installed across 4 codebases.

**Tested**: CLI tested in non-interactive mode. Token savings benchmarked across 428,000 lines of real code.

**Innovation**: 98% reduction in context tokens needed for AI session startup. This is independently verifiable — compare reading 100 lines of YAML vs 5,000 lines of source.

```
Commits: 5  |  Repos touched: 4  |  Files created: 14  |  Version: 0.3.0-alpha
```

### Sprint 6 — Test Coverage Blitz (March 22, Session 2)

**What happened**: 9 out of 17 modules had zero test coverage. That's unacceptable for a product launching in April. Wrote tests for 7 modules in priority order: ignore → rules → profile → stealth → hapi → scarab → sight.

**Built**: 7 test files covering all priority modules. Focused on pure functions and unit tests that work in CI (temp dirs, struct validation, parsing) — no tests that require live network, Docker, or system-level macOS calls.

**Tested**: ~395 tests across 15 suites. All passing with `-race`. Full `go vet` clean.

**Found**:
- ARP parsing edge case: macOS `(incomplete)` entries match the parenthesis-detection regex, overwriting the IP. Not user-facing (rejected by `isValidIP`) but documents a fragile parser.
- Registry comment says "8 IDE rules" but only 7 are listed. Cosmetic.
- Constructor names don't always match internal rule names (`NewRustTargetRule` → `rust_targets`). Tests caught it.
- All default profiles include "general" category — good product design confirmed.

```
Commits: 3  |  Tests written: 94  |  Total: ~395  |  Suites: 15/17
```

### Sprint 7 — Launch Preparation (March 22, Session 2)

**What happened**: Verified all platform builds, updated launch materials with current stats, added Mirror dedup scene to investor demo.

**Built**: GoReleaser snapshot (12 binaries), updated launch copy with competitor table, updated investor demo script.

**Tested**: All 12 binaries compile clean — darwin/linux/windows × amd64/arm64. Binary sizes within budget: anubis ~8MB (≤15MB), agent ~2MB (≤5MB).

**Updated**: CHANGELOG stats, README badge (303 → ~395), goreleaser release header, Thoth memory + journal.

```
Commits: 2  |  Binaries verified: 12  |  Docs updated: 6
```

---

## Current State — v0.3.0-alpha

| Metric | Value | Verified |
|:-------|:------|:--------:|
| Go source lines | ~15,000 | ✅ `find + wc -l` |
| Test count | ~395 | ✅ `go test ./...` |
| Test suites | 15/17 modules | ✅ `go test ./...` |
| Test coverage (best) | Jackal: 93% | ✅ `go test -cover` |
| Test coverage (worst) | 2 modules: 0% (mapper, output) | ✅ Disclosed |
| Lint status | Clean | ✅ golangci-lint |
| Race detector | Clean | ✅ `-race` flag |
| CI/CD | Green | ✅ GitHub Actions |
| Binary size | ~8 MB + ~2 MB | ✅ GoReleaser snapshot |
| Scan rules | 64 across 7 domains | ✅ Counted |
| Protected paths | 29 hardcoded | ✅ Code review |
| Cross-compile | 6 platforms (3 OS × 2 arch) | ✅ GoReleaser snapshot |
| Platform | macOS (100%), Linux (~60%), Windows (~40%) | ✅ Audited |

### What Works:
- [x] Scan 64 waste categories across your system
- [x] Find & deduplicate files 27.3x faster than naive hashing
- [x] GUI and CLI with feature parity
- [x] Hunt ghost apps left by uninstalled software
- [x] AI IDE integration via MCP (5 tools)
- [x] Safety: trash-first, decision log, 29 protected paths
- [x] Thoth: persistent AI memory across sessions
- [x] 15 of 17 modules have test coverage
- [x] Cross-compiles for 6 platforms, all within binary size budget

### What Doesn't (yet):
- [ ] 2 modules have zero test coverage (mapper, output — display-only)
- [ ] Cleaner (safety-critical code) has only ~49% coverage
- [ ] No structured logging
- [ ] GUI folder picker is macOS-only
- [ ] No Linux/Windows trash integration

---

## How to Verify Our Claims

Every claim in this document is verifiable:

```bash
# Test count
go test ./... 2>&1 | grep -c 'ok'

# Coverage by module
go test -cover ./...

# Dedup benchmark (run on your own files)
time anubis mirror ~/Downloads --json | jq '.totalDuplicates'

# Binary size
ls -lh $(which anubis)

# Protected paths (in source)
grep -c 'protected' internal/cleaner/safety.go

# Thoth token savings (compare lines)
wc -l .thoth/memory.yaml
find . -name '*.go' | grep -v _test | xargs wc -l | tail -1
```

---

## Next Milestone: v0.4.0 (Target: March 28, 2026)

**Focus**: Production readiness for April investor demos

| P0 | P1 | P2 |
|:---|:---|:---|
| Cleaner to 80%+ coverage | Linux folder picker | npm publish thoth-init |
| Scanner edge case tests | Structured logging | VS Code extension |
| Product Hunt / HN launch | Platform interface | GitHub Action for Thoth |

---

*Last updated: March 22, 2026 (Sprint 7). This document is updated with every sprint.*

*See [CHANGELOG.md](CHANGELOG.md) for detailed changes. See [.thoth/journal.md](.thoth/journal.md) for design reasoning.*
