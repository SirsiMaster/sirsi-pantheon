# Architecture Design — Sirsi Pantheon
**Version:** 2.1.0
**Date:** March 29, 2026
**Custodian:** 𓁯 Net (The Weaver)

---

## 1. System Overview

Sirsi Pantheon is a unified infrastructure intelligence and DevSecOps platform built on a **Deity-First Architecture**. It operates on the "One Install. All Deities." standard, where a single hardened binary manages the entire workstation and fleet lifecycle.

### 1.1 The Six Master Pillars
The Pantheon is organized into six divine pillars, each assigned a canonical Ancient Egyptian Hieroglyph. This consolidation removes architectural fragmentation and ensures absolute aesthetic purity across all interfaces.

- **𓂀 ANUBIS (Hygiene)**: Infrastructure hygiene, ghost app hunting (Ka), file deduplication (Mirror), and resource protection (Guard).
- **𓁐 MA'AT (Governance)**: Codebase auditing (Scales), QA standards, and autonomous remediation (Isis).
- **𓁟 THOTH (Knowledge)**: Persistent project memory, rule-grounded intelligence, and the zero-token brain ledger.
- **𓈗 HAPI (Compute)**: Hardware optimization, GPU/VRAM flow, and ANE/NPU acceleration (Sekhmet).
- **𓇼 SEBA (Mapping)**: Infrastructure topology, project registry (Book), and fleet discovery (Scarab).
- **𓁆 SESHAT (Scribe)**: Knowledge bridge (Gemini/NotebookLM), MCP context server, and AI sync.

```
                    ┌─────────────────────────────┐
                    │         USER / ADMIN         │
                    │     (runs `pantheon` CLI)    │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │       PANTHEON PLATFORM     │
                    │                             │
                    │  ┌────────┐  ┌───────────┐  │
                    │  │ Anubis │  │   Ma'at   │  │
                    │  │(Clean) │  │  (Truth)  │  │
                    │  └────────┘  └───────────┘  │
                    │  ┌────────┐  ┌───────────┐  │
                    │  │ Thoth  │  │   Hapi    │  │
                    │  │(Memory)│  │ (Power)   │  │
                    │  └────────┘  └───────────┘  │
                    │  ┌────────┐  ┌───────────┐  │
                    │  │  Seba  │  │  Seshat   │  │
                    │  │ (Map)  │  │ (Bridge)  │  │
                    │  └────────┘  └───────────┘  │
                    │                             │
                    │       Transport Layer       │
                    │  (SSH / gRPC / kubectl)     │
                    └──────┬──────┬───────┬───────┘
                           │      │       │
                    ┌──────▼─┐ ┌──▼────┐ ┌▼────────┐
                    │ agent  │ │ agent │ │  agent   │
                    │ (VM)   │ │ (Pod) │ │  (NAS)   │
                    └────────┘ └───────┘ └──────────┘
```

---

## 2. Pillar Architecture

### 2.1 Anubis — The Hygiene Pillar (𓂀)
- **Engine:** Jackal (File Scanning), Ka (Ghost Detection).
- **Scope:** Workstation hygiene, cache purging, orphan application hunting.
- **Functions:** `weigh`, `judge`, `ka`, `mirror`, `guard`.

### 2.2 Ma'at — The Governance Pillar (𓁐)
- **Engine:** Scales (Policy Auditing), Isis (Remediation).
- **Scope:** Code quality, ADR compliance, autonomous healing of lint/test wounds.
- **Functions:** `audit`, `scales`, `heal`.

### 2.3 Thoth — The Intelligence Pillar (𓁟)
- **Engine:** Brain (Neural Weights), Ledger (Persistent Context).
- **Scope:** Zero-token context savings, rule-grounded AI intelligence.
- **Functions:** `sync`, `install-brain`.

### 2.4 Hapi — The Compute Pillar (𓈗)
- **Engine:** Sekhmet (ANE Acceleration), Yield (Resource Management).
- **Scope:** GPU/VRAM optimization, hardware profiling, NPU-driven workflows.
- **Functions:** `scan`, `profile`, `compute`.

### 2.5 Seba — The Mapping Pillar (𓇼)
- **Engine:** Scarab (Discovery), Book (Project Registry).
- **Scope:** Visual dependency graphs, fleet discovery, VLAN/subnet mapping.
- **Functions:** `scan`, `book`, `fleet`.

### 2.6 Seshat — The Scribe Pillar (𓁆)
- **Engine:** Gemini Bridge, MCP Server.
- **Scope:** Bidirectional sync between Gemini, NotebookLM, and Antigravity IDE.
- **Functions:** `sync`, `list`, `server`.

---

## 3. Deployment & Distribution

### 3.1 One Install. All Deities.
The `pantheon` binary is the single source of truth. It is statically compiled for macOS (ARM64/Intel) and Linux.

### 3.2 Registry (docs/index.html)
The public registry provides a high-fidelity visual map of the 6 Master Pillars. All icons and metrics are dynamically reported from the CLI's internal stats engine.

---

## 4. Data Flow Architecture ⚠️ MANDATORY (Neith's Triad §1)

```mermaid
graph TD
    subgraph "User Layer"
        USER["𓀀 User / Admin"]
    end

    subgraph "Unified Entry Point"
        PANTHEON["cmd/pantheon — Unified Platform"]
    end

    subgraph "The Six Master Pillars"
        ANUBIS["𓂀 Anubis (Hygiene)"]
        MAAT["𓁐 Ma'at (Truth)"]
        THOTH["𓁟 Thoth (Wisdom)"]
        HAPI["𓈗 Hapi (Power)"]
        SEBA["𓇼 Seba (Maps)"]
        SESHAT["𓁆 Seshat (Bridge)"]
    end

    subgraph "Safety & Cleaner"
        SAFETY["⛔ Safety Module"]
        CLEANER["🧹 Cleaner Engine"]
    end

    USER --> PANTHEON
    PANTHEON --> ANUBIS
    PANTHEON --> MAAT
    PANTHEON --> THOTH
    PANTHEON --> HAPI
    PANTHEON --> SEBA
    PANTHEON --> SESHAT

    ANUBIS.JACKAL --> CLEANER
    CLEANER --> SAFETY
    MAAT.ISIS --> THOTH
    SESHAT <--> GEMINI["Gemini AI Mobile/Web"]
```

---

*𓁯 This document follows Neith's Architecture Triad (Rule A22). Updated to v2.1.0 for the Great Pantheon Consolidation (v0.7.1-alpha).*
