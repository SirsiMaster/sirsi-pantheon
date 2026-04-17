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
sirsi scan

# Check system health
sirsi doctor

# Audit network security
sirsi isis network

# See what hardware you're running
sirsi seba hardware

# Check for uncommitted work at risk
sirsi osiris assess

# Launch the interactive TUI
sirsi
```

## The 9 Deities

| Command | Deity | What It Does |
|---------|-------|--------------|
| `sirsi anubis` | Anubis | Scans waste, cleans artifacts, hunts ghosts, deduplicates files |
| `sirsi isis` | Isis | System health, network security, auto-remediation |
| `sirsi seba` | Seba | Hardware profiling, architecture mapping, fleet discovery |
| `sirsi thoth` | Thoth | Persistent AI memory for coding sessions |
| `sirsi maat` | Ma'at | Code quality governance, coverage audits |
| `sirsi seshat` | Seshat | Knowledge ingestion from Chrome, Gemini, Claude, Notes |
| `sirsi ra` | Ra | Cross-repo orchestration (requires claude-code-sdk) |
| `sirsi net` | Net | Scope definition and plan alignment |
| `sirsi osiris` | Osiris | Checkpoint assessment and risk scoring |

## Common Shortcuts

These top-level aliases skip the deity prefix:

```bash
sirsi scan       # → anubis weigh
sirsi ghosts     # → anubis ka
sirsi dedup      # → anubis mirror
sirsi guard      # → anubis guard (resource monitor)
sirsi doctor     # → isis doctor (health diagnostic)
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
sirsi mcp    # Start MCP server (configure in Claude/Cursor/VS Code)
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
