# Getting Started with Pantheon

Pantheon is a unified DevOps intelligence platform. One binary, 9 deities, zero telemetry.

## Install

```bash
# macOS / Linux (Homebrew)
brew tap SirsiMaster/tools
brew install sirsi-pantheon

# Or download from GitHub Releases
# https://github.com/SirsiMaster/sirsi-pantheon/releases
```

## Quick Start

```bash
# Scan your machine for infrastructure waste
pantheon scan

# Check system health
pantheon doctor

# Audit network security
pantheon isis network

# See what hardware you're running
pantheon seba hardware

# Check for uncommitted work at risk
pantheon osiris assess

# Launch the interactive TUI
pantheon
```

## The 9 Deities

| Command | Deity | What It Does |
|---------|-------|--------------|
| `pantheon anubis` | Anubis | Scans waste, cleans artifacts, hunts ghosts, deduplicates files |
| `pantheon isis` | Isis | System health, network security, auto-remediation |
| `pantheon seba` | Seba | Hardware profiling, architecture mapping, fleet discovery |
| `pantheon thoth` | Thoth | Persistent AI memory for coding sessions |
| `pantheon maat` | Ma'at | Code quality governance, coverage audits |
| `pantheon seshat` | Seshat | Knowledge ingestion from Chrome, Gemini, Claude, Notes |
| `pantheon ra` | Ra | Cross-repo orchestration (requires claude-code-sdk) |
| `pantheon net` | Net | Scope definition and plan alignment |
| `pantheon osiris` | Osiris | Checkpoint assessment and risk scoring |

## Common Shortcuts

These top-level aliases skip the deity prefix:

```bash
pantheon scan       # → anubis weigh
pantheon ghosts     # → anubis ka
pantheon dedup      # → anubis mirror
pantheon guard      # → anubis guard (resource monitor)
pantheon doctor     # → isis doctor (health diagnostic)
```

## Output Modes

Every command supports these global flags:

```bash
--json      # Machine-readable JSON output
--quiet     # Suppress all output except errors
-v          # Verbose debug logging
```

## IDE Integration (MCP)

Pantheon exposes scanning, memory, and diagnostics as MCP tools for AI IDEs:

```bash
pantheon mcp    # Start MCP server (configure in Claude/Cursor/VS Code)
```

## Per-Deity Guides

- [Anubis — Hygiene Engine](anubis.md)
- [Isis — Health & Remedy](isis.md)
- [Seba — Infrastructure & Hardware](seba.md)
- [Thoth — Session Memory](thoth.md)
- [Ma'at — Quality Gate](maat.md)
- [Seshat — Knowledge Bridge](seshat.md)
- [Ra — Agent Orchestrator](ra.md)
- [Net — Scope Weaver](net.md)
- [Osiris — Snapshot Keeper](osiris.md)
