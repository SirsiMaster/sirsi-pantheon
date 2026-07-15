# 𓁢 Sirsi Pantheon — Canonical Roadmap
**Version:** 6.0.0 (Reality Sync)
**Date:** July 1, 2026
**Status:** **v0.23.8-beta shipped** (signed + notarized, tagged 2026-06-30) — next: v1.0.0-rc1 Star-Grade sprint

> **The coverage tables in §2 are a dated snapshot** verified by `go test -cover ./...` on
> March 31, 2026 (the honest-measurement reset). Current repo state as of July 1, 2026:
> `VERSION` = 0.23.8-beta = latest tag `v0.23.8-beta`; 2,078 test functions
> (`grep -r '^func Test' --include='*_test.go' | wc -l`); Go 1.25.

## 1. Vision: Pantheon Anubis — Standalone DevOps Intelligence CLI
Pantheon is the single, modular brand for all Sirsi automation deities. The standalone CLI (Pantheon Anubis) gives you all features in one binary. Each deity can also be installed independently.

## 2. Deity Pillars (coverage snapshot: v0.8.0-beta, March 31, 2026)

| Pillar | Glyph | Role | Coverage | Status |
|:-------|:------|:-----|:---------|:-------|
| **Anubis** | 𓁢 | Infrastructure Hygiene (Jackal, Ka, Mirror, Cleaner) | 80-95% | ✅ Shipped |
| **Ma'at** | 𓆄 | Governance & Diagnostics (Scales, Isis) | 79-95% | ✅ Shipped |
| **Thoth** | 𓁟 | Knowledge & Memory (Sync, Init, Journal) | 83.0% | ✅ Shipped |
| **Seba** | 𓇽 | Mapping, Discovery & Hardware (Scarab, Seba, Hapi) | 62-95% | ✅ Shipped |
| **Seshat** | 𓁆 | Knowledge Export (MCP bridge) | 63-85% | ⚠️ Minimal |
| **Neith** | 𓇼 | Orchestration | 100% (stub) | 📋 Deferred to v1.0 |

### Sub-module Coverage (Verified `go test -cover`, March 31, 2026)

| Module | Coverage | Verdict | Notes |
|:-------|:---------|:--------|:------|
| brain | 90.0% | ✅ | Neural weight management |
| cleaner | 85.7% | ✅ | Safety-critical (29 protected paths) |
| guard | 87.8% | ✅ | Safety-critical (RAM pressure) |
| hapi | 62.5% | ⚠️ | Hardware detection works; ANE path untested |
| horus | 89.5% | ✅ | Eye of Horus monitoring |
| ignore | 91.8% | ✅ | Gitignore-style path matching |
| isis | 80.1% | ✅ | Diagnostic (lint, vet, AST analysis) |
| jackal | 94.6% | ✅ | Core scan engine |
| jackal/rules | 64.5% | ⚠️ | 81 rules, 37 tested |
| ka | 92.6% | ✅ | Ghost app detection (macOS) |
| logging | 95.2% | ✅ | Structured logging |
| maat | 79.3% | ⚠️ | Governance engine (dynamic discovery fixed) |
| mcp | 62.5% | ⚠️ | Model Context Protocol server |
| mirror | 80.0% | ✅ | Dedup engine (27x partial hash speedup) |
| neith | 100.0% | ✅ | Stub — 2 tests only, deferred to v1.0 |
| osiris | 92.8% | ✅ | Resurrection/recovery |
| output | 87.5% | ✅ | Terminal rendering (lipgloss) |
| platform | 66.5% | ⚠️ | OS detection (structurally capped) |
| profile | 85.1% | ✅ | User profile management |
| scales | 94.6% | ✅ | YAML policy enforcement |
| scarab | 94.8% | ✅ | Container/fleet scanning |
| seba | 87.1% | ✅ | Mermaid diagram generation |
| seshat | 84.9% | ✅ | Knowledge export (Markdown) |
| sight | 68.4% | ⚠️ | macOS LaunchServices (OS-specific) |
| stealth | 82.6% | ✅ | Process stealth detection |
| thoth | 83.0% | ✅ | Knowledge system (sync + init) |
| updater | 87.7% | ✅ | Binary self-update |
| yield | 82.1% | ✅ | Resource yield management |

**Weighted Average:** ~83.5% across 28 packages (27 modules + jackal/rules)

## 3. Global Metrics (snapshot: March 31, 2026 — current test count is 2,078 funcs as of 2026-07-01)

