# ‍‍‍𓂀 Sirsi Anubis — Continuation Prompt
**Date:** March 23, 2026 (Sunday, 4:08 AM ET)
**Session:** Platform Wiring + CI Fix + Pipeline Governance
**Repo:** `github.com/SirsiMaster/sirsi-anubis`
**Path:** `/Users/thekryptodragon/Development/sirsi-anubis`
**CI Status:** ✅ Green (first passing run after 5 consecutive failures)

---

## CRITICAL: Read Before Starting

1. **Run `/session-start`** — the Thoth workflow at `.agent/workflows/session-start.md`
2. **Read `.thoth/memory.yaml`** — compressed project state (~125 lines). This replaces reading source files.
3. **Read `.thoth/journal.md`** — timestamped reasoning (12 entries).
4. **Read `ANUBIS_RULES.md`** — the 15 non-negotiable safety rules (includes A14, A15).
5. **Deadline: Friday March 28** — April investor demos require complete product.
6. **All code compiles and 470 tests pass** — do NOT break the build.
7. **ADR-003 is ACTIVE** — every release must update BUILD_LOG.md, build-log.html, CHANGELOG, Thoth.
8. **Rule A14 (Statistics Integrity)** — every public number must be independently verifiable.
9. **Rule A15 (Session Definition)** — a session = one AI conversation between continuation prompts.
10. **Pre-push hook is ACTIVE** — `.githooks/pre-push` runs golangci-lint before every push. Do NOT skip it.

---

## 𓁟 Thoth — Session Management

Thoth is the project's persistent knowledge system. Two responsibilities:

### 1. Project Memory (Read at start, update at end)
| Layer | File | When |
|:------|:-----|:-----|
| Memory | `.thoth/memory.yaml` | **ALWAYS first** — architecture, decisions, limitations |
| Journal | `.thoth/journal.md` | When WHY matters — 12 timestamped entries |
| Artifacts | `.thoth/artifacts/` | Deep dives — benchmarks, audits, **roi-metrics.md** |

### 2. Context Window Monitoring (Track throughout session)

After every sprint, report session metrics per the template in `.thoth/memory.yaml`.
Heuristics: Turns 1-5 ~10-20%, Turns 5-15 ~20-60%, Turns 15-25 ~60-85%, Turns 25+ >85%.
If truncation detected, wrap immediately.

---

## What Exists Right Now (All Working)

### Core Modules (19 internal packages)
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
| **Logging** | `internal/logging/` | **slog-based structured logging (NEW)** |
| **Platform** | `internal/platform/` | **OS abstraction: Darwin, Linux, Mock (NEW)** |
| Mapper | `internal/mapper/` | Filesystem mapper (no tests) |
| Output | `internal/output/` | Terminal rendering (no tests) |
| Updater | `internal/updater/` | Version check + advisory system |

### CLI Commands (17)
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

### Global Flags (NEW)
- `--json` — JSON output
- `--quiet` — suppress non-error output
- `--verbose` / `-v` — enable debug logging (slog to stderr)
- `--stealth` — ephemeral mode

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
| **logging** | **6** | **Level modes (NEW)** |
| mcp | 5 | Server lifecycle |
| mirror | 12 | Dedup engine |
| **platform** | **11** | **All implementations (NEW)** |
| profile | 16 | Scan profiles |
| scales | varies | Policy engine |
| scarab | 18 | Network + ARP parsing |
| sight | 4 | LaunchServices |
| stealth | 9 | Cleanup engine |
| **Total** | **470** | **17 suites** |

### Infrastructure
- CI: `.github/workflows/ci.yml` (lint + test + build)
- Release: `.github/workflows/release.yml` (goreleaser on v* tag push)
- **Pre-push hook**: `.githooks/pre-push` (gofmt + go vet + golangci-lint + go build)
- v0.3.0-alpha released on GitHub (6 binaries + checksums)
- Homebrew tap: `SirsiMaster/homebrew-tools` (repo exists, needs PAT)
- VS Code extension scaffold: `extensions/vscode/`
- ADRs: 001 (founding), 002 (Ka ghost detection), 003 (build-in-public)

### Case Studies (3 verified)
- `docs/case-studies/thoth-context-savings.md` — 98.7% context reduction
- `docs/case-studies/mirror-dedup-performance.md` — 27.3x faster, 98.8% less I/O
- `docs/case-studies/ka-ghost-detection.md` — 5-step algorithm, 17 locations

### Sirsi Pantheon (Repos)
| Repo | Deity | Version |
|:-----|:------|:--------|
| `sirsi-anubis` | 𓂀 Anubis | v0.3.0-alpha |
| `sirsi-thoth` | 𓁟 Thoth | v1.0.0 |
| `SirsiNexusApp` | ☀️ Ra (coming) | In development |

---

## What's Next

### Priority 1: ✅ DONE — Platform Interface Wired (Session 8)
Completed in session 8. Cleaner and mirror now use `platform.Current()` instead of `runtime.GOOS`.

### Priority 2: Homebrew Tap
```
- Create a GitHub PAT with repo:write scope for SirsiMaster/homebrew-tools
- Add it as HOMEBREW_TAP_TOKEN secret in sirsi-anubis settings
- Uncomment the brews section in .goreleaser.yaml
- Test with a new tag push
```

### Priority 3: Remaining Coverage
```
- Cleaner: 77% → 90% (safety-critical module)
- Ka: 42.7% → 60% (test Clean with real file cleanup)
- Scanner edge cases: permission errors, symlink loops
```

### Priority 4: Anubis Maat — Pipeline Purifier (NEW)
```
- internal/maat/monitor.go — poll gh run list for failures
- internal/maat/diagnose.go — parse failure logs, categorize errors
- internal/maat/fix.go — auto-fix lint (gofmt, misspell, goimports)
- internal/maat/report.go — format actionable reports
- cmd/anubis/maat.go — CLI command
- Modes: --check (diagnose), --fix (auto-remediate), --watch (daemon)
```

### Priority 5: Launch Execution
```
- Product Hunt submission (copy in docs/LAUNCH_COPY.md)
- Hacker News Show HN (copy in docs/LAUNCH_COPY.md)
- Investor demo rehearsal (script in docs/INVESTOR_DEMO.md)
```

### Priority 6: Production Polish
```
- Convert pitch deck stub to full HTML slide
- VS Code extension completion
- npm publish thoth-init
```

---

## Key Context

1. **"Weigh. Judge. Purify."** — canonical tagline
2. **Sirsi Pantheon** — Egyptian-themed tools: Anubis, Thoth, Ka, Ra, Seba, Hapi, Scarab
3. **Thoth is independent** — standalone repo, works without Anubis or MCP
4. **ADR-003** — build-in-public is mandatory
5. **Rule A14** — every public number must be verifiable. No projections as measurements.
6. **Rule A15** — a session = one AI conversation between continuation prompts.
7. **Voice rule**: Never "the user wanted/suggested." Use direct verbs.
8. **April investor demos** — product must be complete by March 28
9. **v0.3.0-alpha is LIVE** — GitHub Release with 6 binaries

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
7. **Monitor context** — report session metrics after every sprint. Wrap at 🔴.
8. **Voice**: Direct verbs only. No "the user wanted."
9. **Thoth manages the session** — memory for context, monitoring for health.
10. **Rule A14**: Include the command to reproduce any public number.

---

## Start Command

```bash
cd /Users/thekryptodragon/Development/sirsi-anubis
cat .thoth/memory.yaml
go build ./cmd/anubis/ && go test ./... && echo "✓ Ready"
```

Then begin Priority 1: Wire Platform interface into cleaner module.
