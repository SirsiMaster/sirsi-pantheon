# ADR-012: Pantheon VS Code Extension (OpenVSX)

**Status:** Accepted
**Date:** March 25, 2026
**Decision Makers:** Cylton Collymore

## Context

Pantheon has matured into a 24-module ecosystem with renice, scoped indexing, context compression, and process control. However, all capabilities require manual CLI invocation. The system cannot operate without oversight — the user must remember to run `sirsi guard --renice lsp` after every IDE restart.

### Problem
- Renice resets on process restart — needs re-application per IDE session
- Status metrics (RAM, CPU, LSP count) are invisible unless you run CLI commands
- Thoth context compression is only useful when manually copied
- Workspace optimization settings must be applied manually

### What We Tried
- ADR-010: Menu bar application — provides visibility but runs outside the IDE
- Manual CLI workflows — effective but require discipline

## Decision

Build a VS Code / OpenVSX extension (`extensions/vscode/`) that packages Pantheon's capabilities as an **always-on** IDE integration. The extension operates without oversight (the Anubis Suite principle).

### Architecture

```
extensions/vscode/
├── package.json          # Extension manifest
├── tsconfig.json         # TypeScript config
├── src/
│   ├── extension.ts      # activate() — starts Guardian, Horus, Thoth
│   ├── guardian.ts        # Background renice + memory pressure monitor
│   ├── statusBar.ts       # Ankh (𓁢) icon with live metrics
│   ├── commands.ts        # Command palette (7 commands)
│   └── thothProvider.ts   # Context compression from .thoth/memory.yaml
├── resources/
│   ├── ankh.svg           # Status bar icon
│   └── ankh-alert.svg     # Alert state icon
└── out/                   # Compiled JavaScript (gitignored)
```

### Key Design Decisions

1. **Lightweight metric collection**: Status bar polls `ps -axo` directly (not the Pantheon binary) every 5s for sub-50ms overhead
2. **Delayed renice**: Guardian waits 30s after activation before first renice — LSPs need time to spawn
3. **Re-renice loop**: Guardian re-applies renice every 60s to catch respawned/reset processes
4. **No MCP yet**: Extension spawns CLI commands — MCP integration deferred to future
5. **Zero telemetry**: All processing stays local (Rule A11)
6. **Binary dependency**: Extension requires `sirsi` binary (`brew install sirsi-pantheon`). Graceful degradation with ENOENT messaging if not installed.

### Extension Capabilities

| Capability | Trigger | Mechanism |
|-----------|---------|-----------|
| Auto-renice | On activation (30s delay) | `sirsi guard --renice lsp` |
| Status bar metrics | Every 5s | Direct `ps -axo` polling |
| Workspace scan | Command palette | `sirsi weigh --json` |
| Ghost report | Command palette | `sirsi ka --json` |
| Thoth context | Command palette | Reads `.thoth/memory.yaml` |
| Workspace optimization | Command palette | VS Code settings API |

### Status Bar States

| State | Icon | Meaning |
|-------|------|---------|
| Healthy | `𓁢 1.2 GB` | Guardian active, normal RAM |
| Warning | `𓁢 3.5 GB ▲` | RAM > 3 GB, yellow background |
| Error | `⚠️ Pantheon` | Binary not found or system error |
| Initializing | `⏳ Pantheon` | First metric fetch pending |

## Consequences

### Positive
- Pantheon operates without oversight — renice is automatic
- Live visibility into system health via status bar
- Thoth context available without leaving the IDE
- Workspace optimization is one click instead of manual JSON editing

### Negative
- Extension Host memory cost (~15-25 MB for the extension process)
- Dependency on external binary (not self-contained)
- ps polling adds minimal but non-zero CPU load

### Risks
- Extension Host runs in a single process — if Guardian hangs, it affects all extensions
- Mitigation: All CLI calls have hard timeouts (5-60s)

## References
- ADR-010: Menu Bar Application (complementary — runs outside IDE)
- ADR-011: Deity Alignment (Guardian = Sekhmet, Thoth = context compressor)
- Rule A11: No telemetry
- Rule A19: No application bundle mutations
