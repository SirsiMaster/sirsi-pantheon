# Product

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

## Users

Pantheon is for an individual operator and their trusted collaborators running Sirsi on one or more Macs. They need to understand a living local and distributed estate, choose work deliberately, run it safely, resolve failures without falling into a shell, and inspect the proof that a result is complete.

## Product Purpose

Pantheon is the complete Sirsi macOS application: a native workspace, companion menu-bar surface, and CLI that make the operator's actual system, work, evidence, releases, models, and connected instances understandable and controllable. Success means an operator can start at a real condition, take an end-to-end action, see the result and its evidence, and recover through guided resolution when it does not complete.

## Positioning

Pantheon does not merely summarize separate utilities. Ma'at continuously turns live state and retained evidence into a bounded, attributable next action; Stack Lab makes every module, recipe, decision, candidate, and release inspectable and runnable; Ra exposes the fabric without turning each Mac into a competing router. The same truthful state is available through the native app, menu bar, CLI, and automation surfaces.

## Operating Context

The primary surface is a native macOS application used at a desk while code, build systems, models, releases, and connected devices are active. The menu bar is an always-available companion that opens the same workspace rather than a separate, dead-end mini-product. Stack Lab is a functional local recipe system: users browse and compose components, choose a machine, inference engine, resident model, CPU, memory, swap, and chip estates, then move into Apollo telemetry. Apollo reports useful measured state including tokens per second, bandwidth, memory, network saturation, GPU/CPU residency, and selectable chip estates. 

Ra unifies Horus instances into a fabric and owns routing; Hermes is the information interconnect; Photon is the hardware-transfer device; Apollo and Apollo Flash are the plain and speculative inference engines. A machine hosting Pantheon is its local authority, while preserving the fabric's shared evidence and routing contracts.

## Capabilities and Constraints

- Ma'at is the installed, local, JEV-like baseline intelligence. It assesses build quality, CI, Stack Lab checks, contention, requests, health, and remediation choices without requiring a cloud LLM.
- Ma'at absorbs Seshat's user-facing knowledge/decision role. Each issue must show its cause, safe resolution path, confirmation state, and evidence. Users must never be stranded with a status they cannot resolve or consciously accept.
- Stack Lab, Ma'at, and release engineering are one methodology: every component can be catalogued, evidenced, reviewed, replaced, and released independently while remaining part of a coherent recipe.
- SNE inference qualification and model work remain inference-owned. Pantheon can compose qualified capability but must not invent qualification.
- Native, CLI, menu-bar, dashboard, and automation paths must share canonical engine truth. No dead-end command shells, ghost flows, duplicate menu-bar instances, or copied router stores.
- The product is a redistributable application that adapts to the local Mac and can participate in the wider Horus/Ra fabric; a specific machine is not globally canonical.

## Brand Commitments

The product name is Sirsi Pantheon. The canonical Sirsi mark at `docs/assets/sirsi-logo-white.png` is the identity asset for the native app, package, and menu bar. Pantheon is black-first: black and near-black own the surfaces; emerald indicates live, healthy, or selected state sparingly; gold carries Sirsi identity and consequential actions. The visual tone is clean, calm, legible, and operational—not a glowing hacker dashboard, not a CLI/TUI skin, and not an ornamental control room.

## Evidence on Hand

- Canonical application mark: `docs/assets/sirsi-logo-white.png`.
- Native Swift application: `macapp/Sources/SirsiMenubar/`.
- Current product shell: `macapp/Sources/SirsiMenubar/PantheonDesktopView.swift`.
- Apollo planner and telemetry: `macapp/Sources/SirsiMenubar/ApolloRunPlannerView.swift`.
- Stack Lab surfaces: `macapp/Sources/SirsiMenubar/StackLabView.swift` and `StackLabCatalogView.swift`.
- Ma'at casework: `macapp/Sources/SirsiMenubar/MaatCasebookView.swift`.
- Release architecture record: `docs/stacklab/pantheon-pt/canon/DESIGN.md`.
- Shipped commercial source v0.24.89 is now an ancestor of `origin/main` through merge commit `3d4c7e8db9701f6d1424b2cb6bbd52422296111b`.

## Product Principles

1. **Resolution is a feature.** A finding always leads to an owned, bounded next step, a guided escalation, or a clearly recorded acceptance.
2. **One action, one truth.** Native controls invoke the same canonical engine and return the same evidence as CLI and automation.
3. **Show the machine, not an abstraction.** Plans, telemetry, capacity, evidence, and release state use current measured data and disclose unavailable data.
4. **Composable without fragmentation.** Stack Lab exposes individual components and recipes while the application preserves one coherent operator journey.
5. **Quiet authority.** Strong hierarchy and real outcomes beat alert noise, ornamental dashboards, and unexplained badges.

## Accessibility & Inclusion

The macOS application must preserve keyboard navigation, VoiceOver labels and state, native focus behavior, readable Dynamic Type, high-contrast dark presentation, reduced-motion alternatives, and clear recovery copy. Every actionable UI control must have an obvious result and a safe disabled/loading/error state.
