# 𓂀 Sirsi Anubis — Continuation Prompt
**Date:** March 21, 2026 (Friday, 4:27 PM ET)
**Session:** Ship Week Day 5–8 Continuation
**Repo:** `github.com/SirsiMaster/sirsi-anubis`
**Path:** `/Users/thekryptodragon/Development/sirsi-anubis`

---

## CRITICAL: Read Before Starting

1. **Read `docs/DEVELOPMENT_PLAN.md`** — the canonical 8-day ship week schedule
2. **Read `ANUBIS_RULES.md`** — the 12 non-negotiable safety rules
3. **Scope is LOCKED** — no new features. Only Days 5–8 remain
4. **Deadline: Friday March 28** — April investor demos require complete product
5. **All code compiles and tests pass** — do NOT break the build

---

## What Exists Right Now (All Working, All Tested)

### Binary
- **Size:** 7.5 MB (macOS arm64)
- **Architecture:** Go 1.22+, Cobra CLI, lipgloss terminal UI
- **Build:** `go build -ldflags="-s -w -X main.version=0.2.0-alpha" -o anubis ./cmd/anubis/`

### 14 CLI Commands (all compiled and running)

| Command | Module | Description | Status |
|:--------|:-------|:-----------|:-------|
| `anubis version` | updater | Version + platform + phone-home update check | ✅ |
| `anubis weigh` | jackal | Scan workstation (64 rules, found 69 GB) | ✅ |
| `anubis judge` | cleaner | Clean artifacts with --dry-run/--confirm/--trash | ✅ |
| `anubis ka` | ka | Ghost app hunter (found 130 ghosts on dev machine) | ✅ |
| `anubis guard` | guard | RAM audit + process slayer (found 1.4 GB orphans) | ✅ |
| `anubis sight` | sight | Launch Services ghost fix + Spotlight reindex | ✅ |
| `anubis profile` | profile | 4 scan profiles (general, developer, ai-engineer, devops) | ✅ |
| `anubis seba` | mapper | Interactive WebGL infrastructure graph (64 nodes) | ✅ |
| `anubis hapi` | hapi | GPU detection (M1 Max/Metal 4/Neural Engine), dedup, snapshots | ✅ |
| `anubis hapi --gpu` | hapi | Hardware detection only | ✅ |
| `anubis hapi --dedup` | hapi | Duplicate file detection (SHA-256, parallel) | ✅ |
| `anubis hapi --snapshots` | hapi | APFS/Time Machine snapshot listing | ✅ |
| `anubis scarab` | scarab | Network discovery (ARP + ping sweep, found 16+ hosts) | ✅ |
| `anubis scarab --containers` | scarab | Docker container audit | ✅ |
| `anubis book-of-the-dead` | (hidden) | 7-chapter system autopsy — HEAVY/HAUNTED/RESTLESS verdicts | ✅ |
| `anubis initiate` | (cli) | macOS permission wizard (Full Disk Access) | ✅ |
| `--stealth` | stealth | Ephemeral mode — delete all Anubis data after execution | ✅ |
| `--json` | (global) | JSON output for all commands | ✅ |
| `--quiet` | (global) | Suppress output except errors/summary | ✅ |

### 13 Internal Modules

| Module | Path | Tests | Description |
|:-------|:-----|:------|:-----------|
| jackal | `internal/jackal/` | ✅ 93% | Scan engine, ScanRule interface, concurrent orchestration |
| cleaner | `internal/cleaner/` | ✅ 49% | File deletion with safety, trash on macOS |
| ka | `internal/ka/` | ✅ 19.5% | Ghost app detection, Launch Services parsing |
| guard | `internal/guard/` | ✅ 42 tests | RAM audit, process grouping, orphan detection, slayer |
| sight | `internal/sight/` | — | Launch Services rebuild, Spotlight reindex |
| profile | `internal/profile/` | — | YAML scan profiles, ~/.config/anubis/ |
| hapi | `internal/hapi/` | — | GPU detection, dedup, APFS snapshots |
| scarab | `internal/scarab/` | — | Network discovery, Docker container audit |
| mapper | `internal/mapper/` | — | Sigma.js graph generation, self-contained HTML |
| updater | `internal/updater/` | — | GitHub Releases API phone-home, ADVISORY.json |
| stealth | `internal/stealth/` | — | Ephemeral mode cleanup |
| ignore | `internal/ignore/` | — | .anubisignore gitignore-style path exclusion |
| output | `internal/output/` | — | Terminal UI (lipgloss), Banner, headers |

