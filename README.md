# 𓂀 Sirsi Anubis

**Infrastructure Hygiene for the AI Era**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-C8A951?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.3.0--alpha-1A1A5E?style=flat)](VERSION)
[![Tests](https://img.shields.io/badge/tests-303%20passing-brightgreen?style=flat)](.github/workflows/ci.yml)
[![MCP](https://img.shields.io/badge/MCP-2025--03--26-purple?style=flat)](https://modelcontextprotocol.io)
[![Build in Public](https://img.shields.io/badge/building-in%20public-C8A951?style=flat)](docs/BUILD_LOG.md)

> *"Weigh. Judge. Purge."*

Sirsi Anubis is a free, open-source infrastructure hygiene platform. It scans, judges, and purges waste across workstations, containers, VMs, and networks — with duplicate file detection, a neural classification brain, and MCP server for AI IDE integration.

**No cleaning tool understands what developers and AI engineers leave behind.** Anubis does — with 64+ scan rules across 7 domains, ghost app detection, file deduplication that's **27x faster** than naive hashing, and a policy engine for fleet enforcement.

---

## ⚡ Quick Start

### Install
```bash
# From source
go install github.com/SirsiMaster/sirsi-anubis/cmd/anubis@latest

# Or clone and build
git clone https://github.com/SirsiMaster/sirsi-anubis.git
cd sirsi-anubis && go build -o anubis ./cmd/anubis/
```

### Scan Your Machine
```bash
anubis weigh                   # Full scan — discover all waste
anubis weigh --dev             # Developer frameworks only
anubis weigh --ai              # AI/ML caches only
anubis weigh --json            # Machine-readable output
```

### Clean What Was Found
```bash
anubis judge --dry-run         # Preview cleanup
anubis judge --confirm         # Execute cleanup
```

### Hunt Ghost Apps
```bash
anubis ka                      # Find remnants of uninstalled apps
anubis ka --target "Parallels" # Hunt specific ghost
anubis ka --clean --dry-run    # Preview ghost cleanup
anubis ka --clean --confirm    # Release the spirits
```

### Find Duplicate Files
```bash
anubis mirror ~/Downloads ~/Desktop  # Scan directories for duplicates
anubis mirror --photos --min-size 1MB # Large photo duplicates only
anubis mirror --json > report.json   # Export results
anubis mirror                        # Launch GUI (browser-based)
```

---

## 📋 All Commands

| Command | Description |
|:--------|:-----------|
| `anubis weigh` | 𓂀 Scan workstation for infrastructure waste |
| `anubis judge` | ⚖️ Clean artifacts found by weigh |
| `anubis ka` | 𓂓 Hunt ghost apps — find spirits of the dead |
| `anubis guard` | 🛡️ RAM audit, zombie process management |
| `anubis sight` | 👁️ Launch Services / Spotlight repair |
| `anubis profile` | 📊 Machine profiling and system info |
| `anubis seba` | 𓇼 Dependency graph mapper |
| `anubis hapi` | 🌊 Resource optimizer (GPU, dedup, snapshots) |
| `anubis scarab` | 🪲 Network discovery + container audit |
| `anubis install-brain` | 🧠 Download neural classification model |
| `anubis uninstall-brain` | 🧠 Remove neural weights |
| `anubis mcp` | 🔌 Start MCP server for AI IDE integration |
| `anubis scales enforce` | ⚖️ Run hygiene policy enforcement |
| `anubis scales validate` | ⚖️ Validate policy YAML |
| `anubis mirror` | 🪞 Duplicate file scanner (GUI + CLI) |
| `anubis book-of-the-dead` | 📜 Deep system autopsy |
| `anubis initiate` | 🔑 Grant macOS permissions |

### Global Flags
```bash
--json      # JSON output for scripting
--quiet     # Suppress non-essential output
--stealth   # Ephemeral mode — delete all Anubis data after execution
```

---

## 🏛 Architecture

Anubis is built on modules named after Egyptian mythology:

| Module | Codename | Role | Status |
|:-------|:---------|:-----|:-------|
| 🐺 **Jackal** | The Hunter | Scan engine — 64 rules across 7 domains | ✅ |
| 𓂓 **Ka** | The Spirit | Ghost app detection — 17 macOS locations | ✅ |
| 🪞 **Mirror** | The Reflection | File deduplication — 27x faster than naive hashing | ✅ |
| 🛡️ **Guard** | The Guardian | RAM audit, zombie process management | ✅ |
| 👁️ **Sight** | The Sight | Launch Services + Spotlight repair | ✅ |
| 📊 **Profile** | The Record | Machine profiling and system info | ✅ |
| 𓇼 **Seba** | The Gateway | Dependency graph mapper | ✅ |
| 🌊 **Hapi** | The Flow | GPU detection, dedup, APFS snapshots | ✅ |
| 🪲 **Scarab** | The Transformer | Network discovery + container audit | ✅ |
| 🧠 **Brain** | Neural | On-demand model downloader + classifier | ✅ |
| 🔌 **MCP** | Context Sanitizer | MCP server for AI IDE integration | ✅ |
| ⚖️ **Scales** | The Judgment | YAML policy engine + enforcement | ✅ |

### Two Binaries

| Binary | Size | Purpose |
|:-------|:-----|:--------|
| `anubis` | 12 MB | Full CLI controller + Mirror GUI |
| `anubis-agent` | 3.2 MB | Lightweight fleet agent (JSON only, fixed command set) |

---

## 📦 Scan Domains (64+ Rules)

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

Anubis includes an on-demand neural classification engine:

```bash
anubis install-brain             # Download CoreML/ONNX model
anubis install-brain --update    # Check for latest version
anubis install-brain --remove    # Self-delete weights
```

The brain classifies files into 9 categories: **junk**, **project**, **config**, **model**, **data**, **media**, **archive**, **essential**, **unknown**. Currently ships with a heuristic classifier; neural model backends (ONNX Runtime, CoreML) are in development.

---

## 🔌 MCP Server — AI IDE Integration

Anubis doubles as a context sanitizer for AI coding assistants:

```bash
anubis mcp    # Start MCP server (stdio)
```

### Configure Claude Code
```json
// ~/.claude/claude_desktop_config.json
{
  "mcpServers": {
    "anubis": {
      "command": "anubis",
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
    "anubis": {
      "command": "anubis",
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

Thoth gives AI assistants **persistent memory across sessions**. Instead of re-reading 10,000+ lines of source code every time, the AI reads a ~100-line memory file for instant context.

```
.thoth/
├── memory.yaml      # Layer 1: WHAT — compressed project state
├── journal.md       # Layer 2: WHY — timestamped reasoning
└── artifacts/       # Layer 3: DEEP — benchmarks, audits, reviews
```

**Measured impact**: 99.3% reduction in context needed to start a session.

Thoth is MIT licensed and works with any project, any language, any AI assistant. See [docs/THOTH.md](docs/THOTH.md) for the full specification.


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
        remediation: Run 'anubis judge --confirm'
```

```bash
anubis scales enforce                      # Run default policies
anubis scales enforce -f custom-policy.yaml # Custom policies
anubis scales validate -f policy.yaml      # Syntax check
```

---

## 🪞 Mirror — File Deduplication

Mirror finds duplicate files across any directory using a **three-phase scan**:

1. **Size grouping** — instant elimination of unique file sizes
2. **Partial hash** — SHA-256 of first 4KB + last 4KB (8KB per file)
3. **Full hash** — complete SHA-256 only for files that pass both phases

### Why This Matters

| Metric | Naive approach | Anubis Mirror |
|:-------|:--------------|:--------------|
| 56 candidate files (97.8 MB) | Reads all 97.8 MB | Reads 448 KB partial, then only matched files |
| Disk I/O | 97.8 MB | **< 2 MB** |
| Time | 84 ms | **3 ms** |
| Speedup | 1x | **27.3x** |
| I/O reduction | — | **98.8%** |

*Benchmarked on real ~/Downloads directory, March 2026.*

### Cleaning Policy

- **Trash first** — all removals go to macOS Trash (reversible, "Put Back" works)
- **Decision log** — every action recorded with path, SHA-256, reason, timestamp
- **Per-file rollback** — session logs persist at `~/.config/anubis/mirror/decisions/`
- **Human approval required** — no automatic deletion, ever

### GUI + CLI Feature Parity

`anubis mirror` (no args) launches a browser-based GUI with native macOS folder picker.
Every feature in the CLI is available in the GUI — identical engine, different interface.

---

## 🛡️ Product Tiers

| Tier | Scope | Price |
|:-----|:------|:------|
| **Anubis Free** | Single workstation, all scan commands, Mirror GUI + CLI | Free forever |
| **Anubis Pro** | Neural brain, importance ranking, semantic search | $9/mo |
| **Eye of Horus** | Subnet sweep (< 100 nodes) | $29/mo |
| **Ra** | Enterprise fleet, SAN/NAS, compliance | Contact |

---

## 🔒 Security & Privacy

- **Rule A11: No Telemetry** — zero analytics, tracking, or data collection
- **Rule A1: Safety First** — all destructive ops require `--confirm` or `--dry-run`
- **Rule A3: Fixed Agent Commands** — agent has no shell access
- **Trash-first cleaning** — every removal goes to Trash with full decision log
- **29 protected paths** — system dirs, user content dirs, keychains, and SSH keys are hardcoded as undeletable
- **`--stealth` mode** — Anubis comes, judges, and vanishes (zero footprint)
- All scanning is local — no data leaves your machine

---

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Adding scan rules is straightforward — implement the `ScanRule` interface in `internal/jackal/rules/`.

---

## 📄 License

MIT License — free and open source forever. See [LICENSE](LICENSE).

---

## 🏢 Sirsi Technologies

Sirsi Anubis is the infrastructure hygiene product from [Sirsi Technologies](https://github.com/SirsiMaster).

| Product | Role |
|:--------|:-----|
| **Sirsi Anubis** | Infrastructure hygiene platform |
| **Sirsi Nexus** | AI infrastructure platform |

---

*𓂀 The jackal sees everything. Nothing escapes the Weighing.*
