# 𓂀 Session Continuation — Pantheon Extension Architecture (Session 19)

## Session Context
- **Status**: 🟢 All work pushed. Clean tree.
- **Version**: 0.5.0-alpha
- **Tests**: 819+ passing, all CI green (pending Windows verification)
- **Rules**: A1-A19 codified (A18=Incremental Commits, A19=No App Bundle Mutations)
- **ADRs**: 001–011 canonized (011 = Deity Alignment & Context Architecture)
- **Last Commit**: `8922114` — protect language_server_macos_arm from slay

## What Was Accomplished (Session 18-19)

### Session 18a: Horus Phase 2 Memory Optimization
- Purged `Entries map[string]Entry` from Manifest struct
- Reduced Pantheon CLI memory footprint: 2.4 GB → ~100 MB
- Hybrid Glob Strategy: Horus dirs first, filesystem fallback for files
- Ma'at Platform Integrity: `internal/maat/platform.go` (prevents hardware misreporting)

### Session 18b: Hot-Swap Catastrophe + Recovery
- Agent replaced IDE's `language_server_macos_arm` binary → crashed IDE
- Rule A19 codified: NEVER modify `/Applications/*.app/` bundles
- Case Study 010 published: full post-mortem

### Session 18c: Deity Alignment + Renice
- **ADR-011**: Canonical deity scopes (Thoth=context compressor, Horus=publisher+lazy FS index, Guard=process control)
- **Horus Phase 3**: Scoped indexing — 856K files → ~50K files (14 roots → 8 targeted)
- **`pantheon guard --renice lsp`**: Live deprioritized 3.1 GB of LSP processes to Background QoS
- **`language_server_macos_arm` protected**: Added to protected process list, excluded from slay targets
- **IDE Settings**: Shell Integration disabled, gopls directory filters, file watcher exclusions
- **CI Fix**: Removed tracked `pantheon-menubar` binary causing Windows test failures

### Key Lessons (Hardcoded)
1. `language_server_macos_arm` is Antigravity's core AI backend — killing it crashes the IDE
2. The IDE's 2.7 GB LSP heap can only be released by restarting the IDE, not from outside
3. The IDE IS multi-process (not single-threaded) — click latency comes from core contention
4. `renice`/`taskpolicy` are safe OS-level scheduling APIs — no binary modification needed
5. Triple-indexing (LSP + Horus + Thoth) is the root cause of memory bloat

## 🎯 SESSION 19 OBJECTIVE: Pantheon as an IDE Extension

### The Problem
Pantheon is not running inside Antigravity. The ankh (𓂀) is not present.
Users cannot install Pantheon as an extension. All our work (renice, scoped indexing,
context compression) only takes effect when manually invoked from the CLI.
This defeats the purpose: **Pantheon should operate without oversight.**

### Primary Goal
Build a VS Code / OpenVSX extension that packages Pantheon's capabilities
as an always-on IDE integration:

```
sirsi-pantheon-extension/
├── package.json          # Extension manifest (activationEvents, contributes)
├── src/
│   ├── extension.ts      # activate() — starts Guardian, Horus, Thoth
│   ├── guardian.ts        # Background renice + memory pressure monitor
│   ├── statusBar.ts       # Ankh (𓂀) icon in status bar with live metrics
│   ├── commands.ts        # Command palette: Pantheon: Scan, Guard, Renice
│   └── thothProvider.ts   # Context compression for AI conversations
├── resources/
│   ├── ankh.svg           # Status bar icon
│   └── ankh-alert.svg     # Alert state icon
└── tsconfig.json
```

### Extension Capabilities (Always-On)
1. **Guardian (background)**: Auto-renices LSP processes on startup + after IDE restart
2. **Status Bar Ankh**: Shows RAM pressure, deity status, active warnings
3. **Context Compression**: Thoth memory available as inline completion context
4. **Command Palette**: `Pantheon: Scan`, `Pantheon: Guard`, `Pantheon: Renice LSP`
5. **Workspace Settings**: Auto-applies optimal gopls filters, watcher exclusions

### OpenVSX Extensions to Evaluate
| Extension | Action | Rationale |
|-----------|--------|-----------|
| `golang.go` | ✅ Keep | Required for gopls integration |
| `esbenp.prettier-vscode` | ⚠️ Evaluate | Running but may not be needed for Go-only work |
| `ms-azuretools.vscode-docker` | ❌ Disable | Docker is uninstalled — this is dead weight |
| `ms-vscode-remote.remote-containers` | ❌ Disable | No containers in use — saves Extension Host memory |
| `pkief.material-icon-theme` | ⚠️ Two versions | Delete the older 5.8.0, keep 5.24.0 |
| `github.vscode-pull-request-github` | ✅ Keep | Used for PR management |

### Architecture Notes
- Extension activates on workspace open (`*` activation event)
- Spawns `pantheon guard --watch` as a child process
- Communicates via JSON-RPC over stdin/stdout (MCP protocol)
- Guardian auto-runs `renice lsp` 30s after activation (LSP needs time to spawn)
- Status bar updates every 5s with RAM/CPU metrics from Guard
- No telemetry (Rule A11) — all processing stays local

### Dependencies
- Node.js + TypeScript (standard VS Code extension)
- `@vscode/vsce` for packaging
- OpenVSX CLI for publishing
- Pantheon binary must be installed (`brew install sirsi-pantheon`)

## Current Deity Registry (ADR-011)

| Deity | Package | Role | Status |
|-------|---------|------|--------|
| 𓁟 Thoth | `internal/brain/` + `.thoth/` | Context Compressor | ✅ Active |
| 𓁹 Horus | `internal/horus/` | Publisher + Lazy FS Index | ✅ Active |
| 🐺 Jackal | `internal/jackal/` | Waste Scanner | ✅ Active |
| 𓂓 Ka | `internal/ka/` | Ghost Detector | ✅ Active |
| 𓁵 Sekhmet | `internal/guard/` | Process Control + Renice | ✅ Active |
| 𓆄 Ma'at | `internal/maat/` | Quality & Truth | ✅ Active |
| 🌊 Hapi | `internal/hapi/` | GPU/VRAM/Storage | ✅ Active |
| 🪲 Scarab | `internal/scarab/` | Fleet Discovery | ✅ Active |
| 🔮 Seba | `internal/seba/` | Dependency Mapper | ✅ Active |
| ☀️ Ra | `internal/ra/` | Hypervisor (Thinker) | 🔮 Future |

## Known Issues
1. **Windows CI**: Tracked binary removed, awaiting verification on next push
2. **2.7 GB LSP**: Cannot release without IDE restart. Renice mitigates.
3. **Extension Host limits**: Single process per extension — Pantheon extension must be lightweight
4. **go.mod lint**: `fyne.io/systray should be direct` warning (low priority)

## One-Line Starter for Next Session

> **"Continue from `docs/CONTINUATION-PROMPT.md` — Session 19: build the Pantheon VS Code extension (OpenVSX) with always-on Guardian, status bar ankh, context compression via Thoth, and auto-renice. The Anubis Suite must operate without oversight."**

---
**Last Updated**: March 25, 2026 — 20:21
**Session Count**: 19 (next)
