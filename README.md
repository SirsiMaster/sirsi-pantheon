# Sirsi Pantheon

**Infrastructure hygiene for your dev machine.** Finds waste that CleanMyMac misses, audits network security, and gives your AI tools persistent memory.

[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-C8A951?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.15.0-1A1A5E?style=flat)](VERSION)
[![Tests](https://img.shields.io/badge/tests-1%2C663%20passing-brightgreen?style=flat)](.github/workflows/ci.yml)

```bash
brew tap SirsiMaster/tools && brew install sirsi-pantheon
```

---

## Why Pantheon Exists

Developer machines accumulate waste — orphaned caches, ghost app remnants, insecure DNS configs, build artifacts from frameworks you stopped using months ago. Existing tools either miss the hard stuff (Launch Services phantoms, Spotlight ghosts, captive portal DNS traps) or require you to trust a closed-source app with root access to your filesystem.

Pantheon is different in three ways:

**1. Ghost detection that nobody else does.** `sirsi ghosts` finds remnants of apps you uninstalled — Launch Services phantoms, orphaned plists, leftover caches, Spotlight metadata for apps that no longer exist. CleanMyMac finds caches. AppCleaner catches most files at uninstall time. Neither finds the ghosts that accumulate over months.

**2. Network security with a safety model borrowed from Kubernetes.** `sirsi network --fix` applies encrypted DNS and firewall hardening, but it probes the target with a raw TCP connect before changing anything, polls resolution after, and auto-reverts within 6 seconds if anything breaks. This is the same probe→mutate→verify→revert pattern that Kubernetes uses for rolling deployments and Terraform uses for infrastructure changes. We built it after the tool [bricked internet on airline WiFi](docs/case-studies/isis-dns-safety-rollback.md) — the safety model exists because we learned the hard way.

**3. AI memory as standard infrastructure.** `sirsi thoth` gives AI coding sessions persistent memory via the [Model Context Protocol](https://modelcontextprotocol.io). Instead of re-reading 22,958 lines of source every session, the AI reads 297 lines of compressed project context. This isn't a plugin — it's built into the tool, syncs from git history, and works with Claude, Cursor, and Windsurf out of the box.

### Where This Is Going

Pantheon runs on one machine today. The same scanning architecture — 58 rules, hardware detection, policy enforcement, event ledger — is designed to scale across fleets. **Pantheon Ra** extends the free tool to multi-machine orchestration: subnet scanning, container auditing, cross-repo AI agents that don't just collect metrics but autonomously detect and fix issues.

The free product solves real problems for individual developers. The enterprise product applies the same intelligence across infrastructure.

| | Traditional monitoring (SolarWinds, Datadog) | Pantheon |
|---|---|---|
| **Architecture** | Agents collect metrics, dashboards visualize | Agents scan, detect, and remediate autonomously |
| **AI integration** | Bolt-on copilots | Native — persistent memory, MCP server, AI-dispatched orchestration |
| **Privacy** | Telemetry to vendor cloud | Zero telemetry. All data stays on your machine. |
| **Ghost detection** | Not applicable | Launch Services phantoms, orphaned plists, Spotlight ghosts |
| **Network safety** | Alert on misconfiguration | Probe, fix, verify, auto-revert — safely |

---

## What It Does

### Scan for waste
```bash
sirsi scan                 # 58 rules across 7 domains — caches, build artifacts, orphaned files
sirsi scan --all           # Deep scan
sirsi scan --json          # Machine-readable output
```

### Hunt ghost apps
```bash
sirsi ghosts               # Find remnants of apps you already uninstalled
sirsi ghosts --sudo        # Include system directories
```

Ghost detection catches Launch Services phantoms, orphaned plists, leftover caches, and Spotlight ghosts that standard cleanup tools miss.

### Deduplicate files
```bash
sirsi dedup ~/Downloads ~/Documents
```

Three-phase scan: size grouping → partial hash (8 KB per file) → full hash. Opens a web UI with smart keep/delete recommendations.

### System health diagnostic
```bash
sirsi doctor               # RAM pressure, disk space, kernel panics, Jetsam events
sirsi doctor --json
```

### Network security audit
```bash
sirsi network              # DNS, WiFi, TLS, CA certs, VPN, firewall — read-only
sirsi network --fix        # Auto-apply encrypted DNS + firewall with safety rollback
sirsi network --rollback   # Restore DNS to pre-fix state
```

The `--fix` command uses a three-layer safety model: TCP probe before changing config, watchdog polling after, auto-revert within 6 seconds if resolution fails. [Case study →](docs/case-studies/isis-dns-safety-rollback.md)

### Hardware profiling
```bash
sirsi hardware             # CPU, GPU, RAM, Neural Engine, accelerators
sirsi hardware --json      # Full hardware profile
```

Detects Apple Silicon (ANE, Metal), NVIDIA (CUDA), AMD (ROCm), and Intel. Routes ML workloads to the fastest available accelerator.

### AI project memory
```bash
sirsi thoth init           # Create .thoth/ knowledge system in your project
sirsi thoth sync           # Sync from source + git history
sirsi mcp                  # Start MCP server for Claude, Cursor, Windsurf
```

Thoth gives AI coding sessions persistent memory via the [Model Context Protocol](https://modelcontextprotocol.io). Instead of re-explaining your project every session, the AI reads `.thoth/memory.yaml` and starts with full context.

### Code quality governance
```bash
sirsi quality              # Full governance audit (coverage, formatting, static analysis)
sirsi quality --skip-test  # Use cached coverage
```

Runs automatically on every `git push` via the pre-push gate. Three depth tiers: fast (10-30s default), standard (60-90s), deep (3-5 min pre-release).

### Knowledge ingestion
```bash
sirsi seshat ingest --source chrome       # Chrome bookmarks + history
sirsi seshat ingest --all-profiles        # All Chrome profiles
sirsi seshat export notebooklm            # Push to Google NotebookLM
```

Ingests from Chrome, Gemini, Claude, Apple Notes, and Google Workspace. Exports to NotebookLM, Thoth, and Gemini. All data stays local.

---

## Install

### Homebrew (macOS / Linux)
```bash
brew tap SirsiMaster/tools
brew install sirsi-pantheon
```

### From source
```bash
git clone https://github.com/SirsiMaster/sirsi-pantheon.git
cd sirsi-pantheon
go build -o sirsi ./cmd/sirsi/
```

### Binary
Download from [GitHub Releases](https://github.com/SirsiMaster/sirsi-pantheon/releases).

---

## All Commands

| Command | What It Does |
|:--------|:-------------|
| `sirsi scan` | Find infrastructure waste (58 rules, 7 domains) |
| `sirsi ghosts` | Detect remnants of uninstalled apps |
| `sirsi dedup [dirs]` | Find duplicate files with three-phase hashing |
| `sirsi doctor` | One-shot system health diagnostic |
| `sirsi network` | Network security audit (DNS, WiFi, TLS, firewall, VPN) |
| `sirsi hardware` | CPU, GPU, RAM, Neural Engine detection |
| `sirsi guard` | Real-time resource monitoring |
| `sirsi quality` | Code governance audit |
| `sirsi thoth init/sync` | AI project memory |
| `sirsi mcp` | MCP server for AI IDEs |
| `sirsi seshat ingest` | Knowledge ingestion from browsers and AI tools |
| `sirsi diagram` | Generate architecture diagrams (Mermaid/HTML) |
| `sirsi version` | Show version and module info |

Every command supports `--json`, `--quiet`, and `--verbose` flags.

---

## Editions

| Edition | Scope | Price |
|:--------|:------|:------|
| **Pantheon** | Single machine — all commands above | **Free forever** |
| **Pantheon Ra** | Fleet management — multi-repo orchestration, subnet scanning, compliance | Contact us |

The free edition has no feature gating, no telemetry, no time limits. MIT licensed.

---

## Security & Privacy

- **Zero telemetry.** No analytics, no tracking, no data leaves your machine. Non-negotiable.
- **Dry-run by default.** Every destructive operation requires explicit `--confirm`.
- **35 protected paths.** System directories, keychains, and SSH keys are hardcoded as undeletable.
- **Trash-first cleaning.** Removals go to Trash with a full decision log.
- **DNS safety model.** Network fixes probe before changing config, auto-revert on failure.

---

## Development

```bash
git clone https://github.com/SirsiMaster/sirsi-pantheon.git
cd sirsi-pantheon
git config core.hooksPath .githooks    # Enable pre-push quality gate
go test ./...                          # 1,663 tests across 27 packages
go build ./cmd/sirsi/
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## Built by Sirsi Technologies

[sirsi.ai](https://sirsi.ai) · [GitHub](https://github.com/SirsiMaster) · [Pantheon Hub](https://pantheon.sirsi.ai)

Apache License 2.0 — free and open source forever.
