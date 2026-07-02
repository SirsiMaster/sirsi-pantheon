# Sirsi Pantheon

**Infrastructure intelligence for developers and operations teams.** Scans, detects, and remediates — from one laptop to a 256-node fleet. 81 rules, zero config, zero telemetry. Works with or without AI.

[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-C8A951?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.23.8--beta-1A1A5E?style=flat)](VERSION)
[![Tests](https://img.shields.io/badge/tests-2%2C000%2B%20passing-brightgreen?style=flat)](.github/workflows/ci.yml)
[![Product Page](https://img.shields.io/badge/sirsi.ai-pantheon-059669?style=flat)](https://sirsi.ai/pantheon)

---

## Install

**CLI tools** (macOS, Linux, Windows):
```bash
brew tap SirsiMaster/tools && brew install sirsi-pantheon
```

**Menu bar app** (macOS — persistent monitoring, click to open the browser dashboard):
```bash
brew install --cask sirsi-pantheon
```

Or download from [GitHub Releases](https://github.com/SirsiMaster/sirsi-pantheon/releases), or build from source with `go build ./cmd/sirsi/`.

---

## Quick Start

```bash
sirsi setup                     # Checks dependencies and Full Disk Access
sirsi                           # Prints help (per-verb commands below)
```

On macOS, `sirsi setup` tells you whether Full Disk Access is granted. If it is missing, run `sirsi permissions` to open System Settings, then add the `sirsi` binary and your terminal app under Privacy & Security → Full Disk Access.

Run standalone commands directly:

```bash
sirsi scan       # Find waste — caches, build artifacts, orphaned files (81 rules)
sirsi diagnose   # System health — RAM pressure, disk space, kernel panics
sirsi ghosts     # Find remnants of apps you already uninstalled
```

> **Interactive surface — under redesign.** v0.22 shipped a BubbleTea TUI that was unreleasable and was removed in v0.23 (ADR-018). A new Mole-grade TUI is in design under [ADR-020](docs/ADR-020-INTERACTIVE-SURFACE-REOPENED.md) (Hybrid C: TUI first cross-platform, Mac native later). Until it ships, `sirsi` with no args prints help; the browser dashboard at `localhost:9119` is the visual surface today.

Every user-facing command ends with a short summary, evidence counts, warnings when needed, and a "What's Next" section. Use `--json` for scripts or `--quiet` for minimal output.

Want a guided walkthrough? Run `sirsi quickstart` for your first scan with recommendations.

---

## How It Works

Pantheon ships three surfaces in v0.23, with a new interactive TUI in design:

**Menu bar (always on)** — An ankh icon sits in your macOS menu bar. It runs a guard watchdog, periodic infrastructure scan, and shows live state: 🟢 Clean / 🟡 12 GB waste / 🔴 RAM pressure / ⚠️ process alert. Click to open the browser dashboard.

**CLI (scriptable)** — Every command works standalone: `sirsi scan`, `sirsi ghosts`, etc. All support `--json` for piping and automation. This is the primary v0.23 surface on every platform.

**Horus Dashboard (web)** — `sirsi dashboard` opens a terminal-first web app at localhost:9119 with API endpoints, SSE streaming, and a command bar. `sirsi horus` is the structural code graph. Optional for power users.

**Interactive TUI (in design — ADR-020 / Hybrid C)** — A new Mole-grade operator console is the next interactive surface. v0.22's BubbleTea TUI was removed in v0.23 because it was unreleasable; the new TUI is a clean-sheet redesign under [ADR-020](docs/ADR-020-INTERACTIVE-SURFACE-REOPENED.md) and ships cross-platform first, with native Mac SwiftUI following in a later phase. No code lands before a `docs/TUI_DESIGN_PROOF.md` clears review.

**Shared suggestion engine** — `internal/suggest/` is the action recommendation engine. After any command completes, it computes contextual next steps based on the command's output and current system state.

**Knowledge Substrate (𓅓)** — A semantic graph of your repo, auto-derived from source via the Understand-Anything plugin. Every file, function, class, import, and architectural layer is a node; relationships are edges. Stored as portable JSON at `.understand-anything/knowledge-graph.json`. Today's per-repo capability is the feeder for the future **Sirsi hypergraph** — a cross-repo, cross-agent knowledge layer built on Hedera Consensus Service. See [ADR-019](docs/ADR-019-KNOWLEDGE-SUBSTRATE.md) for the decision, the [knowledge-substrate user guide](docs/user-guides/knowledge-substrate.md) for usage, and `~/Development/HYPERGRAPH_VISION.md` for the long-term direction.

---

## All Commands

| Command | What It Does |
|:--------|:-------------|
| `sirsi scan` | Find infrastructure waste (81 rules, 7 domains) |
| `sirsi ghosts` | Detect remnants of uninstalled apps |
| `sirsi dedup [dirs]` | Find duplicate files with three-phase hashing |
| `sirsi diagnose` | One-shot system health diagnostic |
| `sirsi network` | Network security audit (DNS, WiFi, TLS, firewall, VPN) |
| `sirsi network --fix` | Auto-apply encrypted DNS + firewall with safety rollback |
| `sirsi hardware` | CPU, GPU, RAM, Neural Engine detection |
| `sirsi guard` | Real-time resource monitoring |
| `sirsi quality` | Code governance audit |
| `sirsi thoth init/sync` | AI project memory ([MCP](https://modelcontextprotocol.io)-compatible) |
| `sirsi hypergraph status` | Semantic graph of the repo — feeds the future Sirsi hypergraph on Hedera ([ADR-019](docs/ADR-019-KNOWLEDGE-SUBSTRATE.md), [user guide](docs/user-guides/knowledge-substrate.md)) |
| `sirsi mcp` | MCP server for Claude, Cursor, Windsurf |
| `sirsi agent preflight/safe-run` | Guard AI agent work with resource checks and output budgets |
| `sirsi seshat ingest` | Knowledge ingestion from Chrome, Gemini, Apple Notes |
| `sirsi diagram` | Generate architecture diagrams (Mermaid/HTML) |
| `sirsi work` | Workstream manager — launch AI sessions across projects |
| `sirsi quickstart` | Guided first scan with recommendations |

Every command supports `--json`, `--quiet`, and `--verbose` flags.

---

## What Makes Pantheon Different

**Works at every scale.** Same tool on one Mac, across a dev team, or orchestrating a 256-node fleet. Scan one machine or scan them all. No separate "enterprise edition" with different commands — same binary, same rules, more reach.

**Ghost detection that nobody else does.** `sirsi ghosts` finds Launch Services phantoms, orphaned plists, leftover caches, and Spotlight metadata for apps that no longer exist. Typically recovers 10–100 GB per machine. [Case study →](docs/case-studies/docker-ghost-64gb.md)

**Network security with auto-revert.** `sirsi network --fix` applies encrypted DNS and firewall hardening using the same probe→mutate→verify→revert pattern as Kubernetes rolling deploys. Auto-reverts within 6 seconds if anything breaks. [Case study →](docs/case-studies/isis-dns-safety-rollback.md)

**No AI required.** Every scan, diagnostic, and remediation works without an AI assistant. If you do use AI coding tools, Pantheon adds persistent project memory (Thoth), token optimization (RTK, Vault, Horus), and an MCP server — but the foundation is pure infrastructure.

### Where This Is Going

The same scanning architecture — 81 rules, hardware detection, policy enforcement — scales to fleets. **Pantheon Ra** extends the free tool to multi-machine orchestration: subnet scanning, container auditing, and autonomous agents that detect and fix issues across every node.

---

## Compatibility

| | Supported |
|:--|:----------|
| **AI Assistants** | Claude, Gemini, Codex (via MCP) |
| **IDEs** | VS Code, Cursor, Windsurf, Zed |
| **Platforms** | macOS, Linux, Windows |
| **Architecture** | Apple Silicon, ARM, Intel x86 |

---

## Scale

| | What | Example |
|:--|:------|:--------|
| **One machine** | Full CLI + menu bar + browser dashboard. All 81 rules, all deities. (New TUI in design per ADR-020.) | Solo developer cleaning up their Mac |
| **Small team** | Same tool, shared policies via `configs/`. Quality gates on every push. | 3-person startup with shared scan rules |
| **Fleet (Ra)** | Multi-machine orchestration, subnet scanning, autonomous remediation. | 256-node cluster with cross-repo AI agents |

One binary at every tier. No feature gating, no telemetry, no time limits. Apache 2.0 licensed.

---

## Security & Privacy

- **Zero telemetry.** No analytics, no tracking, no data leaves your machine. Non-negotiable.
- **Dry-run by default.** Every destructive operation requires explicit `--confirm`.
- **25 protected paths.** System directories, keychains, and SSH keys are hardcoded as undeletable.
- **Trash-first cleaning.** Removals go to Trash with a full decision log.
- **DNS safety model.** Network fixes probe before changing config, auto-revert on failure.

---

## Development

```bash
git clone https://github.com/SirsiMaster/sirsi-pantheon.git
cd sirsi-pantheon
git config core.hooksPath .githooks    # Enable pre-push quality gate
go test ./...                          # 2,251 tests across 36 packages
go build ./cmd/sirsi/
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## Built by Sirsi Technologies

[sirsi.ai](https://sirsi.ai) · [GitHub](https://github.com/SirsiMaster) · [Pantheon Hub](https://sirsi.ai/pantheon)

Apache License 2.0 — free and open source forever.
