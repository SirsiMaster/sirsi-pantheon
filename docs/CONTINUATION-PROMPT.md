# 𓂀 Sirsi Anubis — Continuation Prompt
**Date:** March 23, 2026 (Sunday, 5:33 AM ET)
**Session:** Session 10 — Pantheon Consolidation
**Repo:** `github.com/SirsiMaster/sirsi-anubis`
**Path:** `/Users/thekryptodragon/Development/sirsi-anubis`
**CI Status:** ✅ Green (pre-push hook active since session 8)

---

## CRITICAL: Read Before Starting

1. **Run `/session-start`** — the Thoth workflow at `.agent/workflows/session-start.md`
2. **Read `.thoth/memory.yaml`** — compressed project state. This replaces reading source files.
3. **Read `.thoth/journal.md`** — timestamped reasoning (14 entries).
4. **Read `ANUBIS_RULES.md`** — the 15 non-negotiable safety rules.
5. **Read `docs/SIRSI_PORTFOLIO_STANDARD.md`** — the 26 universal rules (NEW in Session 9).
6. **Deadline: Friday March 28** — April investor demos require complete product.
7. **All code compiles and 522 tests pass** — do NOT break the build.
8. **ADR-003 is ACTIVE** — every release must update BUILD_LOG.md, build-log.html, CHANGELOG, Thoth.
9. **ADR-005 is ACTIVE** — Pantheon is the product. All deities are sub-systems. See below.
10. **Pre-push hook is ACTIVE** — `.githooks/pre-push` runs golangci-lint before every push.

---

## 🏛️ Pantheon Vision (ADR-005 — Canonized Session 9)

**Pantheon** is the unified DevOps intelligence platform. One install, all deities.

```
Sirsi Technologies (super-repo / company)
└── Pantheon (product / monorepo / brand)
    ├── 𓂀 Anubis    — Hygiene (Go)        — v0.3.0-alpha  ← MATURE
    ├── 𓁟 Thoth     — Knowledge (JS/Go)   — v1.0.0        ← MATURE
    ├── 🪶 Ma'at     — Governance (Go)     — v0.1.0        ← BUILT SESSION 9
    ├── ⚖️ Scales    — Policy (Go)         — within Anubis
    ├── 🪲 Scarab    — Network (Go)        — within Anubis
    ├── 🌊 Hapi      — Resources (Go)      — within Anubis
    ├── 🛡️ Guard     — Memory (Go)         — within Anubis
    ├── 🪞 Mirror    — Dedup (Go)          — within Anubis
    ├── 𓂓 Ka         — Ghost Hunting (Go)  — within Anubis
    ├── 👁️ Sight     — LaunchServices (Go) — within Anubis
    └── [future deities — any language]
```

Key principles (ADR-005):
- Deities keep their own repos and versions
- Polyglot by design (Go, JS, future: anything)
- Single install gives all deities
- `npx thoth-init` continues to work standalone

---

## 𓁟 Thoth — Session Management

### 1. Project Memory (Read at start, update at end)
| Layer | File | When |
|:------|:-----|:-----|
| Memory | `.thoth/memory.yaml` | **ALWAYS first** — architecture, decisions, limitations |
| Journal | `.thoth/journal.md` | When WHY matters — 14 timestamped entries |
| Artifacts | `.thoth/artifacts/` | Deep dives — benchmarks, audits, **roi-metrics.md** |

### 2. Context Window Monitoring
After every sprint, report session metrics. Wrap at 🔴.

---

## What Exists Right Now (All Working)

### Core Modules (20 internal packages)
| Module | Package | Description |
|:-------|:--------|:------------|
| Jackal | `internal/jackal/` | 58 scan rules across 7 domains |
| Ka | `internal/ka/` | Ghost app detection (17 macOS locations) |
| Mirror | `internal/mirror/` | File dedup (27.3x faster via partial hashing) |
| Guard | `internal/guard/` | RAM audit + process slayer |
| Cleaner | `internal/cleaner/` | Trash-first deletion with decision log |
| Hapi | `internal/hapi/` | GPU detection, dedup engine, snapshots |
| Sight | `internal/sight/` | LaunchServices ghost repair |
| Scarab | `internal/scarab/` | Network discovery + fleet sweep |
| Brain | `internal/brain/` | Neural model downloader + classifier |
| MCP | `internal/mcp/` | Model Context Protocol server |
| Scales | `internal/scales/` | Policy engine + violation reporting |
| Profile | `internal/profile/` | Scan profiles (quick/full/custom) |
| Stealth | `internal/stealth/` | Ephemeral execution + post-run cleanup |
| Ignore | `internal/ignore/` | .anubisignore file support |
| Logging | `internal/logging/` | slog-based structured logging |
| Platform | `internal/platform/` | OS abstraction: Darwin, Linux, Mock |
| **Ma'at** | `internal/maat/` | **QA/QC governance (NEW session 9)** |
| Mapper | `internal/mapper/` | Filesystem mapper (no tests) |
| Output | `internal/output/` | Terminal rendering (no tests) |
| Updater | `internal/updater/` | Version check + advisory system |

