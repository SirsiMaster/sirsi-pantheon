# 🏛️ Sirsi Pantheon

**Unified DevOps Intelligence Platform — One Install, All Deities.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-C8A951?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.5.1--alpha-1A1A5E?style=flat)](VERSION)
[![Tests](https://img.shields.io/badge/tests-819%2B%20passing-brightgreen?style=flat)](.github/workflows/ci.yml)
[![OpenVSX](https://img.shields.io/badge/OpenVSX-v0.5.1-purple?style=flat)](https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon)
[![MCP](https://img.shields.io/badge/MCP-2025--03--26-purple?style=flat)](https://modelcontextprotocol.io)
[![Build in Public](https://img.shields.io/badge/building-in%20public-C8A951?style=flat)](docs/BUILD_LOG.md)

> *"One Install. All Deities."*

**Sirsi Pantheon** is a unified DevOps intelligence platform that brings together every deity in the Sirsi ecosystem into a single, lightweight binary. Install once, get infrastructure hygiene, QA/QC governance, persistent AI knowledge, and more.

### The Deities

| Glyph | Deity | Domain | What It Does |
|:------|:------|:-------|:-------------|
| 𓂀 | **Anubis** | Judge of the Dead | Finds junk on your computer that cleaning apps miss |
| 𓂓 | **Ka** | Spirit of the Dead | Detects apps you deleted that are still secretly running |
| 𓉡 | **Hathor** | Goddess of Reflection | Finds duplicate files eating your storage |
| 𓁵 | **Sekhmet** | Warrior Guardian | Kills runaway processes that freeze your machine |
| 𓆄 | **Ma'at** | Truth and Order | Grades your code quality and enforces team standards |
| 𓁹 | **Horus** | The All-Seeing Eye | Fixes broken search results and indexes your filesystem |
| 𓆣 | **Khepri** | Scarab of Renewal | Scans every device on your network and audits Docker |
| 𓇼 | **Seba** | Star Map | Draws a visual map of your infrastructure |
| 𓁟 | **Thoth** | God of Knowledge | Cuts AI token waste by 98% so your context window lasts |
| 𓁶 | **Hapi** | God of the Flood | Detects your GPU, Neural Engine, and available hardware |
| ☀️ | **Ra** | Supreme Overseer | The boss — runs all deities automatically (planned) |

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

### Scan Your Machine
```bash
pantheon weigh                   # Full scan — discover all waste
pantheon weigh --dev             # Developer frameworks only
pantheon weigh --ai              # AI/ML caches only
pantheon weigh --json            # Machine-readable output
```

### Clean What Was Found
```bash
pantheon judge --dry-run         # Preview cleanup
pantheon judge --confirm         # Execute cleanup
```

### Hunt Ghost Apps
```bash
pantheon ka                      # Find remnants of uninstalled apps
pantheon ka --target "Parallels" # Hunt specific ghost
pantheon ka --clean --dry-run    # Preview ghost cleanup
pantheon ka --clean --confirm    # Release the spirits
```

### Find Duplicate Files
```bash
pantheon mirror ~/Downloads ~/Desktop  # Scan directories for duplicates
pantheon mirror --photos --min-size 1MB # Large photo duplicates only
pantheon mirror --json > report.json   # Export results
pantheon mirror                        # Launch GUI (browser-based)
```

### Run QA/QC Governance
```bash
pantheon maat                    # Full governance assessment
pantheon maat --coverage         # Test coverage governance
pantheon maat --canon            # Canon document verification
pantheon maat --pipeline         # CI pipeline monitoring
```

---

## 📋 All Commands

| Command | Deity | Description |
|:--------|:------|:-----------|
| `pantheon weigh` | 𓂀 Anubis | Scan workstation for infrastructure waste |
| `pantheon judge` | 𓂀 Anubis | Clean artifacts found by weigh |
| `pantheon ka` | 𓂓 Ka | Hunt ghost apps — find spirits of the dead |
| `pantheon guard` | 𓁵 Sekhmet | RAM audit, zombie process management |
| `pantheon sight` | 𓁹 Horus | Launch Services / Spotlight repair |
| `pantheon mirror` | 𓉡 Hathor | Duplicate file scanner (GUI + CLI) |
| `pantheon hapi` | 𓁶 Hapi | Hardware detection (GPU, Neural Engine, VRAM) |
| `pantheon scarab` | 𓆣 Khepri | Network discovery + container audit |
| `pantheon seba` | 𓇼 Seba | Infrastructure topology graph |
| `pantheon mcp` | 𓁟 Thoth | Start MCP server for AI IDE integration |
| `pantheon install-brain` | 𓁟 Thoth | Download neural classification model |
| `pantheon maat` | 𓆄 Ma'at | QA/QC governance assessment |
| `pantheon scales enforce` | 𓆄 Ma'at | Run hygiene policy enforcement |
| `pantheon profile` | — | Machine profiling and system info |
| `pantheon version` | — | Show version and deity roster |

### Global Flags
```bash
--json      # JSON output for scripting
--quiet     # Suppress non-essential output
--stealth   # Ephemeral mode — delete all Pantheon data after execution
```

---

## 🏛 Architecture

Pantheon is built on modules named after Egyptian mythology. Every deity maintains its identity while sharing a unified runtime:

| Module | Deity | Role | Status |
|:-------|:------|:-----|:-------|
| **Jackal** | 𓂀 Anubis | Scan engine — 58 rules across 7 domains | ✅ |
| **Cleaner** | 𓂀 Anubis | Safe deletion with Trash + SHA-256 verification | ✅ |
| **Ka** | 𓂓 Ka | Ghost app detection — 17 macOS locations | ✅ |
| **Mirror** | 𓉡 Hathor | File dedup — 27x faster than naive hashing | ✅ |
| **Guard** | 𓁵 Sekhmet | RAM audit, zombie process management | ✅ |
| **Sight** | 𓁹 Horus | Launch Services + Spotlight repair | ✅ |
| **Horus** | 𓁹 Horus | Shared filesystem index — walk once, query forever | ✅ |
| **Hapi** | 𓁶 Hapi | GPU/Neural Engine/VRAM detection | ✅ |
| **Scarab** | 𓆣 Khepri | Network discovery + container audit | ✅ |
| **Brain** | 𓁟 Thoth | On-demand model downloader + classifier | ✅ |
| **MCP** | 𓁟 Thoth | MCP server for AI IDE integration | ✅ |
| **Scales** | 𓆄 Ma'at | YAML policy engine + enforcement | ✅ |
| **Ma'at** | 𓆄 Ma'at | Coverage, canon, pipeline assessments | ✅ |
| **Seba** | 𓇼 Seba | Infrastructure topology graph | 🚧 |

---

## 📦 Scan Domains (58 Rules)

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
| `scan_workspace` | Scan a directory for waste |
| `ghost_report` | Hunt dead app remnants |
| `health_check` | System health summary with grade |
| `classify_files` | Semantic file classification |
| `thoth_read_memory` | 𓁟 Load project context without reading source files |

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
| **Status Bar** | `$(eye) PANTHEON 4.8 GB` — live RAM metrics, color-coded health |
| **Command Palette** | 7 commands: Scan, Guard, Renice, Ka, Thoth, Metrics, Settings |
| **Thoth Context** | Reads `.thoth/memory.yaml` for instant project context |

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

## 🏢 Sirsi Technologies

Sirsi Pantheon is built by [Sirsi Technologies](https://sirsi.ai).

- 🌐 [sirsi.ai](https://sirsi.ai)

## 𓂀 Documentation & Registry

- **[FAQ: Which Tool Should I Use?](https://pantheon.sirsi.ai/faq)**: Decision matrix for choosing the right Pantheon tool.
- **[Deity Registry](https://pantheon.sirsi.ai)**: Interactive hub for all 12 deities.
- **[Build Log](docs/build-log.html)**: Real-time chronicle of the Pantheon build.
- **[Case Studies](docs/case-studies.html)**: Origin stories and post-mortems.
- **[ADR Index](docs/ADR-INDEX.md)**: Every architectural decision record.
- **[OpenVSX Listing](https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon)**: VS Code extension on OpenVSX.

---

*𓂀 One install. All deities. Nothing escapes the Weighing.*
