# ADR-001: Founding Architecture — Sirsi Anubis

## Status
**Accepted** — March 20, 2026

## Context
Sirsi Technologies identified a critical gap in the infrastructure tooling landscape: no existing tool — commercial (CleanMyMac, DaisyDisk, OnyX) or open-source (Mole) — adequately addresses the cleanup needs of developer workstations, AI/ML environments, or fleet-scale infrastructure.

This was discovered during a manual cleanup session that revealed:
- 91 ghost Windows apps from Parallels polluting Spotlight
- 38 orphaned Node.js processes consuming 5.7 GB of RAM
- 17.3 GB of stale language server processes from IDE workspaces
- 12+ macOS subsystem directories containing Parallels remnants that no tool detected

The solution requires:
1. Deep knowledge of developer and AI toolchain artifacts (60+ scan categories)
2. Virtualization platform remnant scanning (Parallels, Docker, VMware)
3. RAM and VRAM pressure management
4. Fleet-scale scanning across VLANs, subnets, containers, VMs, and SAN/NAS
5. Policy-driven enforcement across infrastructure

## Decision

### 1. Language: Go 1.22+
Single static binary compilation, cross-platform (macOS, Linux, Windows — arm64 + amd64), excellent CLI ecosystem (cobra, lipgloss, bubbletea), contributor-friendly language with low barrier to entry.

### 2. CLI Framework: cobra
Industry standard for Go CLI tools. Provides subcommands, auto-complete, help generation, and shell integration.

### 3. Architecture: Agent-Controller Model
Two binaries:
- `anubis` (~20 MB) — full CLI controller/orchestrator
- `anubis-agent` (<10 MB) — lightweight agent deployed to targets

Communication via gRPC with SSH+JSON fallback.

### 4. Internal Modules
Named after Egyptian/Kemetic/Nubian mythology:
| Module | Codename | Role |
| :--- | :--- | :--- |
| Local Scanner | **Jackal** | Workstation cleaning |
| Fleet Sweep | **Scarab** | Network/VLAN/container/VM sweep |
| Policy Engine | **Scales** | Fleet-wide rule enforcement |
| Resource Optimizer | **Hapi** | VRAM/storage optimization |

### 5. License: MIT
Free and open source forever. Revenue (if pursued) comes from a future GUI product, not the CLI engine.

### 6. Distribution: Homebrew + goreleaser
- `brew tap SirsiMaster/tools && brew install sirsi-anubis`
- GitHub Releases with pre-built binaries for all platforms
- Docker Hub image for agent deployment

### 7. Reserved Sibling Products
- **Sirsi Rook** — Database & storage orchestration
- **Sirsi Rogue** — Cybersecurity sweeper

## Alternatives Considered

1. **Rust** — Superior performance, better memory safety, and ownership model. Rejected because Go has a significantly lower contribution barrier for an open-source project, Go's performance is more than adequate for filesystem and network operations, and Go's `goreleaser` ecosystem is unmatched for multi-platform binary distribution.

2. **Python** — Largest ecosystem for scripting and automation. Rejected because Python is too slow for fleet-scale scanning, packaging/distribution is painful (no single binary), and requires a runtime on target machines (incompatible with agent deployment).

3. **Contributing to Mole** — Mole is an established open-source Mac cleaner. Rejected because Mole's architecture doesn't support agent-controller fleet management, its scan rule system isn't designed for the depth we need (virtualization, AI/ML, VRAM), and the project's scope is fundamentally different (consumer cleaner vs. developer workstation manager).

4. **Native macOS app (Swift/SwiftUI)** — Premium user experience. Rejected as the initial approach because CLI-first allows faster iteration, broader platform support, and community contribution. GUI deferred to Phase 7 (Temple) as a wrapper around the CLI engine.

## Consequences

- **Positive**: Single binary distribution simplifies installation, Go's compilation speed enables fast iteration, cobra's subcommand system maps perfectly to the Anubis module architecture (weigh/judge/guard/sight/scarab/scales/hapi), MIT license maximizes adoption.

- **Negative**: Go's error handling verbosity increases boilerplate code, Go's lack of enum types requires careful interface design for scan rule types, Go's module system has learning curve for new contributors.

- **Risk**: Scope creep across 7 phases — mitigated by strict phased roadmap where each phase has a defined module boundary. Phase 1 (Jackal) ships independently.

## References
- Product concept: `app_idea_deep_cleanse.md` (Antigravity brain)
- Competitive analysis: Mole (`tw93/Mole` on GitHub)
- Naming brainstorm: `naming_brainstorm.md` (Antigravity brain)
- SirsiNexusApp: `SIRSI_RULES.md` (governance pattern)
- FinalWishes: `CHANGELOG.md` (changelog pattern)