### CLI Commands (18)
| Command | Module | Description |
|:--------|:-------|:------------|
| `anubis weigh` | jackal | Scan workstation (58 rules, 7 domains) |
| `anubis judge` | cleaner | Clean with trash-first safety |
| `anubis ka` | ka | Ghost app hunter |
| `anubis guard` | guard | RAM audit + process slayer |
| `anubis sight` | sight | Fix Spotlight ghost registrations |
| `anubis hapi` | hapi | GPU detect + VRAM status |
| `anubis scarab` | scarab | Network discovery |
| `anubis mirror` | mirror | File dedup (GUI or CLI) |
| `anubis seba` | - | Dependency graph visualization |
| `anubis book-of-the-dead` | - | Deep system autopsy |
| `anubis initiate` | - | macOS permission granting |
| `anubis install-brain` | brain | Download neural models |
| `anubis uninstall-brain` | brain | Remove neural models |
| `anubis mcp` | mcp | Start MCP server |
| `anubis scales` | scales | Enforce policies |
| `anubis profile` | profile | Manage scan profiles |
| `anubis version` | updater | Version + update check |
| **`anubis maat`** | **maat** | **QA/QC governance (NEW session 9)** |

### Ma'at Details (built Session 9)
```
internal/maat/maat.go      — Core types: Verdict, Assessment, CanonLink, Report, Assessor, Weigh()
internal/maat/coverage.go  — Coverage governance (parses go test -cover, per-module thresholds)
internal/maat/canon.go     — Canon verification (git log → canon ref matching)
internal/maat/pipeline.go  — Pipeline monitor (gh CLI → failure categorization)
cmd/anubis/maat.go         — CLI: anubis maat [--pipeline] [--coverage] [--canon] [--json]
57 tests across 3 test files
```

### Test Coverage
| Package | Tests | Coverage |
|:--------|------:|:---------|
| brain | 22 | Unit + integration |
| cleaner | 30 | 77% — safety-critical |
| guard | 12 | RAM + process |
| hapi | 20 | GPU, dedup, snapshots |
| ignore | 17 | Pattern matching |
| jackal/rules | 11 | Rule registry |
| ka | 28 | 42.7% — ghost detection |
| logging | 6 | Level modes |
| **maat** | **57** | **Coverage, canon, pipeline (NEW)** |
| mcp | 5 | Server lifecycle |
| mirror | 12 | Dedup engine |
| platform | 11 | All implementations |
| profile | 16 | Scan profiles |
| scales | varies | Policy engine |
| scarab | 18 | Network + ARP parsing |
| sight | 4 | LaunchServices |
| stealth | 9 | Cleanup engine |
| **Total** | **522** | **18 suites** |

