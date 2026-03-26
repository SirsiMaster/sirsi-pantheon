# 𓂀 Pantheon — Anubis Suite

**Always-on infrastructure hygiene for your IDE.** Auto-renice, RAM guardian, memory GC, context compression. The Anubis Suite operates without oversight.

## Features

### 🛡️ Always-On Guardian
- **Auto-renice**: Deprioritizes LSP processes (gopls, tsserver, rust-analyzer) to Background QoS via native `renice(1)` + `taskpolicy(1)`
- **Memory GC**: Tracks per-process RSS across poll cycles — when a third-party LSP exceeds 500 MB for 3+ consecutive checks, triggers VS Code's built-in LSP restart to reclaim memory
- **30s startup delay**: Gives LSPs time to initialize before intervention
- **60s re-apply loop**: Catches respawned or reset processes  
- Zero telemetry. Zero network calls. No CLI binary dependency for renice.

### 👁️ Pantheon Status Bar
- Live RAM & CPU metrics polled every 5 seconds
- `$(eye) PANTHEON` — healthy display with codicons for visibility
- Color-coded health states: ✅ healthy / ⚠️ warning (>1GB third-party LSPs) / 🔴 error
- Click to open the full metrics panel

### 𓁟 Thoth Context Compression
- Reads `.thoth/memory.yaml` from your workspace
- Provides project context to AI assistants without re-reading source
- Watches for file changes and reloads automatically

## Commands

| Command | Description |
|---------|-------------|
| `Pantheon: Scan Workspace` | Run Anubis waste scan on the current workspace |
| `Pantheon: Start Guardian` | Start the background process guardian |
| `Pantheon: Renice LSP Processes` | Immediately renice all LSP processes |
| `Pantheon: Ghost Report (Ka)` | Detect ghost apps with leftover files |
| `Pantheon: Show Thoth Context` | Display compressed project context |
| `Pantheon: Show System Metrics` | Open the full system metrics panel |
| `Pantheon: Apply Optimal Settings` | Apply performance-optimized workspace settings |

## Requirements

- **Pantheon CLI** must be installed: `brew tap SirsiMaster/tools && brew install sirsi-pantheon`
- macOS (renice requires darwin/arm64 or darwin/amd64)
- VS Code 1.85.0+

## Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `pantheon.binaryPath` | `"pantheon"` | Path to the Pantheon CLI binary |
| `pantheon.guardian.enabled` | `true` | Enable always-on Guardian |
| `pantheon.guardian.reniceDelay` | `30` | Seconds before first renice |
| `pantheon.guardian.pollInterval` | `5` | Seconds between metric refreshes |
| `pantheon.thoth.enabled` | `true` | Enable Thoth context compression |

## Links

- [GitHub](https://github.com/SirsiMaster/sirsi-pantheon)
- [Deity Registry](https://pantheon.sirsi.ai)
- [Build Log](https://pantheon.sirsi.ai/build-log)
- [Sirsi Technologies](https://sirsi.ai)

## License

MPL-2.0 — [Full License](https://github.com/SirsiMaster/sirsi-pantheon/blob/main/LICENSE)
