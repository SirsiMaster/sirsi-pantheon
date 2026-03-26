# 𓂀 Session Continuation — Extension Testing + Publishing (Session 21)

## Session Context
- **Status**: 🟢 All work pushed. Clean tree.
- **Version**: 0.5.0-alpha
- **Tests**: 23 packages, all passing, **85.7% avg coverage**
- **Rules**: A1-A19 codified
- **ADRs**: 001–012 canonized
- **Site**: Live at `sirsi-pantheon.web.app` + `pantheon.sirsi.ai`
- **Last Commit**: `fd379e3` — fix(web): remove back-header from flip cards

## What Was Accomplished (Session 20)

### Firebase Deployment + Custom Domain
- Deployed 15 HTML pages to `sirsi-pantheon.web.app` via Firebase Hosting
- Created Firebase site `sirsi-pantheon` in project `sirsi-nexus-live`
- Wired custom domain `pantheon.sirsi.ai` via Firebase Hosting API + GoDaddy CNAME
- Firebase: `HOST_ACTIVE`, `OWNERSHIP_ACTIVE`, SSL auto-provisioning

### Flip Cards
- Rebuilt Deity Registry index with click-to-flip 3D CSS cards
- Front: user-facing (name, description, key metrics)
- Back: developer info (package path, coverage, test count, commands, deps, ADR)
- 3 action buttons per card: Full Page, Download, Source (GitHub internal/ link)
- Removed redundant card-back headers to maximize developer info space

### Canon Cleanup
- VERSION bumped from `0.4.0-alpha` to `0.5.0-alpha`
- Extension icon (`resources/icon.png`) created — Eye of Horus in gold on dark green
- CHANGELOG, Thoth memory, and continuation prompt updated
- All deity page nav links and URL displays fixed for Firebase paths

## 🎯 SESSION 21 OBJECTIVES

### P0: Extension Sideload + Live Testing
1. Package VSIX: `cd extensions/vscode && npm run package`
2. Sideload: `code --install-extension sirsi-pantheon-0.5.0.vsix`
3. Verify Guardian auto-renice — confirm LSPs deprioritized after 30s
4. Verify status bar metric accuracy — compare with Activity Monitor
5. Test each Command Palette entry end-to-end

### P1: Publish to OpenVSX
1. Create OpenVSX API token at open-vsx.org
2. Publish: `npm run publish:openvsx`
3. Add marketplace badges to README

### P2: MCP Integration
1. Connect Guardian to MCP resources (replace CLI spawn with MCP JSON-RPC)
2. Add `anubis://guardian-status` MCP resource
3. Sidebar TreeView data provider for real-time metrics

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
1. **Extension not yet sideloaded** — built and compiled but never tested in live IDE
2. **OpenVSX publish** — needs API token setup
3. **MCP not wired** — extension uses CLI spawn, not MCP JSON-RPC
4. **Menu bar app** — built (ADR-010) but never sideloaded/tested

## One-Line Starter for Next Session

> **"Continue from `docs/CONTINUATION-PROMPT.md` — Session 21: sideload the Pantheon VS Code extension, test Guardian auto-renice live, package VSIX, and publish to OpenVSX."**

---
**Last Updated**: March 25, 2026 — 22:00
**Session Count**: 21 (next)
