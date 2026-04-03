# ADR-011: Deity Alignment & Context Architecture

**Status:** Accepted  
**Date:** 2026-03-25  
**Deciders:** Cylton Collymore, AI Agent  

## Context

Three indexing systems were redundantly scanning the same filesystem:
- Antigravity's Language Server (2.76 GB)
- Horus shared filesystem index (100 MB)
- Thoth session memory (~50 MB)

This caused 3 GB of RAM waste, memory bandwidth contention, GC pauses,
and perceptible click latency in the IDE.

## Decision

### Deity Canonical Scopes

| 𓇶 **Ra** | The Hypervisor | Supreme Overseer & Orchestrator of Creation |
| 𓁯 **Net** | The Weaver | Weaver of Existence — Owners Plan, Canon & Timelines |
|-------|-----------|----------------------|
| **𓀭 The Code Gods** | **Governance & Knowledge** | **Plan, Quality, Memory, Healing** |
|-------|-----------|----------------------|
| **𓁟 Thoth** | The Memory | Context compressor — ensures no consumer reads raw history |
| **𓆄 Ma'at** | The Feather | Quality & truth validation, weighing of the heart |
| **𓆄 Isis** | The Healer | Autonomous remediation — restores quality after Ma'at's weigh |
|-------|-----------|----------------------|
| **𓀰 The Machine Gods** | **Infrastructure & OS** | **Safety, Resource, Network, Hardware** |
|-------|-----------|----------------------|
| **𓂀 Horus** | The Watcher | Publisher (HTML/GitHub) + demand-driven filesystem index |
| **🐺 Jackal/Anubis** | The Hunter | Waste scanner & Infrastructure Hygiene Judge |
| **⚠️ Ka** | The Spirit | Ghost detector — finds remnants of uninstalled apps |
| **𓁵 Sekhmet** | The Warrior | Process control — watchdog, orphan slayer, NPU hardening |
| **𓈗 Hapi** | The River | GPU/VRAM and NPU (Accelerator) flow management |
| **🪲 Scarab** | The Roller | Fleet discovery — subnets, VLANs, containers |
| **🔮 Seba** | The Star | Dependency graph & Topology mapper |
| **𓂀 Horus** | The Watcher | Publisher (HTML/GitHub) + demand-driven filesystem index |
| **🐺 Jackal** | The Hunter | Waste scanner — uses Horus index as read-only data source |
| **⚠️ Ka** | The Spirit | Ghost detector — finds remnants of uninstalled apps |
| **𓁵 Sekhmet** | The Warrior | Process control — RAM watchdog, orphan slayer, renice |
| **𓆄 Ma'at** | The Feather | Quality & truth validation, platform integrity |
| **𓈗 Hapi** | The River | GPU/VRAM and storage flow management |
| **🪲 Scarab** | The Roller | Fleet discovery — subnets, VLANs, containers |
| **🔮 Seba** | The Star | Dependency graph mapper |
| **𓇶 Ra** | The Thinker | Hypervisor — orchestrates all deities (FUTURE) |

### Key Changes

1. **Horus Phase 3**: Reduced from 14 broad roots to 8 targeted hygiene paths.
   Added `FullScanRoots()` for deep scans and `Release()` for context cleanup.

2. **Guard Renice**: New `pantheon guard --renice lsp` command uses `renice(1)`
   and `taskpolicy(1)` to deprioritize LSP processes so the Renderer gets
   uncontested P-core access.

3. **Thoth remains context-only**: No filesystem scanning. Compresses and serves
   session memory via MCP.

## Consequences

- Horus default scan drops from ~856K files to ~50K files
- Language Server processes run on E-cores under contention
- Click latency reduced by eliminating P-core competition
- `pantheon guard --renice lsp` must be re-run after IDE restart
  (priority resets with the process)