### 64 Scan Rules (7 categories)
Already registered in `internal/jackal/rules/registry.go`:
- General Mac (9), Virtualization (4), Dev Frameworks (10)
- AI/ML (11), IDEs & AI Tools (11), Cloud & Infra (6), Cloud Storage (4)

### Infrastructure Already Shipped
- `.github/workflows/ci.yml` — CI pipeline (go test, vet, gofmt, lint)
- `.github/workflows/release.yml` — goreleaser on v* tag push
- `.github/ISSUE_TEMPLATE/` — bug report, feature request, security vuln
- `.goreleaser.yml` — multi-platform binaries + Homebrew formula
- `ADVISORY.json` — post-release roadblock notifications
- `docs/` — ADR-001, ADR-002, CONTRIBUTING.md, SECURITY.md, SAFETY_DESIGN.md, SCAN_RULE_GUIDE.md
- `configs/default_rules.yaml` — rule configuration
- Git topics: cli, golang, devtools, infrastructure, cleanup, macos, linux, ghost-files, developer-tools, open-source
- GitHub Discussions enabled

### Product Tiers
```
Anubis Free (OSS, MIT)  →  Anubis Pro (Neural)  →  Eye of Horus (Subnet)  →  Ra (Enterprise/Sirsi)
    7.5 MB binary            +install-brain           +agents                    +dashboard
    CLI only                  +CoreML/ONNX            +VLAN sweep               +policy engine
```

---

## WHAT TO BUILD: Days 5–8

### Day 5: Neural Brain Downloader (Tuesday March 25)
```
internal/brain/downloader.go  — on-demand model fetcher
  - Download CoreML/ONNX model to ~/.anubis/weights/
  - Progress bar, checksum verification, version management
  - Size budget: < 100 MB quantized model

internal/brain/inference.go   — model inference wrapper
  - ONNX Runtime Go bindings (ort-go)
  - CoreML bridge via CGO (macOS)
  - CPU fallback for cross-platform
  - Batch inference for file classification

cmd/anubis/brain.go           — CLI commands
  - anubis install-brain          (install default model)
  - anubis install-brain --remove (self-delete weights)
  - anubis install-brain --update (fetch latest)
  - anubis uninstall-brain        (alias for --remove)
```

### Day 6: MCP Server + IDE Integrations (Wednesday March 26)
```
internal/mcp/server.go        — MCP (Model Context Protocol) server
  - Anubis as context sanitizer for Claude/Cursor/Windsurf
  - Tools: scan_workspace, clean_workspace, ghost_report
  - Resources: scan results, ghost list, health status
  - Runs as local stdio server

cmd/anubis/mcp.go             — CLI command
  - anubis mcp                   (start MCP server mode)

extensions/vscode/             — VS Code extension scaffold
  - Extension manifest (package.json)
  - "Eye of Horus" sidebar health meter concept
  - Status bar icon

.anubis/ workspace config      — per-project configuration
```

### Day 7: Scales Policy Engine (Thursday March 27)
```
internal/scales/policy.go      — YAML policy parser
  - Policy definitions for scan thresholds
  - Auto-remediation rules (with approval)
  - Notification targets (Slack, Teams, webhook)

internal/scales/enforce.go     — policy enforcement
  - Evaluate scan results against policies
  - Generate verdicts (pass/warn/fail)

cmd/anubis/scales.go           — CLI command
  - anubis scales enforce         (run policies)
  - anubis scales validate        (check syntax)
  - anubis scales verdicts        (show results)

Agent hardening:
  - cmd/anubis-agent/ — implement scan/report/clean
  - Fixed command set (no shell access)
  - JSON stdout for controller
```

