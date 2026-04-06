# Deity Registry — Canonical Glyph & Domain Reference
**Version:** 2.0.0 | **Date:** April 5, 2026 | **Custodian:** Net (The Weaver)

> This document is the single source of truth for deity identities, glyphs, and functional domains across the entire Sirsi portfolio. Every repo, hook, CLI output, and agent prompt must reference this registry. Misattributing a deity's glyph or function is a governance violation.

---

## Registry

| Deity | Glyph | Domain | Functional Responsibility | Reserved Symbols |
|-------|-------|--------|--------------------------|------------------|
| **Ra** | 𓇶 | Supreme Overseer | Multi-repo orchestration, agent deployment, window management, sprint governance | `𓂀` (ProtectGlyph — Ra's exclusive authority to mark windows as immune to KillAll) |
| **Net** | 𓁯 | Scope Weaver | Task definition, scope assembly, tiled context rendering, canon alignment, drift detection, development plan ownership | |
| **Thoth** | 𓁟 | Session Memory | Context compression, session persistence, memory sync, journal | |
| **Ma'at** | 𓆄 | Quality Gate | QA governance, quality gates, pre-push hooks, coverage audits, Feather Weight scoring | All pre-push gates across all repos are Ma'at's domain |
| **Isis** | 𓁐 | Health & Remediation | Doctor, network security, process guard, remediation engine, auto-fix lint/vet/coverage/canon drift, watchdog daemon, CPU/RAM monitoring, ANE hardening | |
| **Seshat** | 𓁆 | Knowledge Bridge | Knowledge grafting, ingestion/export, Gemini Bridge, NotebookLM sync, cross-platform knowledge | |
| **Anubis** | 𓃣 | Hygiene Engine | Infrastructure hygiene, waste scanning, policy enforcement, ghost app detection, residual hunting, file deduplication, semantic ranking | Jackal head (profile), NOT full-body jackal |
| **Hapi** | 𓈗 | Hardware Profiler | Hardware detection, GPU/ANE/CUDA profiling, resource optimization | |
| **Seba** | 𓇽 | Infra Mapper | Architecture mapping, topology visualization, dependency graphs, fleet discovery, subnet scanning, container audit | |
| **Osiris** | 𓁹 | Snapshot Keeper | State preservation, checkpoints, death/rebirth cycles, FinalWishes integration | NOT the quality gate — that's Ma'at |
| **Stele** | 𓊖 | The Ledger | Append-only hash-chained event bus, universal deity communication | Infrastructure, not a deity |

---

## Rules

### Rule D1: Deity Functions Are Universal
A deity's function is the same in every repo. Ma'at governs quality everywhere. Thoth manages memory everywhere. Ra orchestrates everywhere. No repo may reassign a deity's function to a different domain.

### Rule D2: Glyphs Are Identity
Each deity's glyph is their identity marker. Using the wrong glyph for a deity misrepresents the system. When displaying deity attribution in CLI output, hooks, or documentation, use the correct glyph from this registry.

### Rule D3: Ma'at Owns All Quality Gates
Every pre-push hook, CI gate, coverage check, and quality assessment across the entire Sirsi portfolio is Ma'at's domain. The output must be branded `𓆄 Ma'at` with the repo name in brackets: `𓆄 Ma'at pre-push gate... [RepoName]`. No other deity may be attributed for quality gate functions.

### Rule D4: The ProtectGlyph Is Ra's Authority
`𓂀` (Eye of Horus), when used as a Terminal.app window title marker, is Ra's exclusive ProtectGlyph. It means one thing: "this window is immune to KillAll during Ra deploy." Windows bearing `𓂀` in their custom title survive between sprints, redeploys, and kill-all operations. No other deity may use `𓂀` for window protection purposes.

### Rule D5: The Stele Is Universal Communication
All deities inscribe events to the Stele (`~/.config/ra/stele.jsonl`) via `stele.Inscribe()`. The Stele is the single source of truth for inter-deity communication. Every deity identifies itself by name in the `deity` field of Stele entries. The deity name must match this registry.

### Rule D6: Hierarchy Is Invariant
```
Ra (Supreme Overseer)
  └── Net (Scope Weaver — owns the plan)
        ├── Code Gods: Thoth, Ma'at, Isis, Seshat
        └── Machine Gods: Anubis, Hapi, Seba, Osiris
```
Ra supervises. Net aligns. Ma'at weighs. Isis heals. This cycle governs all work across all repos.

### Rule D7: No Repo-Specific Deity Aliases
A deity is never renamed for a specific repo. "Osiris (FinalWishes)" is wrong — Osiris is Osiris everywhere. Repos are identified by name in brackets, not by deity reassignment. Correct: `𓆄 Ma'at pre-push gate... [FinalWishes]`. Wrong: `𓁹 Osiris (FinalWishes) pre-push gate...`.

---

## Cross-Repo Application

This registry applies to:
- **sirsi-pantheon** — Source of truth. All deity implementations live here.
- **FinalWishes** — Consumes Ma'at (quality gates), Thoth (memory), Seshat (knowledge).
- **Assiduous** — Consumes Ma'at (quality gates), Thoth (memory), Seshat (knowledge).
- **SirsiNexusApp** — Consumes Ma'at (quality gates). Hosts shared infrastructure (Sirsi Sign, UCS).

Every `CLAUDE.md`, pre-push hook, CLI output, and Ra scope prompt must reference deities consistently with this registry.

---

*𓆄 The feather does not care which repo you push from. The standard is the standard.*
