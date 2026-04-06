# 🏛️ Sirsi Pantheon

**Unified DevOps Intelligence Platform by [Sirsi Technologies](https://sirsi.ai) — One Install, All Deities.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-C8A951?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.9.0--rc1-1A1A5E?style=flat)](VERSION)
[![Tests](https://img.shields.io/badge/tests-1500%2B%20passing-brightgreen?style=flat)](.github/workflows/ci.yml)
[![OpenVSX](https://img.shields.io/badge/OpenVSX-v0.5.1-purple?style=flat)](https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon)
[![MCP](https://img.shields.io/badge/MCP-2025--03--26-purple?style=flat)](https://modelcontextprotocol.io)
[![Build in Public](https://img.shields.io/badge/building-in%20public-C8A951?style=flat)](docs/BUILD_LOG.md)

> *"One Install. All Deities."*

**Sirsi Pantheon** is a unified DevOps intelligence platform that brings together every deity in the Sirsi ecosystem into a single, lightweight binary. Install once, get infrastructure hygiene, QA/QC governance, persistent AI knowledge, and more.

### The Pantheon (10 Deities)

| Glyph | Deity | Domain | What It Does |
|:------|:------|:-------|:-------------|
| 𓇶 | **Ra** | Orchestrator | Dispatches parallel agents across repos, Command Center TUI |
| 𓁯 | **Net** | Scope Weaver | Defines task scopes for Ra, cross-module alignment, drift detection |
| 𓁟 | **Thoth** | Session Memory | Cuts AI token waste by 98% via persistent memory |
| 𓆄 | **Ma'at** | Quality Gate | Grades code quality, enforces standards, pre-push hooks |
| 𓁐 | **Isis** | Health & Remedy | System diagnostics, network security, DNS fix/rollback, auto-remediation |
| 𓁆 | **Seshat** | Knowledge Bridge | Ingests from Chrome, Gemini, Claude, Notes; exports to NotebookLM, Thoth |
| 𓃣 | **Anubis** | Hygiene Engine | Scans waste, cleans artifacts, ghost hunting, file dedup |
| 𓈗 | **Hapi** | Hardware Profiler | GPU, Neural Engine, CUDA, Metal, accelerator detection |
| 𓇽 | **Seba** | Infra Mapper | Architecture diagrams, fleet discovery, container audit |
| 𓁹 | **Osiris** | Snapshot Keeper | State preservation, checkpoints, rollback points |

---

## ⚡ Installation — All Platforms & Methods

> **Not sure which tool to use?** See the [FAQ: Which Pantheon Tool Should I Use?](https://pantheon.sirsi.ai/faq)

### CLI (macOS / Linux)
```bash
# Homebrew (recommended)
brew tap SirsiMaster/tools && brew install sirsi-pantheon

# From source (requires Go 1.22+)
go install github.com/SirsiMaster/sirsi-pantheon/cmd/pantheon@latest

# Or clone and build
git clone https://github.com/SirsiMaster/sirsi-pantheon.git
cd sirsi-pantheon && go build -o pantheon ./cmd/pantheon/

# Install individual deities
brew install SirsiMaster/tools/pantheon-anubis    # Infrastructure hygiene only
brew install SirsiMaster/tools/pantheon-maat      # Code quality only
brew install SirsiMaster/tools/pantheon-thoth     # AI memory only
```

### CLI (Windows)
```powershell
# From source (requires Go 1.22+)
go install github.com/SirsiMaster/sirsi-pantheon/cmd/pantheon@latest

# Or download binary from GitHub Releases
# https://github.com/SirsiMaster/sirsi-pantheon/releases
```

### VS Code Extension (macOS / Linux / Windows)
Search **"Pantheon"** in your extension marketplace, or:
```bash
# VS Code
code --install-extension SirsiMaster.sirsi-pantheon

# Antigravity / Cursor / Windsurf — works on any VS Code-based IDE
# Also available on OpenVSX: https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon
```

### Menu Bar App (macOS only)
```bash
# Build from source
go build -o /opt/homebrew/bin/pantheon-menubar ./cmd/pantheon-menubar/

# Auto-start at login
cp cmd/pantheon-menubar/bundle/ai.sirsi.pantheon.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/ai.sirsi.pantheon.plist
```

### MCP Server (any platform with AI IDE)
```bash
pantheon mcp    # Starts JSON-RPC 2.0 server over stdio
```
See [MCP Configuration](#-mcp-server--ai-ide-integration) below.

### Binary Sizes

| Component | Download | On Disk |
|:----------|:---------|:--------|
| CLI (`pantheon`) | 6.6 MB | 12 MB |
| Menu Bar (`pantheon-menubar`) | 2.6 MB | 4.5 MB |
| Extension (`.vsix`) | 40 KB | 40 KB |
| Agent (`pantheon-agent`) | ~1.2 MB | 2.1 MB |

---

## Getting Started

Get up and running in under a minute. For the full walkthrough, see the [Getting Started Guide](https://pantheon.sirsi.ai/getting-started).

```bash
# 1. Install
brew tap SirsiMaster/tools && brew install sirsi-pantheon

# 2. Scan your machine for waste
pantheon scan

# 3. Find ghost apps and duplicates
pantheon ghosts
pantheon dedup ~/Documents

# 4. Set up AI memory in a project
cd ~/my-project && pantheon thoth init && pantheon thoth sync

# 5. Audit code quality and auto-fix
pantheon maat audit
pantheon maat heal
```

---

### Scan Your Machine
```bash
pantheon scan                    # Find infrastructure waste (caches, build artifacts, orphaned files)
pantheon scan --all              # Scan all categories
pantheon scan --json             # Machine-readable output
```

### Find Ghost Apps
```bash
pantheon ghosts                  # Detect remnants of uninstalled applications
pantheon ghosts --sudo           # Include system directories
```

### Find Duplicate Files
```bash
pantheon dedup ~/Downloads ~/Desktop  # Scan directories for duplicates
```

### Monitor Resources
```bash
pantheon guard                   # System resource monitoring and memory pressure
```

### AI Project Memory
```bash
pantheon thoth init              # Initialize .thoth/ knowledge system
pantheon thoth sync              # Sync memory from source + git history
pantheon thoth compact -s "..."  # Persist session decisions before context compression
```

### Start MCP Server (any AI IDE)
```bash
pantheon mcp                     # JSON-RPC 2.0 over stdio — works with Claude, Cursor, Windsurf
```

### Run Governance Audit
```bash
pantheon maat audit              # Coverage governance across all modules
pantheon maat audit --skip-test  # Use cached coverage (instant)
```

---

## Commands

### Core (what most users need)
| Command | Description |
|:--------|:-----------|
| `pantheon scan` | Scan for infrastructure waste (caches, build artifacts, orphaned files) |
| `pantheon ghosts` | Detect remnants of uninstalled applications |
| `pantheon dedup [dirs...]` | Find duplicate files across directories |
| `pantheon guard` | Monitor system resources and memory pressure |
| `pantheon mcp` | Start MCP server for AI IDE integration |
| `pantheon thoth init` | Initialize AI project memory (.thoth/) |
| `pantheon thoth sync` | Sync project memory from source + git |
| `pantheon thoth compact` | Persist session decisions before context compression |
| `pantheon maat audit` | Governance and coverage auditing |
| `pantheon version` | Show version |

### Global Flags
```bash
--json      # JSON output for scripting
--quiet     # Suppress non-essential output
--verbose   # Debug logging
```

---

## 🏛 Architecture

Pantheon is built on modules named after Egyptian mythology. Every deity maintains its identity while sharing a unified runtime:

| Module | Deity | Role | Status |
|:-------|:------|:-----|:-------|
| **Ra** | 𓇶 Ra | Multi-repo agent orchestration, Command Center | ✅ |
| **Net** | 𓁯 Net | Scope weaving, alignment, drift detection | ✅ |
| **Thoth** | 𓁟 Thoth | Session memory, brain, MCP server | ✅ |
| **Ma'at** | 𓆄 Ma'at | Coverage, canon, policy enforcement | ✅ |
| **Isis** | 𓁐 Isis | Health diagnostics, network security, guard, remediation | ✅ |
| **Seshat** | 𓁆 Seshat | Universal knowledge grafting engine | ✅ |
| **Anubis** | 𓃣 Anubis | Scan engine (64 rules), ghost hunting, file dedup | ✅ |
| **Hapi** | 𓈗 Hapi | GPU/Neural Engine/VRAM detection | ✅ |
| **Seba** | 𓇽 Seba | Architecture mapping, fleet discovery, container audit | ✅ |
| **Osiris** | 𓁹 Osiris | State snapshots, checkpoints | 🚧 |

---

## Scan Domains (64 Rules)

| Domain | Examples |
|:-------|:--------|
| 🖥️ **General Mac** | Caches, logs, crash reports, browser junk, downloads |
| 🐳 **Virtualization** | Parallels, Docker, VMware, UTM, VirtualBox |
| 📦 **Dev Frameworks** | Node/npm, Next.js, Rust/Cargo, Go, Python/conda, Java/Gradle |
| 🤖 **AI/ML** | Apple MLX, Metal, NVIDIA CUDA, HuggingFace, Ollama, PyTorch |
| 🛠️ **IDEs & AI Tools** | Xcode, VS Code, JetBrains, Claude Code, Codex, Gemini CLI |
| ☁️ **Cloud/Infra** | Docker, Kubernetes, nginx, Terraform, gcloud, Firebase |
| 📱 **Cloud Storage** | OneDrive, Google Drive, iCloud, Dropbox |

---

## 🧠 Neural Brain

Pantheon includes an on-demand neural classification engine:

```bash
pantheon install-brain             # Download CoreML/ONNX model
pantheon install-brain --update    # Check for latest version
pantheon install-brain --remove    # Self-delete weights
```

The brain classifies files into 9 categories: **junk**, **project**, **config**, **model**, **data**, **media**, **archive**, **essential**, **unknown**. Currently ships with a heuristic classifier; neural model backends (ONNX Runtime, CoreML) are in development.

---

## 🔌 MCP Server — AI IDE Integration

Pantheon doubles as a context sanitizer for AI coding assistants:

```bash
pantheon mcp    # Start MCP server (stdio)
```

### Configure Claude Code
```json
// ~/.claude/claude_desktop_config.json
{
  "mcpServers": {
    "pantheon": {
      "command": "pantheon",
      "args": ["mcp"]
    }
  }
}
```

### Configure Cursor / Windsurf
```json
// .cursor/mcp.json
{
  "mcpServers": {
    "pantheon": {
      "command": "pantheon",
      "args": ["mcp"]
    }
  }
}
```

### MCP Tools
| Tool | Description |
|:-----|:-----------|
| `scan_workspace` | Scan a directory for infrastructure waste |
| `ghost_report` | Detect remnants of uninstalled applications |
| `health_check` | System health summary (cached, <10ms response) |
| `thoth_read_memory` | Load project context without reading source files |
| `thoth_sync` | Sync project memory from source + git history |
| `detect_hardware` | CPU, GPU, Neural Engine, and accelerator detection |

---

## 𓁟 Thoth — Persistent Knowledge System

Thoth gives AI assistants **persistent memory across sessions**. Instead of re-reading thousands of lines of source code every time, the AI reads a ~100-line memory file for instant context.

```
.thoth/
├── memory.yaml      # Layer 1: WHAT — compressed project state
├── journal.md       # Layer 2: WHY — timestamped reasoning
└── artifacts/       # Layer 3: DEEP — benchmarks, audits, reviews
```

### Measured Impact (Dogfooding on This Repo)

| Metric | Without Thoth | With Thoth | Savings |
|:-------|:-------------|:-----------|:--------|
| Lines read at startup | 22,958 | 297 | **98.7% fewer** |
| Tokens consumed | 275,496 | 3,564 | **271,932 saved** |
| Context window used | 137.7% (⚠️ doesn't fit) | 1.7% | **136% preserved** |
| Cost per session (Opus 4) | $4.13 | $0.05 | **$4.08 saved** |

> We built Thoth because our own AI sessions were failing — the codebase was too large to fit in context. The before/after is measurable. [Read the case study →](docs/case-studies/thoth-context-savings.md)

---

## 🪶 Ma'at — QA/QC Governance

Ma'at ensures every change meets quality standards:

```bash
pantheon maat                    # Full governance assessment
pantheon maat --coverage         # Test coverage thresholds
pantheon maat --canon            # Canon document verification
pantheon maat --pipeline         # CI pipeline health
pantheon maat --json             # Machine-readable output
```

57 tests, 3 governance domains, per-module threshold enforcement.

---

## 𓁆 Seshat — Universal Knowledge Grafting

Seshat ingests knowledge from multiple sources and exports it to any target in the Sirsi ecosystem.

### Chrome Profile Ingestion
```bash
pantheon seshat ingest --source chrome                    # Default Chrome profile
pantheon seshat ingest --source chrome --profile "Work"   # Named profile
pantheon seshat ingest --source chrome --all-profiles     # All Chrome profiles
pantheon seshat profiles --source chrome                  # List available profiles
```

### Supported Sources
- **Chrome** — bookmarks, history, saved tabs (per-profile or all profiles)
- **Gemini** — conversation exports
- **Claude** — session memory
- **Apple Notes** — local note ingestion
- **Google Workspace** — Docs, Sheets, Drive

### Export Targets
- **Thoth** — merge into `.thoth/memory.yaml` for AI context
- **GEMINI.md** — export as Gemini-compatible project memory
- **NotebookLM** — export for Google NotebookLM ingestion

```bash
pantheon seshat export --target notebooklm --output ./export/
pantheon seshat export --target thoth
pantheon seshat export --target gemini
```

---

## ⚖️ Policy Enforcement

Define infrastructure hygiene policies in YAML:

```yaml
api_version: v1
policies:
  - name: workstation-hygiene
    rules:
      - id: waste-limit
        metric: total_size
        operator: gt
        threshold: 20
        unit: GB
        severity: fail
        remediation: Run 'pantheon judge --confirm'
```

```bash
pantheon scales enforce                      # Run default policies
pantheon scales enforce -f custom-policy.yaml # Custom policies
pantheon scales validate -f policy.yaml      # Syntax check
```

---

## 🪞 Mirror — File Deduplication

Mirror finds duplicate files across any directory using a **three-phase scan**:

1. **Size grouping** — instant elimination of unique file sizes
2. **Partial hash** — SHA-256 of first 4KB + last 4KB (8KB per file)
3. **Full hash** — complete SHA-256 only for files that pass both phases

### Why This Matters

| Metric | Naive approach | Pantheon Mirror |
|:-------|:--------------|:----------------|
| 56 candidate files (97.8 MB) | Reads all 97.8 MB | Reads 448 KB partial, then only matched files |
| Disk I/O | 97.8 MB | **< 2 MB** |
| Time | 84 ms | **3 ms** |
| Speedup | 1x | **27.3x** |
| I/O reduction | — | **98.8%** |

*Benchmarked on real ~/Downloads directory, March 2026.*

---

## 🛡️ Product Tiers

| Tier | Scope | Price |
|:-----|:------|:------|
| **Pantheon Free** | Single workstation, all commands, Mirror GUI + CLI | Free forever |
| **Pantheon Pro** | Neural brain, importance ranking, semantic search | $9/mo |
| **Eye of Horus** | Subnet sweep (< 100 nodes) | $29/mo |
| **Ra** | Enterprise fleet, SAN/NAS, compliance | Contact |

---

## 🔒 Security & Privacy

- **Rule A11: No Telemetry** — zero analytics, tracking, or data collection
- **Rule A1: Safety First** — all destructive ops require `--confirm` or `--dry-run`
- **Rule A3: Fixed Agent Commands** — agent has no shell access
- **Trash-first cleaning** — every removal goes to Trash with full decision log
- **35 protected paths** — system dirs, user content dirs, keychains, and SSH keys are hardcoded as undeletable
- **`--stealth` mode** — Pantheon comes, judges, and vanishes (zero footprint)
- All scanning is local — no data leaves your machine

---

## 🛠️ Development

### Setup
```bash
git clone https://github.com/SirsiMaster/sirsi-pantheon.git
cd sirsi-pantheon
git config core.hooksPath .githooks    # Enable pre-push gate
go build ./...
```

### Pre-Push Gate — Tiered Depth

Every `git push` runs a quality gate. The default is **fast** (~10-30 seconds).
Set `MAAT_DEPTH` to control how deep the gate checks:

| Tier | Time | What Runs | When to Use |
|:-----|:-----|:----------|:------------|
| **`fast`** (default) | ~10-30s | `gofmt` + `go vet` + build + tests on changed packages only | Every push |
| **`standard`** | ~60-90s | Fast + Ma'at coverage/canon on changed packages | Before PR merge |
| **`deep`** | ~3-5 min | Standard + full test suite + full Ma'at assessment | Pre-release only |

```bash
git push                          # fast (default) — seconds, not minutes
MAAT_DEPTH=standard git push      # standard — adds Ma'at on changed pkgs
MAAT_DEPTH=deep git push          # deep — full suite (pre-release)
git push --no-verify              # skip gate entirely (use sparingly)
```

> **Why tiered?** The full Ma'at + test suite takes 3-5 minutes and floods the IDE
> with terminal output, causing IPC starvation on Electron-based editors.
> The fast tier runs only what's relevant to your change. CI still runs the
> full deep suite on every push — the local gate catches the 95% case in seconds.

### Adding Scan Rules

Implement the `ScanRule` interface in `internal/jackal/rules/`.
See [CONTRIBUTING.md](CONTRIBUTING.md) for full guidelines.

---

## 🔌 VS Code Extension — Always-On Guardian

The Pantheon extension brings Guardian auto-renice, memory GC, and live metrics into your IDE.

| Feature | Description |
|:--------|:-----------|
| **Guardian Auto-Renice** | Deprioritizes LSP processes (gopls, tsserver, rust-analyzer) to nice +10 / Background QoS |
| **Memory Pressure GC** | Restarts bloated language servers when >500 MB sustained for 3+ checks |
| **Crashpad Monitor** | 🆕 Monitors crash dumps — detects Extension Host instability before cascade crashes. No other extension does this. |
| **Status Bar** | `$(eye) PANTHEON 4.8 GB` — live RAM metrics + crash stability indicator |
| **Command Palette** | 10 commands: Scan, Guard, Renice, Ka, Thoth, Metrics, Accountability, Crashpad, Settings |
| **Thoth Context** | Reads `.thoth/memory.yaml` for instant project context |
| **Thoth Accountability** | Cold-start benchmark proving 371K tokens saved per session |

**Install**: Search "Pantheon" in extensions, or visit [OpenVSX](https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon).
**Works in**: VS Code, Antigravity, Cursor, Windsurf — any VS Code-based IDE.
**No AI required.** No CLI binary dependency for core features.

---

## 🖥️ Menu Bar App — IDE-Independent Monitoring

Native macOS menu bar application that runs independently of any IDE.

| Feature | Description |
|:--------|:-----------|
| **System Tray Icon** | Heavyweight ankh (☥) in macOS menu bar — always visible |
| **Live Stats** | RAM pressure, Git status, branch, accelerator detection |
| **Command Shortcuts** | Quick-launch scan, guard, ghost hunt from the menu |
| **Auto-Start** | LaunchAgent with `RunAtLoad: true`, `KeepAlive: true` — survives reboots |

**Footprint**: 4.5 MB on disk, ~50 MB RSS at runtime.
**macOS only** (12.0+).

---

## 📄 License

MIT License — free and open source forever. See [LICENSE](LICENSE).

---

## 🏢 Built by Sirsi Technologies

**Sirsi Pantheon** is built and maintained by [Sirsi Technologies](https://sirsi.ai) — the company behind the "Own Your Intelligence" platform for on-device AI infrastructure.

- **Website**: [sirsi.ai](https://sirsi.ai)
- **GitHub**: [github.com/SirsiMaster](https://github.com/SirsiMaster)
- **Pantheon Hub**: [pantheon.sirsi.ai](https://pantheon.sirsi.ai)

## 𓃣 Documentation & Registry

- **[FAQ: Which Tool Should I Use?](https://pantheon.sirsi.ai/faq)**: Decision matrix for choosing the right Pantheon tool.
- **[Deity Registry](https://pantheon.sirsi.ai)**: Interactive hub for all 10 deities.
- **[Build Log](docs/build-log.html)**: Real-time chronicle of the Pantheon build.
- **[Case Studies](docs/case-studies.html)**: Origin stories and post-mortems.
- **[ADR Index](docs/ADR-INDEX.md)**: Every architectural decision record.
- **[OpenVSX Listing](https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon)**: VS Code extension on OpenVSX.

---

*Built by [Sirsi Technologies](https://sirsi.ai). Open source, MIT licensed.*