| Metric | Value | Source |
|:-------|:------|:-------|
| **Tests Passing** | 1,450+ | `go test -short ./...` |
| **Packages Passing** | 28/28 | `go test ./...` (27 modules + jackal/rules) |
| **Weighted Coverage** | ~83.5% | `go test -cover ./...` |
| **Internal Modules** | 27 | `ls internal/` |
| **Binary Size** | ~12 MB | Compiled `sirsi` binary |
| **Lint Errors** | 0 | `golangci-lint run ./...` |
| **CI Status** | ✅ Green | All 5 jobs passing |
| **E2E Smoke Tests** | 9 | `scripts/smoke.sh` |

## 4. Phase Schedule (2026)

### Phase 1: Foundation (Anubis Launch) — ✅ March 21
- CLI, scan engine, safety system, 81 rules, ghost hunter.

### Phase 2: Unification (Pantheon Launch) — ✅ March 23
- Ma'at, Horus, Thoth integrated into unified CLI.

### Phase 2.5: Token Intelligence (v0.17.0) — ✅ Shipped April 2026
- **RTK** (`internal/rtk/`) — Output filter: ANSI strip, dedup, truncation with tail preservation. 4 files, 12 tests. CLI: `sirsi rtk`. MCP tool: `filter_output`.
- **Vault** (`internal/vault/`) — SQLite FTS5 context sandbox + BM25 code search index. 4 files, 9 tests. CLI: `sirsi vault`. 6 MCP tools.
- **Horus** (`internal/horus/`) — Go AST symbol graph, file outlines (8-49x reduction), symbol context queries. 5 files, 10 tests. CLI: `sirsi horus`. 3 MCP tools.
- **Totals**: 16 new files, 31 new tests, 10 new MCP tools, 8 new Stele event types. Module count: 30 to 33. Deity/module count: 9 to 12.
- Composition pipeline: RTK -> Vault -> Horus. Zero new external dependencies.
- Horus dogfood: parsed 169 Go files from Pantheon itself, extracted 328 types, 15 interfaces. `tools.go` outline: 700+ lines to ~30 lines (23x).

### Phase 3: Hardening & Honest Measurement — ✅ March 31 (v0.8.0-beta)
- Fixed Ma'at dynamic module discovery (was reporting false 0% for 10 modules)
- Thoth folded from standalone repo into Pantheon (Go port + subtree merge)
- All golangci-lint errors resolved (40+ fixes across 19 files)
- CI/CD fully green (Go 1.24, golangci-lint v6)
- E2E smoke test suite (9 tests against compiled binary)
- Honest version: v0.8.0-beta (not premature RC1)

### Phase 4: Public Beta Release — ✅ Shipped (April–June 2026)
- v0.8.0-beta tagged and published; releases continued through **v0.23.8-beta** (tagged 2026-06-30)
- Signed + notarized DMG releases with stable TCC identity (since v0.23.3-beta)
- `sirsi update` self-update installing signed releases (#98)
- Native macOS menubar — NSStatusItem + NSPopover + SwiftUI (ADR-030)
- Mac-first platform roadmap adopted: CLI → Menubar → TUI → GUI (ADR-032); Windows/Linux GUI deferred
- ADR-033 Remediation Catalog accepted (2026-06-30): every finding maps to a real macOS lever, never a monitor

### Phase 5: v1.0.0-rc1 (Earned, Not Declared) — 🚧 CURRENT: the Star-Grade sprint
- See §5 below — governed by `docs/prd/RELEASE_V1_STAR_GRADE.md`

## 5. Next: v1.0.0-rc1 Star-Grade Sprint

The current sprint drives v0.23.8-beta → **v1.0.0-rc1**, governed by the release PRD:
**[`docs/prd/RELEASE_V1_STAR_GRADE.md`](prd/RELEASE_V1_STAR_GRADE.md)** (lands with PR #132).
Every public claim must match reality (this document included); rc1 is earned by closing the
PRD's star-grade gates, not declared.

---
**Custodian**: 𓆄 Ma'at (dynamic measurement, not hardcoded)
**Last Verified**: July 1, 2026 — `VERSION` 0.23.8-beta = tag `v0.23.8-beta`; 2,078 test functions. (§2 coverage tables: March 31, 2026 snapshot.)
**Measurement**: `go test -cover ./...` (verified) + `golangci-lint run` (clean).
*Building in public. The feather weighs true. No excuses.*
