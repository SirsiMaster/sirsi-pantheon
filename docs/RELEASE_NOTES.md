# Sirsi Pantheon v0.9.0-rc1 Release Notes

**Date:** March 31, 2026
**Status:** Release Candidate
**Platform:** macOS Apple Silicon (primary), Linux, Windows (CLI only)

---

## What is Pantheon?

Pantheon is a unified DevOps intelligence CLI. One binary, all deities. It scans, judges, and purges infrastructure waste across workstations, containers, and development environments.

```bash
brew tap SirsiMaster/tools && brew install sirsi-pantheon
# or
go install github.com/SirsiMaster/sirsi-pantheon/cmd/sirsi@v0.8.0-beta
```

## Deity Pillars

| Deity | Glyph | What It Does |
|-------|-------|-------------|
| **Anubis** | 𓁢 | Infrastructure hygiene — scan, judge, purge waste |
| **Ma'at** | 𓆄 | Governance — test coverage, lint, policy enforcement |
| **Thoth** | 𓁟 | Project memory — cross-session knowledge for AI workflows |
| **Seba** | 𓇽 | Mapping — Mermaid diagrams, hardware detection |
| **Seshat** | 𓁆 | Knowledge export — MCP bridge for AI IDEs |

## Quick Start

```bash
# Scan your machine for waste
sirsi anubis weigh

# Clean up (dry-run first, always)
sirsi anubis judge --dry-run

# Run governance audit
sirsi maat audit

# Fast audit (cached coverage, instant)
sirsi maat audit --skip-test

# Initialize project memory
sirsi thoth init --yes .

# Find duplicate files
sirsi mirror ~/Downloads

# Check what deities are available
sirsi --help
```

## Key Features

### Anubis — Infrastructure Hygiene
- **64 scan rules** covering caches, logs, AI models, IDE data, cloud tools, VMs
- **Safety system** with 29 protected paths — never deletes system files
- **Trash-first cleaning** — moves to trash by default, permanent delete opt-in
- **Mirror dedup** — finds duplicate files with 27x partial hash speedup
- **Ka ghost hunter** — detects dead app remnants on macOS
- **Guard watchdog** — monitors RAM pressure and zombie processes

### Ma'at — Governance
- **Streaming audit** — per-package test results as they happen
- **Dynamic module discovery** — automatically measures all internal packages
- **Scales policy engine** — YAML-based infrastructure policies
- **Isis remediation** — auto-fix lint, formatting, coverage gaps

### Thoth — Project Memory
- **3-layer knowledge**: `memory.yaml` (identity) -> `journal.md` (history) -> `artifacts/` (exports)
- **Auto-detection**: Go, TypeScript, Next.js, Rust, Python projects
- **AI workflow integration**: designed for cross-session context with Claude, Gemini, Cursor

### MCP Server
- `sirsi mcp` exposes tools via Model Context Protocol
- Works with Claude Code, Cursor, Windsurf, and any MCP-compatible IDE
- Tools: `scan_workspace`, `ghost_report`, `health_check`, `thoth_read_memory`, `classify_files`

## Installation

### Homebrew (recommended)
```bash
brew tap SirsiMaster/tools
brew install sirsi-pantheon
```

### Go Install
```bash
go install github.com/SirsiMaster/sirsi-pantheon/cmd/sirsi@v0.8.0-beta
```

### Binary Download
Download from [GitHub Releases](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.8.0-beta).

Available for: macOS (arm64/amd64), Linux (arm64/amd64), Windows (amd64/arm64).

### Individual Deities
Each deity is also available as a standalone binary:
```bash
go install github.com/SirsiMaster/sirsi-pantheon/cmd/anubis@v0.8.0-beta
go install github.com/SirsiMaster/sirsi-pantheon/cmd/maat@v0.8.0-beta
go install github.com/SirsiMaster/sirsi-pantheon/cmd/thoth@v0.8.0-beta
```

## Known Limitations

- **macOS-first**: Ghost detection (Ka) and some scan rules are macOS-only
- **No GUI**: CLI only in this release. macOS GUI and cross-platform GUI planned for v1.0
- **Neith stub**: Orchestration deity is deferred to v1.0
- **Ra not started**: Web portal (inside SirsiNexusApp) is a future goal

## Verified Metrics

All numbers verified by `go test -cover ./...` on March 31, 2026:

- 1,500+ tests passing across 28 packages
- ~85% weighted test coverage
- 0 lint errors (`golangci-lint run ./...`)
- 64 scan rules across 7 categories
- 27 internal modules
- ~12 MB compiled binary

## What's Next

v1.0.0-rc1 will be **earned through dogfooding**, not declared:
1. 30-day production use on real machines
2. Cross-platform testing (Linux, Windows)
3. Neith orchestration
4. MCP plugin for Claude Code users
5. macOS GUI (menubar app)

---

*Building in public. The feather weighs true. No excuses.*

**GitHub:** https://github.com/SirsiMaster/sirsi-pantheon
**Web:** https://sirsi.ai/pantheon
