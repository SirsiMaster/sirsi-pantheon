# ADR-005: Pantheon — Unified DevOps Intelligence Platform

## Status
**Accepted** — 2026-03-23

## Context
Sirsi Technologies has built multiple deity-themed tools independently:
- **Anubis** (Go) — workstation hygiene (scan, clean, dedup, ghost hunting, GPU, network)
- **Thoth** (JavaScript) — persistent AI knowledge system (memory, journal, context savings)
- **Ma'at** (Go, within Anubis) — QA/QC governance (coverage, canon, pipeline)

Each tool solves a real problem, but they're deployed separately: `brew install sirsi-pantheon`, `npx thoth-init`, and Ma'at only exists within Anubis. Users need to know about each tool individually. There's no single install, no unified brand, and no shared runtime.

Meanwhile, every repo in the Sirsi portfolio needs ALL of these tools — not just one. Session 9 proved this: deploying Thoth and Ma'at governance across 5 repos required manual work per repo.

The question: what if the product isn't any individual deity — it's the **Pantheon** itself?

## Decision

**Pantheon** is the unified DevOps intelligence platform. It is the package, the brand, the web presence, and the single install that provides all deity agents.

### Core principles

1. **Pantheon is the package.** All deities are sub-systems within Pantheon. Installing Pantheon gives you all deities.

2. **Deities keep their own repos and versions.** Anubis remains v0.3.0-alpha. Thoth remains v1.0.0. Each deity matures independently. Pantheon wraps them without replacing them.

3. **Deities are NOT limited to one framework.** Thoth is JavaScript. Anubis is Go. Future deities may be Python, Rust, or anything. The Pantheon is polyglot by design. Each deity is built with the best tool for its domain.

4. **Pantheon is the monorepo and web presence.** Individual deity repos are sub-repos of Pantheon. Sirsi is the super-repo (the company). Hierarchy: `Sirsi > Pantheon > [Anubis, Thoth, Ma'at, ...]`

5. **The name covers all current and future deity agents.** New deities are added as they're created and matured. The Pantheon grows organically.
    - **𓇳 Ra**: Hypervisor service manager and all-seeing overseer. Oversees all deities, nodes, services, agents, networks, subnets, vlans, vms, containers, processes, and users.
    - **𓏞 Seba**: Powerful mapping feature (active research/focus).
    - **𓆄 Ma'at**: Observation and assessment (remediation passed to other acting deities).

6. **Independent Operation & Deployment.** All deities CAN and SHOULD operate independently. Users from the public can download any single deity (e.g., `npx thoth-init` or `brew install anubis`) without requiring the entire Pantheon. Deities can also be deployed to repositories individually without "platooning" the entire suite.

7. **Inter-Deity Referencing (Referral Logic).** Findings offered by any deity should allude to whether another deity can provide the necessary action to create a result or mitigate a finding. This creates a cross-referenced ecosystem while maintaining modular independence.

### Architecture

```
Sirsi Technologies (super-repo / company)
└── Pantheon (product / monorepo / brand)
    ├── 𓇳 Ra        — Hypervisor (future) — v0.1.0-alpha
    ├── 𓏞 Seba      — Mapping (Go)        — within Anubis
    ├── 𓂀 Anubis    — Hygiene (Go)        — v0.3.0-alpha
    ├── 𓁟 Thoth     — Knowledge (JS/Go)   — v1.0.0
    ├── 🪶 Ma'at     — Governance (Go)     — v0.1.0
    ├── ⚖️ Scales    — Policy (Go)         — within Anubis
    ├── 🪲 Scarab    — Network (Go)        — within Anubis
    ├── 𓈗 Hapi      — Resources (Go)      — within Anubis
    ├── 𓅓 Horus     — System Sentinel (Go) — within Anubis (was Guard)
    ├── 🪞 Mirror    — Dedup (Go)          — within Anubis
    ├── ⚠️ Ka         — Ghost Hunting (Go)  — within Anubis
    ├── 👁️ Sight     — LaunchServices (Go) — within Anubis
    ├── 𓄿 Isis      — [Undesignated]      — (pending)
    ├── 𓀭 Osiris    — [Undesignated]      — (pending)
    └── [future deities as needed]

```

### What Pantheon manages

| Layer | Dev Cost Reduction | Ops Cost Reduction |
|:------|:------------------|:-------------------|
| **Knowledge** (Thoth) | Token savings, context reduction | — |
| **Quality** (Ma'at) | Canon enforcement, coverage gaps | Pipeline health |
| **Hygiene** (Anubis) | Disk waste, ghost apps | Storage, cleanup |
| **Resources** (Hapi) | — | GPU/VRAM, memory |
| **Policy** (Scales) | Code compliance | Infrastructure thresholds |
| **Network** (Scarab) | — | Fleet discovery, subnet scanning |
| **Memory** (Guard) | — | RAM pressure, process management |

### Release strategy

- Each deity releases independently at its own version
- Pantheon has its own version that tracks the bundle
- Deities can be installed standalone OR via Pantheon
- `npx thoth-init` continues to work independently
- `pantheon` CLI wraps all Go-based deities
- Non-Go deities distribute through their native channels AND through Pantheon

### Investor pitch reframe

> "Pantheon is a DevOps intelligence platform. One install deploys autonomous agents that manage workstation hygiene, code quality, AI context efficiency, and infrastructure policy. Each agent is named after an Egyptian deity — each is a domain expert. They share knowledge through Thoth, are governed by Ma'at's quality standard, and run locally with zero telemetry."

## Alternatives Considered

1. **Keep tools separate** — Individual branding per tool (Anubis, Thoth, Ma'at). Rejected because Session 9 proved every repo needs ALL tools. Separate branding creates discovery and deployment friction.

2. **Merge everything into one Go binary** — Force all deities into Go. Rejected because Thoth's JS distribution (`npx thoth-init`) is a feature. Polyglot design lets each deity use the best tool for its domain.

3. **Rename Anubis to Pantheon** — Just rebrand the CLI. Rejected because Pantheon is bigger than Anubis. Anubis is one deity within the Pantheon. The Pantheon is the platform.

## Consequences

- **Positive**: Single brand, single install, clear product story. Every deity benefits from the Pantheon umbrella. New deities slot in naturally. Investor pitch is dramatically stronger.
- **Positive**: Deities keep their independence. Thoth still works as `npx thoth-init`. Anubis still tracks its own version. No forced migration.
- **Negative**: Needs a Pantheon repo/monorepo to orchestrate releases. Additional packaging work.
- **Risk**: Scope creep — "manages everything" is a big promise. Mitigated by releasing deity-by-deity as each matures.

## References
- [ADR-001](ADR-001-FOUNDING-ARCHITECTURE.md) — Founding architecture (deity module codenames)
- [ADR-004](ADR-004-MAAT-QA-GOVERNANCE.md) — Ma'at as first governance agent
- [SIRSI_PORTFOLIO_STANDARD](SIRSI_PORTFOLIO_STANDARD.md) — Universal governance (Pantheon enforces this)
- [CONTINUATION-PROMPT](CONTINUATION-PROMPT.md) — Session 9 context