### Infrastructure
- CI: `.github/workflows/ci.yml` (lint + test + build)
- Release: `.github/workflows/release.yml` (goreleaser on v* tag push)
- Pre-push hook: `.githooks/pre-push` (gofmt + go vet + golangci-lint + go build)
- v0.3.0-alpha released on GitHub (6 binaries + checksums)
- Homebrew tap: `SirsiMaster/homebrew-tools` (repo exists, needs PAT)
- ADRs: 001 (founding), 002 (Ka), 003 (build-in-public), 004 (Ma'at), **005 (Pantheon)**

### Portfolio-Wide Pantheon Deployment (Session 9)
| Repo | Thoth Memory | GEMINI/CLAUDE | Portfolio Standard | Session Workflow |
|:-----|:------------|:-------------|:------------------|:----------------|
| sirsi-anubis | ✅ Full (133 lines) | ✅ | ✅ | ✅ |
| SirsiNexusApp | ✅ Real (85 lines) | ✅ | ✅ | ✅ |
| FinalWishes | ✅ Real (93 lines) | ✅ | ✅ | ✅ |
| assiduous | ✅ Real (66 lines) | ✅ | ✅ | ✅ |
| sirsi-thoth | ✅ Real (55 lines) | ✅ | ✅ | ✅ |

---

## What's Next

### Priority 1: Remaining Test Coverage
```
- Cleaner: 77% → 90% (safety-critical, Ma'at threshold = 80%)
- Ka: 42.7% → 60% (test Clean with real file cleanup)
- Scanner edge cases: permission errors, symlink loops
```

### Priority 2: Pre-Push Hooks for All Repos
```
- SirsiNexusApp: Node lint gate (eslint + prettier)
- FinalWishes: Node lint gate
- assiduous: Node lint gate (already has commit-msg hook)
- sirsi-thoth: Node lint gate (basic — node index.js --help)
```

### Priority 3: Homebrew Tap
```
- Create a GitHub PAT with repo:write scope for SirsiMaster/homebrew-tools
- Add it as HOMEBREW_TAP_TOKEN secret in sirsi-anubis settings
- Uncomment the brews section in .goreleaser.yaml
- Test with a new tag push
```

### Priority 4: Launch Execution
```
- Product Hunt submission (copy in docs/LAUNCH_COPY.md)
- Hacker News Show HN (copy in docs/LAUNCH_COPY.md)
- Investor demo rehearsal (script in docs/INVESTOR_DEMO.md)
- REFRAME: Pitch as "Pantheon" not just "Anubis" (per ADR-005)
```

### Priority 5: Pantheon Repo Setup
```
- Create github.com/SirsiMaster/sirsi-pantheon monorepo
- Add Anubis and Thoth as sub-modules or workspace references
- Pantheon CLI wrapper (optional: rename binary in goreleaser)
- pantheon.dev or sirsi.ai/pantheon web presence
```

### Priority 6: Missing Canon Documents
```
- SECURITY.md: FinalWishes, assiduous, sirsi-thoth
- CONTRIBUTING.md: FinalWishes, assiduous, sirsi-thoth
- CHANGELOG.md: SirsiNexusApp, assiduous, sirsi-thoth
- VERSION file: SirsiNexusApp, FinalWishes, sirsi-anubis
- ADRs: assiduous (0 ADRs), sirsi-thoth (0 ADRs)
```

---

## Key Context

1. **Pantheon is the product** (ADR-005) — not just Anubis. All deities are sub-systems.
2. **"Weigh. Judge. Purify."** — canonical tagline (for Anubis within Pantheon)
3. **Sirsi Portfolio Standard v2.0.0** — 26 universal rules deployed to all 5 repos
4. **Ma'at is operational** — `anubis maat` runs QA/QC governance assessments
5. **Thoth is live everywhere** — real memories in all 5 repos (not skeletons)
6. **ADR-003** — build-in-public is mandatory
7. **Rule A14** — every public number must be verifiable
8. **Rule A15** — a session = one AI conversation between continuation prompts
9. **Voice rule**: Never "the user wanted/suggested." Use direct verbs.
10. **April investor demos** — product must be complete by March 28
11. **v0.3.0-alpha is LIVE** — GitHub Release with 6 binaries
12. **Pre-push hook is active** — `.githooks/pre-push` gates every push

---

## Session 9 Completed Work (for context)

- ✅ Ma'at built — 57 tests, 4 source files, CLI command wired
- ✅ ADR-004 (Ma'at QA/QC Governance) canonized
- ✅ ADR-005 (Pantheon Unification) canonized
- ✅ Sirsi Portfolio Standard v2.0.0 — 26 universal rules
- ✅ Real Thoth memories deployed to all 5 repos (replacing skeletons)
- ✅ GEMINI.md + CLAUDE.md deployed to assiduous + sirsi-thoth
- ✅ Session workflows deployed to sirsi-thoth
- ✅ Pantheon coverage: ~20% → ~75% across portfolio
- ✅ All 5 repos committed and pushed, working trees clean
- ✅ 522 total tests, 20 modules, 18 commands, CI green

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
5. **ADR-005 is enforced** — Pantheon is the product. Think Pantheon, not just Anubis.
6. **Check actual struct field names** before using them.
7. **Binary size budget:** controller < 15 MB, agent < 5 MB.
8. **Monitor context** — report session metrics after every sprint. Wrap at 🔴.
9. **Voice**: Direct verbs only. No "the user wanted."
10. **Thoth manages the session** — memory for context, monitoring for health.
11. **Rule A14**: Include the command to reproduce any public number.
12. **Ma'at governs quality** — every feature must link to canon. No unjustified code.
13. **Portfolio Standard v2.0.0** — 26 rules apply universally. Read it.

---

## Start Command

```bash
cd /Users/thekryptodragon/Development/sirsi-anubis
cat .thoth/memory.yaml
go build ./cmd/anubis/ && go test ./... && echo "✓ Ready"
```

Then begin Priority 1: Coverage sprint (cleaner 77%→90%, ka 42.7%→60%).
