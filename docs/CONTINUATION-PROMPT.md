# 𓂀 Session Continuation — OpenVSX Publishing + MCP Integration (Session 22)

## Session Context
- **Status**: 🟢 All work pushed. Clean tree.
- **Version**: 0.5.0-alpha
- **Tests**: 23 packages, all passing, **85.7% avg coverage**
- **Rules**: A1-A19 codified
- **ADRs**: 001–012 canonized
- **Site**: Live at `sirsi-pantheon.web.app` + `pantheon.sirsi.ai`
- **Last Commit**: `c9e4ef1` — feat(extension): native renice, memory GC, codicon status bar

## What Was Accomplished (Session 21)

### Guardian Rewrite
- Replaced CLI-dependent renice with **native `renice(1)` + `taskpolicy(1)`**
- Guardian discovers LSP processes via `ps`, applies nice +10 and Background QoS directly
- Skips already-deprioritized processes (nice ≥ 10)
- Host LSP (`language_server_macos_arm`) excluded from warnings and GC

### Memory Pressure GC
- Tracks per-process RSS across poll cycles using a pressure map
- When a third-party LSP exceeds **500 MB for 3+ consecutive checks**, triggers VS Code's built-in LSP restart
- Maps process names to restart commands (gopls → `go.languageserver.restart`, etc.)
- Prevents ever-growing memory bloat from runaway LSPs

### Codicon Status Bar
- Replaced invisible hieroglyph `𓂀` with `$(eye) PANTHEON` codicons
- Loading spinner `$(loading~spin)` on init, warning icon on pressure
- Warning threshold: >1 GB third-party LSPs (host LSP at 4-6 GB is normal)

### Live Testing Results
- **Guardian auto-renice**: ✅ All 3 LSPs reniced to nice 10 after 30s delay
  - `language_server_macos_arm` (PID 28387): nice 0 → 10
  - `gopls` (PID 29214): nice 0 → 10
  - `gopls` (PID 29215): nice 0 → 10
- **Extension Host footprint**: ~199 MB RSS (Code Helper Plugin)
- **Sideloaded**: Installed in both Antigravity and VS Code
- **CLI Fix**: Commands now use correct flags (`weigh --dev --json`, `guard --json`)

## 🎯 SESSION 22 OBJECTIVES

### P0: Thoth Savings Dashboard (MOST IMPORTANT FEATURE)
1. **Status bar**: `𓁟 Thoth: 271K tokens saved ($4.08)` — calculated from file sizes
2. **Calculation**: `(all source files chars / 4) - (memory.yaml chars / 4)` = tokens saved
3. **Dollar estimate**: tokens saved × $15/M (Opus input pricing)
4. **Staleness warning**: If memory.yaml is older than latest source edits → "⚠ Thoth stale"
5. **Context % meter**: `memory.yaml tokens / 200K context window` = % used
6. **Session ROI**: End-of-session summary showing savings vs. cost
7. **Surface in**: Status bar tooltip, System Metrics dashboard, and menu bar app

### P0: Host LSP Auto-Cleanse
1. **Problem**: `language_server_macos_arm` grows to 5+ GB and never releases memory
2. **Solution**: Extend Guardian GC to cleanse the host LSP under safe conditions:
   - **Idle detection**: No editor activity for 5+ min AND RSS > 4 GB → restart LSP
   - **Conversation boundary**: If Antigravity exposes "new conversation" event, trigger on that
   - **Manual command**: `Pantheon: Cleanse LSP Memory` in Command Palette
3. **Implementation**: Use `workbench.action.reloadWindow` or find Antigravity-specific LSP restart command
4. **Guard rails**: Never cleanse during active edits. Log RSS before/after. Notify user via status bar.
5. **Research**: Check if Antigravity exposes conversation lifecycle events in its extension API

### P0: Publish to OpenVSX
1. Create OpenVSX API token at open-vsx.org (SirsiMaster Chrome profile)
2. Publish: `npm run publish:openvsx`
3. Add marketplace badges to README
4. Verify listing at open-vsx.org/extension/SirsiMaster/sirsi-pantheon

### P1: System Metrics Expansion
1. **macOS Memory Pressure** — read from `memory_pressure` or `vm_stat` (green/yellow/red)
2. **Overall RAM Usage** — total system RAM used vs. available (not just LSPs)
3. Display both in status bar tooltip and System Metrics dashboard
4. Surface pressure state in the status bar icon (healthy → pressure → critical)

### P1: MCP Integration
1. Connect Guardian to MCP resources (replace CLI spawn with MCP JSON-RPC)
2. Add `anubis://guardian-status` MCP resource
3. Sidebar TreeView data provider for real-time metrics
4. Wire memory GC stats into MCP resource

### P2: Version Bump + Release
1. Bump VERSION from `0.5.0-alpha` to `0.5.1-alpha` (or `0.6.0-alpha` if MCP wired)
2. GoReleaser: tag and publish GitHub release
3. Update Homebrew formula with new SHA

### P3: Audit + Performance
1. Evaluate installed VS Code extensions per Session 18 audit table
2. Disable unnecessary extensions (Docker, Remote Containers, etc.)
3. Measure Extension Host memory before/after optimization
4. Test macOS menu bar app (ADR-010 Phase 1)

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
1. **OpenVSX not published** — needs API token setup via SirsiMaster Chrome profile
2. **MCP not wired** — extension uses CLI spawn + native renice, not MCP JSON-RPC
3. **Menu bar app** — built (ADR-010) but never sideloaded/tested
4. **`weigh` command slow** — `pantheon weigh --dev --json` may timeout in extension (60s limit)

## One-Line Starter for Next Session

> **"Continue from `docs/CONTINUATION-PROMPT.md` — Session 22: publish the Pantheon extension to OpenVSX, wire MCP integration for Guardian metrics, and tag a GitHub release."**

---
**Last Updated**: March 26, 2026 — 17:05
**Session Count**: 22 (next)
