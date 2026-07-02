# Getting Started with Pantheon

Pantheon is a unified DevOps intelligence platform. One binary, 12 modules, zero telemetry.

## Install

```bash
# macOS / Linux (Homebrew)
brew tap SirsiMaster/tools
brew install sirsi-pantheon

# Or download from GitHub Releases
# https://github.com/SirsiMaster/sirsi-pantheon/releases
```

## Quick Start

The fastest way to get started is the interactive TUI:

```bash
sirsi                    # Opens the persistent TUI session
```

Once inside, type any command in plain English. Try `scan` to run a waste scan. After it completes, you'll see gold-highlighted "What's Next" suggestions — press Tab to cycle through them, or type your own command.

TUI state persists between sessions. Deity run outcomes (✓/✗/◆) are saved to `~/.config/pantheon/tui-state.json`.

### Standalone CLI commands

Every command also works directly from your shell:

```bash
# Scan your machine for infrastructure waste
sirsi scan

# Check system health
sirsi diagnose

# Audit network security
sirsi isis network

# See what hardware you're running
sirsi seba hardware

# Check for uncommitted work at risk
sirsi osiris assess
```

## The 12 Modules

| Command | Module | What It Does |
|---------|--------|--------------|
| `sirsi anubis` | Cleanup | Scans waste, cleans artifacts, hunts ghosts, deduplicates files |
| `sirsi isis` | Health | System health, network security, auto-remediation |
| `sirsi seba` | Hardware | Hardware profiling, architecture mapping, fleet discovery |
| `sirsi thoth` | Memory | Persistent AI memory for coding sessions |
| `sirsi maat` | Quality | Code quality governance, coverage audits |
| `sirsi seshat` | Knowledge | Knowledge ingestion from Chrome, Gemini, Claude, Notes |
| `sirsi ra` | Fleet | Cross-repo orchestration (requires claude-code-sdk) |
| `sirsi net` | Alignment | Scope definition and plan alignment |
| `sirsi osiris` | Recovery | Checkpoint assessment and risk scoring |
| `sirsi rtk` | Filter | Output filtering — strip ANSI, dedup, truncate for AI context |
| `sirsi vault` | Vault | Context sandbox — store large output in SQLite FTS5, search later |
| `sirsi horus` | Code Graph | Structural code graph — AST outlines, symbol extraction, context queries |

## Common Shortcuts

These top-level aliases skip the module prefix:

```bash
sirsi scan       # → anubis scan (find waste)
sirsi ghosts     # → anubis ghosts (find app remnants)
sirsi duplicates # → anubis dedup (find duplicate files)
sirsi monitor    # → anubis guard (resource monitor)
sirsi diagnose   # → isis diagnose (health diagnostic)
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

## Feature Guides

- [`sirsi clean` — reclaim storage, safely](clean.md)
- [`sirsi relieve` — hand the CPU (or memory) back](relieve.md)
- [`sirsi reclaim-snapshots` — free disk from local Time Machine snapshots](reclaim-snapshots.md)
- [`sirsi insight` — cross-deity state of the union](insight.md)
- [`sirsi vitals` — the fast read on memory right now](vitals.md)
- [`sirsi activity` — the trust ledger](activity.md)
