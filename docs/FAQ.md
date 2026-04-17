# FAQ: Which Pantheon Tool Should I Use?

Pantheon runs across **four layers** — use whichever matches your workflow. Most developers use 2-3 simultaneously.

---

## Quick Decision Matrix

| I want to... | Use | Install |
|:---|:---|:---|
| **Auto-deprioritize LSPs and monitor RAM in my IDE** | VS Code Extension | `ext install SirsiMaster.sirsi-pantheon` from OpenVSX |
| **Always see system health in my Mac menu bar** | Menu Bar App | `brew install sirsi-sirsi-menubar` or LaunchAgent |
| **Scan for waste, ghosts, and bloat from terminal** | CLI | `brew install sirsi-pantheon` |
| **Let my AI coding assistant query Pantheon directly** | MCP Server | `sirsi mcp` (stdio mode) |

---

## Layer Breakdown

### 1. 🔌 VS Code Extension (40 KB)
**Best for**: Any developer using VS Code, Cursor, Windsurf, or Antigravity.

**What you get**:
- **Guardian**: Auto-renice LSPs to Background QoS (nice +10) — zero config
- **Memory GC**: Restarts bloated language servers automatically (>500 MB sustained)
- **Status Bar**: Live `$(eye) PANTHEON 4.8 GB` metrics, refreshed every 5s
- **Command Palette**: Scan workspace, ghost report, renice, apply optimal settings
- **Thoth**: Context compression from `.thoth/memory.yaml`

**Requires AI?** No. Works identically on vanilla VS Code.

**Install**: Available on [OpenVSX](https://open-vsx.org/extension/SirsiMaster/sirsi-pantheon) — search "Pantheon" in extensions.

---

### 2. 🖥️ Menu Bar App (4.5 MB)
**Best for**: Anyone who wants Pantheon monitoring regardless of which IDE (or no IDE) is running.

**What you get**:
- **System tray icon**: Always-visible ankh in macOS menu bar
- **Live stats**: RAM pressure, Git status, branch, accelerator detection
- **Command shortcuts**: Quick-launch scan, guard, ghost hunt
- **Auto-start**: LaunchAgent runs at login, survives reboots

**Requires AI?** No. Pure native macOS.

**Install**: `brew install sirsi-pantheon` then `sirsi-menubar`, or install the LaunchAgent for auto-start.

---

### 3. ⌨️ CLI (12 MB)
**Best for**: Power users, scripts, CI/CD pipelines, automation.

**What you get**:
- `sirsi weigh` — Full infrastructure scan (58 rules across 7 domains)
- `sirsi ka` — Ghost detection (17 macOS locations)
- `sirsi guard` — Process monitoring and resource control
- `sirsi sekhmet` — **ANE-accelerated tokenization** (high-perf BPE)
- `sirsi maat` — Quality governance and coverage tracking
- `sirsi mirror` — File deduplication with GUI
- 18 total commands + JSON output for scripting

**Requires AI?** No. Standard Unix tool.

**Install**: `brew tap SirsiMaster/tools && brew install sirsi-pantheon`

---

### 4. 🤖 MCP Server (Built into CLI)
**Best for**: Developers using AI-enabled IDEs (Claude, Gemini, Copilot) who want their AI assistant to query Pantheon directly.

**What you get**:
- AI can scan your workspace without you running commands
- AI can check for ghosts, classify files, read Thoth memory
- Structured JSON responses — no copy-paste needed
- 5 tools: `scan_workspace`, `ghost_report`, `health_check`, `classify_files`, `thoth_read_memory`

**Requires AI?** Yes — this layer is exclusively for AI coding assistants.

**Install**: Add to your AI IDE config:
```json
{
  "mcpServers": {
    "pantheon": {
      "command": "sirsi",
      "args": ["mcp"]
    }
  }
}
```

---

## Recommended Combinations

| Developer Profile | Recommended Stack |
|:---|:---|
| **VS Code / Antigravity user** | Extension + Menu Bar |
| **JetBrains / Xcode / Sublime user** | Menu Bar + CLI |
| **Terminal-only (vim/emacs)** | CLI + Menu Bar |
| **AI-paired developer** | Extension + Menu Bar + MCP |
| **DevOps / CI engineer** | CLI only (with `--json`) |
| **Fleet administrator** | CLI + Agent (future: Ra) |

---

## Smart Installer Detection (Coming Soon)

The `sirsi install` command will auto-detect your environment and offer the right tools:

| Detection | Offered |
|:---|:---|
| VS Code / Antigravity / Cursor installed | → Extension auto-install |
| macOS detected | → Menu Bar app + LaunchAgent |
| AI IDE detected (Claude, Gemini config) | → MCP server configuration |
| `brew` available | → Homebrew tap setup |
| Linux / no GUI | → CLI only |

---

## Binary Sizes

| Component | Download | On Disk | Budget |
|:---|:---|:---|:---|
| CLI (`sirsi`) | **6.6 MB** | **12 MB** | < 15 MB ✅ |
| Menu Bar (`sirsi-menubar`) | **2.6 MB** | **4.5 MB** | < 8 MB ✅ |
| Extension (`.vsix`) | **40 KB** | **40 KB** | < 100 KB ✅ |
| Agent (`sirsi-agent`) | **~1.2 MB** | **2.1 MB** | < 5 MB ✅ |

Zero external dependencies. Zero telemetry. Zero network calls (except GitHub update check).