### Day 8: Polish + Release (Friday March 28)
```
- Update README.md — accurate feature list, ALL commands documented
- Update CHANGELOG.md — complete v0.2.0-alpha entry
- Update VERSION file to 0.2.0-alpha
- Final test suite (target: 70%+ core coverage)
- Binary size audit (< 15 MB controller, < 5 MB agent)
- gofmt + go vet + golangci-lint clean
- Tag v0.2.0-alpha
- goreleaser snapshot build (verify all platforms)
- GitHub Release draft
- Product Hunt / Hacker News launch copy draft
- Investor demo script update (5-minute walkthrough)
```

---

## Key Context from Google AI Brainstorm Session

The user had a conversation with Google AI Mode that shaped several product decisions:

1. **"Scratch your own itch" origin** — Anubis was born from frustration with CleanMyMac and Mole missing developer-specific junk (ghost files, broken symlinks, orphaned .plist files)
2. **Anubis = public scout, Sirsi = private commander** — value funnel from free tool to enterprise
3. **Egyptian/Kemetic branding is intentional** — Ka, Hapi, Scarab, Seba, Book of the Dead, Ma'at references
4. **GPU/Neural Engine integration** — user wants to max out on ANE/tensor core implementations for semantic cleanup
5. **"Context Sanitizer for AI era"** — Anubis cleans context before AI agents index code
6. **"Zero footprint" narrative** — --stealth mode, anubis comes judges and vanishes
7. **April investor timing** — Product Hunt/HN launch planned alongside investor meetings
8. **IDE integrations are high priority** — MCP server for Claude/Cursor/Windsurf, VS Code extension
9. **Seba (𓇼)** — user chose this name for the graph mapper ("star" and "gateway" in Egyptian)
10. **Install-brain concept** — on-demand neural weight download keeps base binary light

---

## Dev Machine Specs (Live Detected by Anubis)

- **CPU:** Apple M1 Max (10 cores)
- **GPU:** Apple M1 Max (32 cores, Metal 4)
- **Neural Engine:** ✅ Available
- **RAM:** 32 GB unified memory
- **Disk:** 926 GB (3% used)
- **Network:** 192.168.12.0/24, 16+ hosts discovered
- **Local IP:** 192.168.12.113
- **Hostname:** Cylton.local

---

## Git History (Recent)

```
7501943 feat(scarab): Day 4 — network discovery + container audit
72f430f feat(hapi): Day 3 — resource optimizer with GPU detection, dedup, snapshots
fb1cf50 feat(day2): Book of the Dead, Initiate, Stealth, .anubisignore
461bba9 feat(seba): 𓇼 infrastructure graph visualization + phone home + issue templates
9615065 feat(infra): phone home, issue templates, release pipeline
7bafa67 docs(plan): v2.0.0 — ship week schedule March 21-28
da15cee feat(dist): Sprint 1.7 — goreleaser config, SCAN_RULE_GUIDE, binary polish
d3ddbcd feat(profile): Sprint 1.6 — scan profiles + config system
f89512a feat(sight): Sprint 1.5 — Spotlight ghost detection + Launch Services rebuild
dac007c feat(rules): Sprint 1.4 — scan rule expansion 34 → 64 rules
```

---

## Rules of Engagement

1. **Scope is LOCKED** — Days 5, 6, 7, 8 only. No new features.
2. **Build → Test → Commit → Push** after every feature.
3. **Never break the build** — `go build && go test ./... && go vet ./...` must pass.
4. **gofmt before every commit** — `gofmt -w $(find . -name '*.go' -not -path './.git/*')`
5. **Check actual struct field names** before using them (past sessions had bugs from assumed field names).
6. **Book of the Dead stays Hidden: true** — initiates only.
7. **Binary size budget:** controller < 15 MB, agent < 5 MB.
8. **All commands need --json support** for IDE/agent consumption.
9. **Eye of Horus / Sirsi upsell footers** on network/fleet features.
10. **Context monitoring** — save context for continuation prompts at 90%.

---

## Start Command

```bash
cd /Users/thekryptodragon/Development/sirsi-anubis
git pull origin main
go build ./cmd/anubis/ && go test ./... && echo "✓ Ready"
```

Then begin Day 5: `internal/brain/downloader.go`
