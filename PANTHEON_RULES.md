# PANTHEON_RULES.md
**Operational Directive for All Development Agents (sirsi-pantheon)**
**Version:** 3.0.0 (v0.23.8-beta Release)
**Date:** July 1, 2026

---

## 0. Identity
This is the **sirsi-pantheon** repository — Sirsi Technologies' infrastructure hygiene platform.
An open-source CLI tool that scans, judges, and purges infrastructure waste across workstations, containers, VMs, networks, and storage backends.

- **GitHub**: `https://github.com/SirsiMaster/sirsi-pantheon`
- **Local Path**: `/Users/thekryptodragon/Development/sirsi-pantheon`
- **CLI Binary**: `sirsi`
- **Agent Binary**: `sirsi-agent`

### Platform Roadmap (ADR-032 — Mac-first)
Pantheon is **100% Mac**, built in this exact order: **(1) Mac CLI → (2) Mac Menubar → (3) Mac TUI → (4) Mac desktop GUI** (built FROM the menubar). Windows/Linux are **deferred 3–6 months**, revisited only on demand once all four Mac surfaces are engineered, working, AND selling. CI builds Mac-only (PR #64); off-strategy jobs (Windows installer, Android/iOS) are gated off, not deleted. See `docs/ADR-032-MAC-FIRST-PLATFORM-ROADMAP.md`.

**This repo is NOT SirsiNexusApp. This repo is NOT FinalWishes. This repo is NOT Assiduous.**
Rules, design tokens, and business logic from other repositories do NOT apply here unless explicitly inherited through Universal Rules (§1).

### Portfolio Position
| Repo | Type | Description |
| :--- | :--- | :--- |
| **SirsiNexusApp** | Platform Monorepo | Core infrastructure, shared services, UCS components |
| **FinalWishes** | Tenant Application | Estate planning platform (Royal Neo-Deco) |
| **Assiduous** | Tenant Application | Real estate platform (Assiduous Modern) |
| **sirsi-pantheon** (this repo) | **Infrastructure Tool** | Infrastructure hygiene CLI + fleet management |
| **sirsi-rook** (reserved) | **Database Tool** | Database & storage orchestration |
| **sirsi-rogue** (reserved) | **Security Tool** | Cybersecurity sweeper |

### Deity Hierarchy (canon — ADR-015, ADR-017, ADR-028)

```
Ra 𓇶 (Fleet Aggregator — Enterprise SKU)
 └── aggregates ConduitReports from all Horus instances (internal dev fabric = Ra #1)

Horus 𓂀 (ONE per node — shared node conduit)
 └── router items + observability = one unified flow → Ra
 └── per-node singleton: ~/.config/sirsi/horus/conduit.json

Anubis 𓃣 (single-node product = SNE + local Horus)
 └── SNE (Sirsi Node Engine, profile selected)
```

### Internal Modules
| Module | Codename | Archetype | Role |
| :--- | :--- | :--- | :--- |
| Local Scanner | **Jackal** 🐺 | The Hunter | Patrols and cleans individual machines |
| Ghost Hunter | **Ka** 𓂓 | The Spirit | Detects dead app remnants and residual hauntings |
| Fleet Sweep | **Scarab** 🪲 | The Transformer | Rolls across VLANs, subnets, domains |
| Policy Engine | **Scales** ⚖️ | The Judgment | Weighs findings against defined policies |
| Resource Optimizer | **Hapi** 🌊 | The Flow | Controls VRAM, GPU memory, and storage flow |
| Output Filter | **RTK** ⚡ | The Sieve | Strips noise from tool output before it hits AI context |
| Context Vault | **Vault** 🏛️ | The Keeper | Sandboxes large output in SQLite FTS5, indexes code for BM25 search |
| Node Conduit | **Horus** 𓂀 | The All-Seeing | Per-node singleton conduit; unified router + observability flow → Ra; code graph + ops dashboard capabilities (ADR-028) |

---

## 1. Universal Rules (Apply to ALL Sirsi Portfolio Repos)

> These rules are inherited from the Sirsi Portfolio Standard and are identical across every Sirsi repo.

0.  **Minimal Code** (Rule 0): Write the smallest amount of clean, correct code per page/file. If you're layering fixes on top of hacks, **DELETE AND REWRITE**. Band-aids are technical debt. Simplicity is non-negotiable.
1.  **Challenge, Don't Just Please**: If a user request is suboptimal, dangerous, or regressive, you MUST challenge it. Provide the "Better Way" before executing the "Requested Way".
2.  **Critical Analysis First**: Before writing a line of code, analyze the *Architecture*, *Security*, and *Business* impact.
3.  **Solve the "How"**: The user provides the "What". You own the "How". Do not ask for permission on trivial implementation details; use your expertise.
4.  **Agentic Ownership**: You are responsible for the entire lifecycle of a task: Plan -> Build -> Verify -> Document.
5.  **Sirsi First (Rule 1)**: Before building, check if it exists in the Sirsi ecosystem. We build assets, not disposable code.
6.  **Implement, Don't Instruct (Rule 2)**: Build working code end-to-end. No "here's how to set it up" responses.
7.  **Test in Terminal (Rule 3)**: Verify zero errors in build and test output. If you haven't verified it technically, it's not done.
8.  **Follow the Pipeline (Rule 4)**: Local -> GitHub -> Production. Never skip CI/CD.
9.  **Always Push & Verify (Rule 5)**: ALWAYS push changes to production via git. Verify the push status immediately.
10. **ADRs are Mandatory (Rule 8)**: Every significant decision requires an Architecture Decision Record.
11. **Do No Harm (Rule 14)**: You MUST NOT break any working process. A regression is worse than a missing feature.
12. **Additive-Only Changes (Rule 15)**: You may ADD or IMPROVE functionality, but MUST NOT recode any module in a way that disrupts the current working state.
13. **Mandatory Canon Review (Rule 16)**: Before writing code, re-read this file, relevant ADRs, and the files you intend to modify.
14. **Sprint Planning is Mandatory (Rule 17)**: Before ANY code change, present a detailed sprint plan. No code is written until the USER approves.
15. **Living Canon (Rule 18)**: These canonical documents are living documents. When new rules emerge, they MUST be codified immediately.
16. **Identity Integrity (Rule 19)**: All GitHub identities MUST use the `SirsiMaster` account exclusively.

---

## 2. Anubis-Specific Rules

### 2.1 Safety Protocol (PARAMOUNT)
> **These rules are PARAMOUNT. They override ALL other directives when in conflict.**

*   **Safety First (Rule A1)**: NEVER delete a file without dry-run verification available. Every destructive operation (`judge`, `guard --slay`, `hapi --kill-orphans`) MUST have a `--dry-run` flag. Protected system paths are hardcoded in `internal/cleaner/safety.go` and CANNOT be overridden by configuration, flags, or user input. A deletion that bypasses dry-run is a **critical security bug**.

*   **Scan Rule Isolation (Rule A2)**: Each scan rule is a self-contained Go file implementing the `ScanRule` interface. Rules MUST NOT have side effects during the `Scan()` phase — they may only read the filesystem and report findings. Side effects (deletion, modification) happen ONLY during the `Clean()` phase, which requires explicit user confirmation.

*   **Cross-Platform Safety (Rule A3)**: Agent binaries (`sirsi-agent`) must be statically compiled with `CGO_ENABLED=0` and zero external dependencies. They run on untrusted targets (customer VMs, containers, remote hosts). The agent MUST NOT execute arbitrary commands received from the controller — it implements a fixed, auditable command set.

*   **Network Safety (Rule A4)**: Fleet sweep operations (`anubis scarab`) require explicit opt-in via `--confirm-network` flag. Anubis MUST NEVER auto-discover and scan network targets without user initiation. Subnet scanning requires the user to explicitly provide the target range. No "scan everything" defaults.

*   **VRAM/GPU Safety (Rule A5)**: The Hapi module MUST NOT kill GPU processes that are actively training or inferencing. Before terminating any GPU process, check if it has had CPU activity in the last 60 seconds. Offer `--force` flag for override, but default is conservative.

### 2.2 Code Style
*   **Formatting**: `gofmt` is mandatory. No exceptions.
*   **Linting**: `golangci-lint` with the project's `.golangci.yml` config must pass.
*   **Testing**: Table-driven tests. Every scan rule must have at least one test.
*   **Error Handling**: Wrap errors with context using `fmt.Errorf("context: %w", err)`. Never swallow errors silently.
*   **Naming**: Use Go naming conventions. Exported types are PascalCase, unexported are camelCase. Package names are lowercase, single-word.

### 2.3 CI/CD QA Gate (Rule A6)
> **Every push and PR MUST pass the CI validation gate.**

*   **Workflow**: `.github/workflows/ci.yml`
*   **Pre-merge checks** (automated on every push/PR):
    1. **Lint** — `golangci-lint run ./...` must pass with zero errors.
    2. **Test** — `go test ./...` must pass with zero failures.
    3. **Build** — `go build ./cmd/sirsi/` and `go build ./cmd/sirsi-agent/` must succeed.
    4. **Binary Size Guard** — Warning if `sirsi` > 25MB or `sirsi-agent` > 12MB.
*   **Agent Responsibility**: After ANY `go get` that modifies `go.sum`, the agent MUST commit and push the updated sum file immediately.

### 2.4 Commit Traceability Protocol (Rule A7)
> Adapted from FinalWishes Rule 29. **No orphan commits.**

Every commit MUST be cross-referenced to the relevant:
1.  **Canon Document** — Which document(s) from §4 does this change relate to?
2.  **Version** — What version does this bump? (SemVer: patch/minor/major)
3.  **Changelog** — An entry MUST be added to `CHANGELOG.md` for every commit.
4.  **ADR** — Which Architecture Decision Record governs this change? If none exists, determine if one is needed.

Commit messages MUST include a `Refs:` footer linking to at least the canon doc and ADR.

```
type(module): description

[optional body]

Refs: [canon docs, ADRs]
Changelog: [version entry]
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `chore`
**Modules:** `jackal`, `scarab`, `scales`, `hapi`, `guard`, `sight`, `core`, `ci`, `docs`, `agent`

**Example:**
```
feat(jackal): add Parallels deep scan rule

Scans 12+ macOS subsystem directories for Parallels remnants:
Application Scripts, Group Containers, keychains, HTTPStorages,
package receipts, ghost apps in Launch Services.

Refs: PANTHEON_RULES.md, ARCHITECTURE_DESIGN.md, ADR-001
Changelog: v0.1.0 — Parallels scan rule
```

This ensures every line of code is traceable to a decision, documented for users, and visualized in the architecture. **No orphan commits.**

### 2.5 Feature Documentation Protocol (Rule A8)
> Adapted from FinalWishes Rule 30. **A feature without documentation is an incomplete feature.**

Every feature, scan rule, or module MUST have:
1.  **User-Facing Documentation** — Written in `docs/user-guides/` in plain language. Explains what the feature does in the CLI, what flags are available, and what to expect. Written for the sysadmin, developer, or DevOps engineer.
2.  **Developer-Facing README** — Written in the feature's directory (e.g., `internal/jackal/rules/README.md`). Explains the architecture, how to add new rules, the interface contract, dependencies, and known limitations.

Neither document is optional. The docs and README must be created **in the same commit** as the feature code.

### 2.6 Context Monitoring Protocol (Rule A9)
> Adapted from FinalWishes Rule 31. **The agent is responsible for ensuring the session never gets cut short.**

The agent MUST monitor context window and token usage throughout every session. After **every sprint or phase**, the agent MUST report:
1.  **Commits this session** — total count
2.  **Context health** — 🟢 Healthy / 🟡 Getting Deep / 🔴 Critical
3.  **Recommendation** — Continue / Wrap Soon / Wrap Now

When context health is 🟡 or 🔴, the agent MUST proactively:
- Commit all work
- Update `CHANGELOG.md`
- Generate a fresh `docs/CONTINUATION-PROMPT.md`
- Report final metrics

**The agent is responsible for ensuring the session never gets cut short by context exhaustion.** If the context is getting deep, say so. Don't wait to be asked.

### 2.7 Terminal UI Fidelity (Rule A10)
> Adapted from FinalWishes Rule 27 (design fidelity). Applied to terminal output.
> **v2 (2026-07-13, ADR-038):** the palette is **emerald + gold** and lives in ONE place — `internal/brand`. Green is Sirsi's; green + gold are Pantheon's. Every surface (CLI, TUI, dashboard, menubar, Swift app, Nexus) derives from `internal/brand` — Go surfaces import it, others via `sirsi brand tokens --format css|swift|json`. No surface hardcodes a hex. Emerald leads (identity / healthy / interactive); gold is the second accent (owner-action, the 𓂀 glyph). The gold-primary + lapis palette below is superseded.

All terminal output MUST use the Pantheon brand language (`internal/brand`, ADR-038):
*   **Colors**: **Emerald** (`#2bd29b`) for identity/healthy/interactive, **Gold** (`#cdad5a`) for owner-action highlights, White/Ink for body text, Red for errors, Amber for warnings. Resolve tokens from `internal/brand`, never a literal. No raw unstylized output in interactive mode.
*   **Rendering**: Uses `lipgloss` for styled output and `table` for tabular data.
*   **Headers**: 𓃣 glyph prefix for section headers.
*   **Progress**: Spinner or progress bar for operations > 2 seconds.
*   **JSON mode**: `--json` flag outputs raw JSON for piping/scripting. No styling in JSON mode.
*   **Quiet mode**: `--quiet` flag suppresses all output except errors and final summary.

### 2.8 Scan Data Privacy (Rule A11)
> Adapted from FinalWishes Rule 26 (PII siloing).

Anubis scans filesystems and processes. Scan results may contain sensitive information:
*   **File paths** in scan reports MUST NOT be transmitted to any external service.
*   **Process names and arguments** MUST be sanitized before any fleet reporting (strip environment variables, connection strings, tokens).
*   **Network scan results** (IPs, hostnames, open ports) are stored locally only — never transmitted unless the user explicitly exports them.
*   **Audit logs** (`~/.config/anubis/audit.log`) are local-only and NEVER uploaded.
*   Anubis has **zero telemetry, zero analytics, zero phone-home**. This is non-negotiable.

### 2.9 Code Freeze & Stability Protocol (Rule A12)
> Adapted from SirsiNexusApp §2.2. **PARAMOUNT.**

*   **Do No Harm**: You **MUST NOT** break any working scan rule, CLI command, or module. Before touching any file, verify what currently works and ensure it still works after.
*   **Additive-Only Changes**: Do not refactor working scan rules, restructure working module interfaces, or rewrite working logic unless explicitly directed.
*   **Mandatory Canon Review**: Before writing code, re-read `PANTHEON_RULES.md`, relevant ADRs, `SAFETY_DESIGN.md`, and the files you intend to modify.
*   **Sprint Planning is Mandatory**: Present a detailed sprint plan before ANY code change. No code without USER approval.
*   **Living Canon**: Codify new rules immediately — never defer.

### 2.10 Release Versioning Protocol (Rule A13)
> Adapted from SirsiNexusApp §5.1.

*   **Semver**: `MAJOR.MINOR.PATCH-channel` (channels: `alpha` → `beta` → `rc` → `stable`)
*   **Source of Truth**: `VERSION` file at project root
*   **On Every Release**, update ALL of: `VERSION`, `CHANGELOG.md`, git tag
*   **goreleaser** handles binary distribution with version injection via `-ldflags`
*   **Tag format**: `v0.1.0-alpha`, `v1.0.0`, etc.

### 2.11 Statistics Integrity Protocol (Rule A14)
> Established March 22, 2026, after audit found 5 inflated claims in public-facing materials.

*   **Every public-facing number MUST be independently verifiable.** Include the command to reproduce it.
*   **No projections presented as measurements.** If a number is projected, it MUST be labeled as a projection.
*   **No cross-repo claims without cross-repo evidence.** Benchmarks measured on Anubis do not apply to other repos unless measured there.
*   **Cumulative claims require cumulative evidence.** "X tokens saved across N sessions" requires N to be counted, not estimated.
*   **When in doubt, report per-session numbers only.** Per-session savings are verifiable; cumulative extrapolations are speculation.

### 2.12 Session Definition (Rule A15)
> Established March 22, 2026. Canonical definition for all Thoth metrics and context monitoring.

*   A **session** is defined as one AI conversation — the work between two continuation prompt runs.
*   Sessions are NOT defined by time gaps, commit clusters, or calendar days.
*   `docs/CONTINUATION-PROMPT.md` is the session boundary marker.
*   Session counts in ROI calculations, case studies, and build logs MUST use this definition.

### 2.13 Side Effect Injection (Rule A16)
> Established March 24, 2026. Required for 99% test coverage and deterministic safety audits. (ADR-009)

*   **Rule**: ALL logic performing system-level side effects (`exec.Command`, `signals`, `os.RemoveAll`, `os.UserHomeDir`) MUST be abstracted through an interface or function type (Injection Pattern).
*   **Standard**: Every module MUST export a default simple function (e.g., `Slay()`) that delegates to an internal "With" variant (e.g., `SlayWith(killer)`).
*   **Safety**: Testing MUST exercise at least one failure path for every system side effect (e.g., "kill root process fails") without mutating the host.
*   **Verification**: A module with system side effects and zero mock-based coverage is a **governance failure**.

### 2.14 The QA Sovereign (Rule A17)
> Established March 24, 2026. Required for across-the-board quality in the Pantheon. (docs/QA_PLAN.md)

*   **Rule**: 𓆄 **Ma'at** is the sole deity of quality, truth, and order. She maintains the **Quality Charter** (`docs/QA_PLAN.md`).
*   **Feather Weight**: All Sirsi ecosystem code (Sirsi, Assiduous, FinalWishes) is judged by Ma'at's **Feather Weight (0-100)** score.
*   **Sovereignty**: Ma'at administers the tests, validates the scores, and provides the "Quality Insight" for all other deities.
*   **Canon Gate**: A module failing a Ma'at assessment (score < 85) is considered "not yet canon" and cannot be included in a stable release.

### 2.15 Incremental Commits (Rule A18)
> Established March 25, 2026. Prevents session loss from IDE crashes or context exhaustion.

*   **Rule**: After every **5 file changes**, the agent MUST perform a checkpoint commit and push.
*   **Rationale**: A single IDE crash can erase an entire session's unsaved work. Incremental commits ensure that progress is preserved regardless of external failures.
*   **Format**: `chore: checkpoint — [brief description of changes]`

### 2.16 No Application Bundle Mutations (Rule A19) — ABSOLUTE PROHIBITION
> Established March 25, 2026. Hardened March 26, 2026 after forensic proof that manifest-only patches caused a V8 OOM crash cascade requiring full IDE reinstall.

*   **Rule**: The agent MUST NEVER write to, modify, delete, or replace **ANY** file inside `/Applications/*.app/` bundles. **No exceptions.** This includes:
    *   Language server binaries (`language_server_macos_arm`, etc.)
    *   Extension `package.json` manifests (even "JSON-only" changes)
    *   Extension source files, frameworks, or helper binaries
    *   Any file inside `Contents/Resources/`, `Contents/Frameworks/`, or `Contents/MacOS/`
*   **Rationale**: Application bundles carry two layers of integrity:
    1. **Code signing** — Modifications invalidate the macOS signature, triggering Gatekeeper blocks.
    2. **Semantic integrity** — Extension manifests declare commands, menus, and activation events. Adding declarations without corresponding handlers creates an un-realizable state that causes the Extension Host to leak memory through repeated validation failures, leading to **V8 heap OOM** (`electron.v8-oom.is_heap_oom`) and **macOS Jetsam termination** (`libMemoryResourceException.dylib`). This crash chain is invisible to the user and requires forensic analysis of Crashpad dumps to diagnose.
*   **Enforcement**: Any `cp`, `mv`, `rm`, or `write` operation targeting a path matching `/Applications/*.app/**` is a **CRITICAL SAFETY VIOLATION** equivalent to Rule A1 (Safety First).
*   **Evidence**: Session 23 crash forensics — 3 crash dumps in 59 minutes, 34 total pending dumps, full IDE reinstall required. See `docs/case-studies/session-23-extension-host-crash-forensics.md`.
*   **If the IDE has bugs in bundled extensions**: Report upstream. Do NOT patch locally.

### 2.17 SirsiMaster Browser Profile (Rule A20)
> Established March 26, 2026. All browser-based agent activities must use the SirsiMaster identity.

*   **Rule**: ALL browser subagent activities MUST use the **SirsiMaster Chrome profile**. This includes but is not limited to:
    *   OpenVSX publishing (Eclipse Foundation login)
    *   GitHub OAuth flows
    *   Firebase Console operations
    *   Any marketplace, registry, or service authentication
*   **Rationale**: The SirsiMaster profile contains all stored credentials (Eclipse/OpenVSX, GitHub, GoDaddy, Firebase) for Sirsi ecosystem services. Using the wrong profile leads to authentication failures and identity mismatches.
*   **Enforcement**: Browser subagents MUST be instructed to use the SirsiMaster Chrome profile in their task description. Thoth MUST propagate this requirement to all session continuations.

### 2.18 Concurrency-Safe Injectable Mocks (Rule A21)
> Established March 27, 2026, after 4 consecutive CI failures caused by data races on `sampleTopCPUFn`. **𓆄 Ma'at governs this rule as QA Sovereign.**

*   **Rule**: Package-level function pointers used for test injection (the "Injectable Provider" pattern from Rule A16) MUST be protected by a `sync.RWMutex`. Direct assignment (`pkgFn = mockFn`) is a **race condition** when goroutines spawned by previous tests may still be reading the variable.
*   **Pattern**: Every injectable function pointer MUST have a paired accessor:
    ```go
    var (
        sampleMu    sync.RWMutex
        sampleFn    = defaultImpl
    )
    func getSampleFn() func(...) { sampleMu.RLock(); defer sampleMu.RUnlock(); return sampleFn }
    func setSampleFn(fn func(...)) { sampleMu.Lock(); defer sampleMu.Unlock(); sampleFn = fn }
    ```
*   **Test Pattern**: Tests MUST use `setSampleFn()` to install mocks and `getSampleFn()` to save/restore:
    ```go
    old := getSampleFn()
    setSampleFn(mockFn)
    // ... test logic ...
    cancel()                         // stop goroutines first
    time.Sleep(100 * time.Millisecond) // drain
    setSampleFn(old)                 // restore under lock
    ```
*   **Why `defer` is dangerous**: `defer func() { sampleFn = old }()` runs AFTER the test function returns, but goroutines from `StartBridge`/`StartWatch` may still be reading `sampleFn` on a locked OS thread. The race detector sees the write (restore) and read (goroutine) on the same address without synchronization.
*   **Enforcement**: Any module using Rule A16 (Injectable Providers) with goroutine-based consumers MUST comply with this rule. A package-level `var fn = defaultFn` without a mutex is a governance failure under Ma'at.
*   **Evidence**: Sessions 29-30 — 4 consecutive CI failures, all `WARNING: DATA RACE` on `sampleTopCPUFn` at `watchdog.go:160`. Fixed by `getSampleFn()`/`setSampleFn()` accessor pattern.

### 2.19 Neith's Architecture Triad (Rule A22)
> Established March 28, 2026. Every architecture document must contain the three mandatory sections decreed by 𓁯 Net (The Weaver).

*   **Rule**: Every `ARCHITECTURE_DESIGN.md` (or equivalent primary architecture document) in every Sirsi portfolio repository MUST contain the following three sections, known as **Neith's Triad**:
    1. **Data Flow Architecture** — A Mermaid diagram showing all data flows, transformations, and system boundaries. Must label every edge with the data transformation. Must show error/fallback paths where applicable.
    2. **Recommended Implementation Order** — A Mermaid Gantt chart or numbered phase list showing build sequence, dependencies, and estimated effort. Must identify the minimum viable pipeline and distinguish required vs. optional phases.
    3. **Key Decision Points** — A Markdown table matrix of architectural decisions with columns: Question | Options | Recommendation. Must capture at least 3 decision points, include rationale, and record rejected alternatives.
*   **Retroactive**: Existing repos (`sirsi-pantheon`, `SirsiNexusApp`, `FinalWishes`, `Assiduous`) MUST be audited and updated to include these sections in their next architecture session.
*   **Enforcement**: A new architecture document missing any of the three sections is considered **incomplete** under Ma'at's governance (Rule A17). It cannot be merged until all three are present.
*   **Custodian**: 𓁯 Net (Neith) owns this standard and the templates. The Triad is maintained in `docs/NEITH_ARCHITECTURE_TEMPLATE.md`.
*   **Evidence**: Established from the Gemini Bridge architecture document, which demonstrated that these three sections provide complete decision traceability, implementation clarity, and project alignment.

### 2.20 Truth Vector (Rule A23)
> Established March 28, 2026. The foundational honesty protocol governing all AI-assisted development across the Sirsi ecosystem.

*   **Rule**: Every AI agent operating within a Sirsi repository MUST adhere to the following six axioms. Violation of any axiom is considered a governance failure under Ma'at.
    1. **Always tell the truth.** If you do not know how to do something — whether it is coding, planning, integration, or any other task — you must say so. Pretending competence is worse than admitting uncertainty.
    2. **Declare confidence.** Before writing code, provide your confidence level in both the plan and your capability to implement it. This is a hard requirement, not optional transparency.
    3. **Ask, never guess.** When you don't know or don't understand, it is always better to ask rather than infer or guess. Guessing wastes sessions. A question costs nothing; a wrong assumption costs a refactor.
    4. **Measure thrice, cut once.** Do not write code until you understand the task and are confident you can achieve the requested goal. Premature implementation creates technical debt.
    5. **Advanced simplicity.** Always seek the most advanced solution that does NOT increase or create complexity. Simple, direct solutions that never require refactoring are the standard. Clever code that needs explaining is a failure.
    6. **Use existing tools.** Use the tools at hand — skills, extensions, Pantheon deities, external APIs. If it is easier and cheaper to use an external tool rather than building yourself, suggest that approach and explain the integration cost.
*   **Scope**: This rule applies to ALL Sirsi repositories and ALL AI agents (Antigravity, Claude, Gemini, Cursor, Windsurf). It is not project-specific.
*   **Enforcement**: An AI agent that guesses instead of asking (Axiom 3), or codes before understanding (Axiom 4), has violated Truth Vector. The resulting work must be reviewed before canonization.
*   **Custodian**: The user is the sole arbiter of Truth Vector compliance.

### 2.21 Ra Scope Autonomy (Rule A24)
> Established April 3, 2026, after 4 Ra-deployed agents blocked indefinitely waiting for sprint plan approval that could never arrive in non-interactive mode.

*   **Rule**: Ra scope configs (`configs/scopes/*.yaml`) define **pre-approved sprint plans**. Agents spawned by `sirsi ra deploy` MUST execute scopes without asking for human approval. The Neith loom (`internal/neith/loom.go`) injects a **Ra Autonomy Directive** at the top of every woven prompt that overrides Rule 17 (Sprint Planning is Mandatory).
*   **Scope Authoring**: Scopes MUST be written as directive, numbered task lists — not vague descriptions. Each task must name specific files, paths, or concrete actions. Vague scopes cause agents to ask clarifying questions, which hang forever in `--print` mode. See `configs/scopes/README.md` for the full authoring guide.
*   **Prompt Structure**: The autonomy directive and scope of work are placed at the **top** of the woven prompt and are **never truncated**. Canon context (CLAUDE.md, Thoth memory, ADRs) fills the remaining token budget and may be truncated.
*   **Permission Model**: Ra agents run with `--dangerously-skip-permissions` because the scope is pre-approved. This flag MUST NOT be used outside of Ra-deployed agents.
*   **Streaming Output**: Ra agents MUST use `--output-format stream-json --verbose` with `--print`. Default `--print` mode buffers ALL output until the session completes, making agents appear lifeless for 10+ minutes. The stream-json output is piped through a python filter (`terminal.go`) that extracts human-readable text and tool-use summaries, writing to both the terminal (live progress) and the log file (Ra monitoring).
*   **Evidence**: Session where `sirsi ra deploy` spawned 4 windows; all 4 agents asked for approval and blocked. Root causes: (1) CLAUDE.md Rule 14 conflict, (2) vague scope descriptions, (3) directive placed after canon context and truncated, (4) `--print` default text mode buffered all output making agents appear dead.

### 2.22 Deity Registry & Attribution (Rule A25)
> Established April 4, 2026, after pre-push hooks in FinalWishes and Assiduous misattributed deity glyphs and functions.

*   **Rule**: Every deity has one glyph, one domain, and one functional responsibility. These are defined in `docs/DEITY_REGISTRY.md` and are invariant across all Sirsi repos. No repo may reassign a deity's function or glyph.
*   **Ma'at Owns All Quality Gates**: Every pre-push hook, CI gate, and quality assessment is `𓆄 Ma'at`. Output format: `𓆄 Ma'at pre-push gate... [RepoName]`. No other deity may be attributed for quality gate functions.
*   **ProtectGlyph Is Ra-Exclusive**: `𓂀` in a Terminal.app window title is Ra's authority to mark windows as immune to `KillAll`. It is not a general-purpose glyph and must not be used as another deity's symbol in functional contexts.
*   **No Repo-Specific Aliases**: A deity is never renamed for a repo. Correct: `𓆄 Ma'at pre-push gate... [FinalWishes]`. Wrong: `𓁹 Osiris (FinalWishes) pre-push gate...`.
*   **Evidence**: FinalWishes used `𓂀 Osiris` for its pre-push gate (wrong deity, wrong glyph, wrong function). Assiduous used `𓇼 Seba` (wrong deity for quality gates). Both corrected to `𓆄 Ma'at`.

### 2.23 Idea Router Workstream Protocol (Rule A26)
> Established May 15, 2026. Codex and Claude must collaborate through the Idea Router for multi-agent and cross-agent workstreams.

*   **Rule**: All non-trivial Sirsi workstreams MUST begin with `/plan`. Codex and Claude MUST collaborate through `.agents/idea-router/` to create or review the plan before implementation when both agents are involved.
*   **Goal Flag**: Every workstream MUST define a `/goal` flag in the plan. The `/goal` is the explicit completion condition, including required verification, tests, review, and handoff artifacts. Agents continue working until the `/goal` is met, blocked by safety/user approval, or impossible with a stated reason.
*   **Repo Segmentation**: Work on repositories MUST be segmented. Each repository requires its own agent/workstream. A single agent MUST NOT modify multiple repositories unless it is explicitly designated as a **super agent** with a written cross-repo mandate in the `/plan`.
*   **Super Agent Mandate**: A super agent may coordinate multiple repo agents, compare evidence across repos, and write cross-repo decisions, but MUST avoid direct code edits across repos unless the mandate explicitly permits those paths.
*   **Parallel Agents**: When enough context and token budget exist, spawn multiple repo-scoped agents rather than serializing unrelated repo work. Each agent owns one repo and one bounded task set.
*   **Idea Router Handoff**: Proposals, reviews, decisions, and completion notes MUST be written to `.agents/idea-router/`. A submission by Codex should create a pending item for Claude; a submission by Claude should create a pending item for Codex.
*   **Completion Relay**: Agents MUST continue the relay until the `/goal` is met. If the current environment cannot automatically wake the other agent, the submitting agent MUST leave an explicit pending router item and a concise next-action instruction.
*   **No Silent Cross-Repo Drift**: Any claim about repo state, completion, test status, or deployment status must name the repo and cite evidence gathered in that repo.
*   **Enforcement**: Ma'at treats unmandated cross-repo edits, missing `/plan`, missing `/goal`, or unclosed router handoffs as governance failures.
*   **Automation Boundary**: Full automatic triggering between Codex and Claude is provided by the autorouter daemon: `sirsi router daemon` for foreground operation and `sirsi router install-agent --load` for the resident macOS launch agent. The filesystem router remains the source of truth; the daemon dispatches pending inbox items but never acknowledges them for an agent.

### 2.24 Heartbeat Loop Mandate (Rule A27)
> Established June 1, 2026. A registered router thread that is not looping is invisible to its own inbox. Extends A26 (Completion Relay): registration means "alive and watching," not merely "known."

*   **Rule**: Every agent thread that registers with the router (`sirsi thread register`) MUST run a persistent heartbeat loop — a wake-loop that watches its inbox — from registration until it de-registers (`sirsi thread close`). Registered-but-not-looping is a node-health failure under Horus `router node-status` and a governance failure under Ma'at.
*   **The loop IS the heartbeat**: This is one primitive across all surfaces; only the mechanism differs. **Claude threads MUST implement the loop via `/loop`** (self-paced, calling `sirsi router wait <agent>`, which BLOCKS on the durable store until work lands). **Codex** uses its app heartbeat automation (`ctr-thread-wake` polling the inbox; native thread heartbeat where available). **Gemini/Gemma/Qwen** use a surface-native loop or fall back to `sirsi router daemon`. **mcp/api/webhook/worker** use `sirsi router daemon` or the resident launch agent.
*   **Loop scope**: The heartbeat loop is a *watcher*, not a work driver. Its job is minimal and bounded: pull the inbox, act on or queue new items, emit `sirsi thread heartbeat`, sleep. Prefer event-driven waking (`sirsi router wait`, which blocks on the store) over fixed polling, with a long fallback tick so a missed event never strands the thread.
*   **Lifecycle binding**: One loop per thread. Start it at register, stop it ONLY at `thread close`. De-registration is the single clean way to end the loop — never abandon a registered thread with no loop.
*   **Why**: Without a live loop, items addressed to a registered thread sit unread until a human types `ctr`. Codex already approximates this via its heartbeat automation (`ctr-thread-wake`); this rule gives every Claude thread the same parity so the multi-agent relay (A26) actually completes without manual nudging.
*   **Resident UI surfaces are nodes too** (added 2026-06-01): An interactive surface that can initiate work or take operator interaction — **menubar, TUI, IDE plugin, SwiftUI/macapp** — is a router-registered thread, not merely a renderer. It registers bound to its **own process PID** and heartbeats from its **native runloop on a bounded interval (≥60s)** — never on a frequent render/stats tick, which floods the registry and feeds Spotlight `mds_stores` (the 2026-06-01 lockup). The heartbeat proves liveness to Horus/Ra; a surface that does not act on inbox items need not run the full watcher loop. Close on graceful shutdown (SIGTERM/quit); hard kill falls back to OS-truth reaping (ADR-022). Registration MUST be idempotent on `(agent_id, pid)` so surface restarts never accumulate duplicate active records. Surface ids: `menubar`, `tui`, `vscode`/`jetbrains`/`cursor`, `macapp`.
*   **Reference**: `.agents/idea-router/README.md` § "Heartbeat Loop (mandatory from register → close)".

### 2.25 Ma'at Gate & CI Protection Mandate (Rule A28)
> Established June 5, 2026. Ratified by claude-pantheon + codex-pantheon (router `20260605-231444`). Operationalizes A6 (CI QA Gate) + A17/A25 (Ma'at is the sole QA Sovereign): the gate must EXIST **and be ARMED** everywhere — shipping a disarmed gate is not compliance.

*   **Rule**: Every Sirsi repo MUST (1) **auto-arm its local 𓆄 Ma'at pre-push gate during setup/install** — `gofmt`/fmt + `vet` + `golangci-lint` (matching CI) + diff tests, attributed to Ma'at per A25 — and (2) **protect `main` with branch protection** requiring all CI status checks to pass, strict up-to-date branches, blocked force-pushes, and blocked deletions.
*   **Armed, not just shipped**: a gate at `.githooks/pre-push` is inert until `git config core.hooksPath .githooks`; a fresh clone defaults to `.git/hooks` (empty) and ships **DISARMED**. The repo's installer/setup MUST arm it so contributors are never silently ungated. Evidence: a `govet` shadow slipped Pantheon's CI because the shipped gate was never armed (2026-06-05).
*   **Auto-merge is OPTIONAL** (codex-pantheon guardrail): repos MAY enable GitHub auto-merge where maturity and owner policy allow, but it is NOT mandatory canon — some repos require manual release gates, regulated review, or staged deploy timing.
*   **Admin override**: branch protection runs with `enforce_admins=true` — admins (including the founder) go through the same CI + bind gates (verified live 2026-07-16; canon previously claimed `false`). The founder override (A23 — sole arbiter) is the documented, deliberate toggle (`gh api -X DELETE .../branches/main/protection/enforce_admins`), exercised per-decision and re-armed after — never a standing bypass. Because all agents authenticate as the founder's account, a standing `enforce_admins=false` would let `gh pr merge --admin` skip **all** required checks — how #213–#216 self-merged.
*   **Bind is identity-enforced on authority-model paths (ADR-041, owner decision 2026-07-15)**: a PR touching `.github/`, `scripts/bind/`, `cmd/sirsi/`, `internal/router/`, `PANTHEON_RULES.md`, or `docs/ADR-*` MUST carry an **APPROVED review from a login other than its author, on the current head SHA**, before merge. Every agent is one account (`SirsiMaster`), so any *label* an agent can apply the author can apply — a marker is not a gate. GitHub's ban on self-approval is the only unforgeable primitive available, so bind is pinned to it, recorded by the second identity `sirsi-bind` (`scripts/bind/sirsi-bind.sh`; key local-only, never in Secrets). **Ordinary product PRs are unaffected and stay autonomous** — the scope is deliberate. Honest limit: this proves an independent *identity* approved, not that a *human* reviewed.
*   **Reference**: sirsi-pantheon `internal/setup.ArmMaatGate()` + `.githooks/pre-push` + `gh api -X PUT repos/SirsiMaster/<repo>/branches/main/protection`. Custodian: 𓆄 Ma'at (A17). Portfolio-standard candidate — mirror into the Universal Rules (§1) once each repo confirms adoption.

### 2.26 Orchestration Brain: Tiered & Pluggable (Rule A29)
> Established July 2, 2026. Co-authored claude-home ↔ claude-pantheon (router bind `20260630-190342`, all 6 amendments + the 7th Registry/Wake invariant). Custodian: 𓁟 the Brain (control plane) + 𓁢 the Router (Tier-0 substrate). Full design: `docs/prd/ORCHESTRATION_BRAIN.md`; decision: ADR-034.

*   **Rule**: Pantheon's orchestration intelligence is a **tiered, pluggable, user-navigable brain**, not a single always-on model. It has three tiers — **Tier-0 Dispatch** (watch/route/heartbeat/ack), **Tier-1 Triage** (classify ambiguous items), **Tier-2 Execution** (agentic build/review/bind) — over an LLM spectrum **Level 0–3** (0 Deterministic → 1 +local triage → 2 +agentic execution → 3 +hosted). The Level is **derived** from per-role provider config, never separately stored.
*   **The deterministic floor (mandatory)**: **Tier-0 dispatch MUST run with zero LLM** and ships **ON at Level 0** on a fresh public install — dispatch/route/heartbeat/ack all work with no AI, no keys, no cost. The model is *invoked by* the loop, never *is* the loop. The config layer and `sirsi brain doctor` both **reject a model plugged into dispatch**.
*   **Per-role pluggable**: each role independently selects a provider — `none` (floor) · `local:<model-id>` (zero-token) · `hosted:<provider-id>` (opt-in, the only per-token path). Config lives in **`~/.sirsi/brain.yaml`** (structured YAML via the repo's yaml.v3 standard — not a bespoke `.conf`, not a new viper dependency; Rule 0). Swaps take effect on **next read — no restart**.
*   **Tier-0 Registry/Wake invariant (7th amendment — ENFORCE, don't rebuild)**: "the router can always see and wake every registered thread." Registration binds a **persistent wake-channel**; a registered thread with no live channel is a broken contract (the zombie state). This invariant is **already implemented** in `internal/router` (`WakePass`, `ProbeWakeReadiness`, `InstallWakeLaunchAgent`, `RunWakeLoop`, `wakemechanism.go`; `sirsi router doctor --fix` runs the wake pass). A29 **codifies that existing system** — it MUST NOT be reimplemented (Rule 0). The brain's control plane **surfaces + enforces** it: `sirsi brain status`/`doctor` read the existing wake API to flag every registered-but-unwakeable agent and every stranded inbox, and point the fix at the **existing** verbs (`sirsi router wake-install`, `sirsi router doctor --fix`). Waking + repair stay the router's job; the brain observes. Honest boundary preserved: a fully-closed interactive Claude process cannot be resurrected locally → **"needs-owner"**, stated not faked. This makes A27 (Heartbeat Loop Mandate) *enforced*, not advisory.
*   **Resource-broker consumer (RAM gate)**: before loading a local model the brain consults `guard.NodeCapacity.Fits()`; `doctor` reports "**won't fit — N GB short**" instead of letting it OOM (defense-in-depth with ADR-031-A/B).
*   **Hosted-key handling (A11 + safety)**: Level-3 keys live in the OS keychain or `~/.sirsi/` 0600 — **never in brain.yaml, never logged**, transmitted only to the chosen provider. `doctor` reports "auth present" without printing the key.
*   **Visible + modifiable + troubleshootable**: the active tier + per-role model MUST be visible and swappable in the **CLI** (`sirsi brain {status,use,levels,doctor,test}`) and the **menubar**, and visible in **SirsiNexus** (`--json` read-model). "No black box" — brain decisions append to the Activity/stele provenance ledger.
*   **Decoupled from Router v2**: the brain is built against a `Dispatcher` interface over the **current** router; Router v2 swaps in underneath the same interface later (Amendment 1) — the brain never blocks on that rewrite.
*   **Reference**: `internal/brain/{config.go,controlplane.go}` + `cmd/sirsi/braincmd.go` (P1b control plane, shipped); ADR-034; PRD `docs/prd/ORCHESTRATION_BRAIN.md`. Custodian: 𓁟 the Brain.

### 2.27 Model Tiering Doctrine — Compute Economy Law (Rule A30)
> Established July 13, 2026 by owner directive. **Permanent, universal across every repo, thread, agent, and model family — present and future.** Canonical source text: `~/Development/AGENTS.md` § "Model Tiering Doctrine (Compute Economy Law)". This rule codifies that law into Pantheon canon (Living Canon, Rule 18) and is the routing **policy** that A29's Orchestration Brain **enforces**. Ma'at treats violations as governance failures.

*   **The law in one line**: **generation is cheap to get wrong; judgment is expensive to get wrong — push generation down-tier, keep judgment up-tier.** Independence in review comes from a fresh context with no stake in the code, **not** from a different brand of model. What is tiered is *cognitive difficulty*, not vendor.
*   **The three tiers** (map onto A29's Tier-0/1/2 and the Level 0–3 spectrum):
    *   **Tier 0 — local, on-device model (zero API tokens)**: high-volume, low-stakes screening + drafting — queue triage, first-draft code for well-specified decomposed tasks, summarization, log-reading, boilerplate, NL queries over local state. **A Tier-0 output is a SCREEN or a DRAFT, never a verdict.**
    *   **Tier 1 — cloud model, standard effort**: routine agentic work — routing, nudging, ACK-closes, board publishing, grinding decomposed task lists (dep bumps, doc updates, test fixes). Most scheduled/loop runs are Tier 1; an empty run needs almost nothing.
    *   **Tier 2 — frontier model, high effort**: reserved for exceptional thinking — **binding verdicts (source-deep review before merge — ALWAYS Tier 2)**, architecture decisions, security review, ESCALATE-classed ambiguity, debugging that resisted a first pass.
*   **Operating rules**: (1) **builders decompose; the cheapest competent tier types** — a thread's job on well-specified work is spec → hand to Tier 0 → review, not typing code at frontier prices; (2) **the bind is always frontier** — a slightly-off draft is caught at bind, a slightly-off bind ships a bug to main, so spend where failure is irreversible; (3) **screens never become verdicts** — no Tier-0 classification stands as a binding security/review/architecture decision; (4) **read only what escalates** — cloud models don't read whole queues/logs/repos when a Tier-0 screen can classify first (the 2M-token incident is the cautionary tale); (5) **route by difficulty, not habit** — if a tier is unclear, start one lower and escalate on failure (escalation is cheap, standing overspend is not); (6) **enforced by the system, not discipline** — the Orchestration Brain (A29; `docs/prd/ORCHESTRATION_BRAIN.md`) is the reference implementation, and every repo's automation should route through it or mirror its policy.
*   **Brand invariant** (with A25/Brand-Over-Model-Name): user-facing surfaces never expose model identity ("Ask Sirsi", never a vendor/model name); the on-device privacy promise stands independent of which local model serves Tier 0.
*   **Reference**: canonical law `~/Development/AGENTS.md`; enforced by A29 (Orchestration Brain) + `docs/prd/ORCHESTRATION_BRAIN.md` default routing table. Custodian: 𓁟 the Brain (routing) + 𓆄 Ma'at (governance).

### 2.28 CTR — Check The Router, the universal wake primitive (Rule A31)
> Established July 13, 2026 by owner directive, after background wake (monitors, arming, watchers, launchd daemons) repeatedly failed to keep threads consuming their inboxes — a level-triggered daemon that dies silently strands every item addressed to it. Custodian: 𓁢 the Router.

*   **Rule**: `ctr` ("Check The Router") is the canonical **on-demand** wake primitive. It is ONE synchronous router pass — surface every open inbox item, then wake the agents that have work waiting but no live watcher — with **no daemon to keep alive**. The trigger moves to events that already happen (a human typing, a hook, a git commit, a cron, another agent), which is why it works where resident processes have not.
*   **One primitive, three call sites** (all thin adapters over the same Go verb, so they can never diverge): **`ctr`** (a PATH shim, any shell / IDE terminal, mac/linux/windows, headless or resident) · **`/ctr`** (a Claude Code skill, known to every session in every repo) · **`sirsi ctr`** (the cross-platform source of truth). Both a human and a process may call it; `sirsi ctr --json` is the machine contract.
*   **Wrap, don't rebuild (Rule 0)**: `sirsi ctr` orchestrates the EXISTING substrate — `router.CollectNodeStatus` (pending + stranded truth) and `router.WakePass` (wake-or-declare-unavailable). It MUST NOT reimplement wake logic. Honest boundary (A29): a fully-closed interactive session cannot be resurrected locally → reported **"needs-owner"**, stated not faked; heartbeat-fresh ≠ consuming, and CTR says so.
*   **Local model first (A30 Tier-0)**: `ctr --reconcile` hands the open items to the on-device model to RECEIVE and RECONCILE (triage into TIER0/1/2) BEFORE anything escalates to a cloud thread — threads work through the local model first. The reconciliation is a Tier-0 **screen, never a binding verdict**; it runs warm-broker-only (never cold-loads a multi-GB model) and is time-bounded so a plain `ctr` stays a fast wake primitive. User-facing name is **"Ask Sirsi"**, never the model id (A25 brand-over-model-name).
*   **Ubiquity is mandatory**: `sirsi setup` wires the shim + skill on every machine (present and future). `sirsi ctr --install` re-wires idempotently. The user-global skill (`~/.claude/skills/ctr`) makes `/ctr` known to every agent/thread; canon here + `~/Development/AGENTS.md` make it known to every repo. Like `/thoth`, it is a first-class shippable Pantheon command.
*   **Reference**: `cmd/sirsi/ctr.go` + `cmd/sirsi/ctrinstall.go`; skill `.claude/skills/ctr/SKILL.md`; substrate `internal/router/{wake.go,strand.go,nodestatus.go}`. Custodian: 𓁢 the Router.

### 2.29 Do No Harm To The Running Host — Load-Bearing Recognition (Rule A32)
> Established July 14, 2026 by owner directive, after an agent nearly killed the process holding 25.8 GB to "reclaim RAM" — which was the local-model **broker itself** (`sirsi gemma serve`, running as `Python`). Resized instead of killed, but it exposed that Pantheon's own governor treated the Tier-0 substrate as expendable. Custodian: 𓁢 the Broker (Hapi 🌊). Decision: ADR-040. Extends A1 (Safety First) + A5 (VRAM/GPU Safety) to the host Pantheon runs ON.

*   **Rule**: While the system is working, an agent — or the continuous loop, or any Pantheon governor — MUST NOT kill or starve **load-bearing Pantheon infrastructure**. The canonical load-bearing service is the local-model broker (the Tier-0 substrate the router, the reconcile, and gemma-the-builder all depend on); more may be added. Breaking the host to do the work is a governance failure.
*   **Recognition by pidfile, not name**: `internal/guard.LoadBearingPIDs()` / `IsLoadBearing(pid)` reads the infra pidfiles (`~/.sirsi/gemma-server.pid`, `gemma-worker.pid`), excludes dead PIDs (a stale pidfile never protects a reused PID — the PID-alive lesson), and is the single authority every kill/suspend path consults. The broker runs as `Python`, so name-based protection (A24 ProtectGlyph, `isProtectedReniceTarget`) misses it — recognition MUST be by PID. `FindRunaway` never selects a load-bearing PID even as top RSS.
*   **Right-size over kill**: the correct response to an oversized Tier-0 model (a 25 GB 12B where a 2 GB 3B belongs) is to **right-size** it (`~/.sirsi/gemma-model.conf` → a smaller model; `sirsi gemma serve --stop && sirsi gemma serve`), reclaiming the RAM while keeping the builder. Killing the broker is an absolute last resort at true emergency (imminent Jetsam) — never a routine reclaim, and **never something an agent does mid-work**. Verify the full argv before signalling any hog (`ps -p <pid>`); "biggest RSS" is not "kill me".
*   **Gemma-the-builder is bound the same**: when the local model does build/triage work, its instructions carry this constraint — do not kill or starve Pantheon infrastructure; resize/reconfigure, never SIGKILL a serving process. Gemma must not break Pantheon while working.
*   **Fix-don't-narrate corollary** (from the same incident): a blocker an agent has DIAGNOSED and CAN fix within remit gets FIXED on sight, never narrated back to the owner as a standing constraint (a RAM starvation reported three times but never fixed is the anti-pattern).
*   **Reference**: `internal/guard/loadbearing.go` (+ `hapi.go` FindRunaway); ADR-040. Custodian: 𓁢 Hapi.


### 2.30 Universal Thread Census & Work Board Overseer (Rule A33)
> Established July 17, 2026 by owner directive, after the on-device model broker — a GPU process holding ~30 GB of wired Metal memory — drove two system-wide jetsam incidents while registered in NO registry: no board, reaper, or overseer could see, govern, or resize it. Co-implemented claude-pantheon (census primitive) ↔ claude-home (overseer duties). Custodian: 𓂀 Horus (visibility) + 𓁢 the Router (registry).

*   **The invariant**: **every non-system agent-class process on every surface — CPU and GPU — exists as a registered thread, with no misses, current and future.** A process the registry cannot see is a process nothing can govern. This completes A27 (registration means alive-and-watching) and A29's Registry/Wake invariant (see-and-wake every thread) with the missing third leg: *discover-and-register every process*.
*   **Two complementary reconcilers, one contract**: `sirsi thread discover` maps INTERACTIVE sessions (claude/codex/… by repo cwd, never guessing); the **Universal Thread Census** (`sirsi thread census`; `internal/router/census.go`) maps INFRASTRUCTURE processes with no repo binding — model brokers/servers (surface `gpu-server`), workers, supervisors — via the `censusMatchers` table. The census runs as a supervisor duty (10-min cadence), so any future process is caught within one cadence of first launch. It REGISTERS, never kills — governance stays with the reaper (ADR-022) and the resource broker (ADR-031).
*   **Extensibility = no misses for FUTURE threads**: every new agent-class service MUST ship its `censusMatchers` row in the same change that introduces the process. A service reachable only by `ps` archaeology is a governance failure under Ma'at.
*   **The Work Board** (`sirsi router workboard [--json]`, `internal/router/workboard.go`): every agent's work packages (title/sender/age), peers (live agents + surfaces — idle live agents included), and pace (closed today/7d, avg open→close turnaround, per agent + fabric-wide). Computed on read from the items corpus + thread registry; stored nowhere.
*   **Overseer role — claude-home/Horus**: the router conduit (claude-home) is the Work Board OVERSEER: its scheduled sweeps read the board, verify the census invariant (zero unregistered agent-class processes), escalate misses as router items, and publish board state to the ambient surfaces. Pantheon owns the primitives; the overseer owns the watching. An overseer sweep that cannot run leaves the supervisor duty as the machine-local backstop — two independent legs, no single point of blindness.
*   **Reference**: `internal/router/census.go` + `workboard.go`; `sirsi thread census`, `sirsi router workboard`; supervisor duties `thread-census` + the board consumers. Refs: A27, A29, ADR-022, ADR-031.

### 2.31 Bind Directives Cannot Supersede a Live Rejection (Rule A34)
> Established August 3, 2026. Ratified by claude-home on the recommendation of codex-pantheon (router `20260803-170255`). Root cause: PR #416 merged after a head-pinned `CHANGES_REQUESTED` review was dismissed and replaced by an approval under a standing bind directive; the rejected defect was real and reached `main` (repaired by PR #431).

*   **Rule**: A standing bind directive may **automate an already-positive independent verdict** — turning an existing `APPROVE` on the current head into a merge without re-asking. It MUST NEVER dismiss, override, or supersede a `CHANGES_REQUESTED` review. Blanket authorization to bind is structurally equivalent to hardcoding `event=APPROVE`: a machine-readable approval that can contradict the reviewer's actual verdict is not a verdict, it is a forgery of one.
*   **Clearing a rejection requires ONE of:**
    *   **(a) a new head** (new commits) **plus a new independent review** on that head that explicitly resolves the rejected finding; or
    *   **(b) an explicit owner override** that names the specific PR **and** the specific rejected finding it clears.
    *   A directive that predates the rejection satisfies neither — it cannot "resolve" a finding it never saw.
*   **The binder MUST fail closed**: if any review on the current head is `CHANGES_REQUESTED` and neither (a) nor (b) is present, the bind is **refused** and the item **escalates to the owner** (per the conduct runbook — ESCALATE, never act). Absence of evidence that a rejection was cleared is treated as an un-cleared rejection.
*   **Why this is Scope-The-Check-shaped** (Rule "Scope The Check To The Claim"): "the directive authorizes this bind" *claims* the reviewer approved; its actual *scope* is "the owner once said auto-bind low-risk PRs." Those two differ exactly at a live rejection — the false-assurance gap that rule forbids. A bind is a check on the reviewer's verdict; scoped to its claim, it must read the *current* verdict, not a standing intent.
*   **Mechanical enforcement**: `scripts/bind/sirsi-bind.sh` and `scripts/router/sirsi-claude-worker.sh`'s bind path query `gh pr view <pr> --json reviews` for the current head SHA before binding. Any `CHANGES_REQUESTED` review on that exact head blocks the bind (`--request-changes`-style refusal) unless a later review on the same head is `APPROVED`, or the caller passes an explicit `--override-pr <n> --override-finding "<text>"` naming both. Fails closed on an API error (treat as unknown = uncleared, never as cleared).
*   **Custodian**: 𓆄 Ma'at (A17). Refs: A23 (owner is the sole arbiter of overrides), A26 (router handoff), A28 (CI/branch protection is the other half — a bind never merges past a red gate either).

---

### 2.32 Scope The Check To The Claim (Rule A35)
> **Renumbered 2026-08-05.** This rule shipped as §2.26 / Rule A29 — numbers
> already held by §2.26 Orchestration Brain (Rule A29). Two different rules
> answered to the same citation, so an agent resolving "A29" got whichever it
> happened to find first. That is the Rule 14/17 collision again (see PR #491,
> where every Ra-deployed agent was told to override "Do No Harm" because a
> list ordinal was written as a rule tag). Orchestration Brain keeps A29 — it is
> older and carries ~32 references against this rule's ~9. **Older citations of
> "A29" that mean scope-the-check refer to THIS rule; both numbers are load-
> bearing in the archive, so neither is silently rewritten.**
> Established July 27, 2026, after a single day in which five independent defects — three found by codex, two by claude-home — turned out to be the same shape.

**Rule**: A check, guard, cap, probe or status MUST be scoped to the full extent of the claim it makes. If it cannot cover the claim, it MUST narrow the claim instead. A check narrower than its claim is worse than no check: it converts an unknown risk into a false assurance, and nobody re-examines a thing that reads fine.

**The five instances, all 2026-07-27, all in merged or deployed code:**

| the claim | the actual scope | what it cost |
|---|---|---|
| "all 210 font sites scale, 0 unscaled" | one file (`Views.swift`) | 16 live bypasses; the owner's menubar stayed broken after the "fix" |
| "the wake loop now logs" | one condition (depth *change*) | a wedged loop and a healthy loop leave identical records |
| "the broker is capped at 20.8 GiB" | one allocator (MLX's) | 43.94 GB footprint; three OOM kills in 24h |
| "Phase 4 — all four deliverables shipped" | graded by its own author | a required `DEPRECATED` warning was never shipped and was marked complete |
| "the fork storm is *the* cause of the OOM" | one window (before 22:17Z) | a third Jetsam fired 21 min later from a different consumer |

Two more from the same week, same shape: `sirsi diagnose` reporting **100/100 across 16 signals** while macOS displayed *out of application memory* (none of the 16 measured swap headroom or process growth); and `isCapacityCappedGemmaBroker`, which **exempted Sirsi's own broker** from the memory-hog check on the premise that two other checks would catch it — both of which also sampled the wrong metric.

**How to apply — four questions before a check ships:**

1. **What exactly does this assert?** Write the sentence. "All fonts on the surface scale" is a different claim from "all fonts in this file scale."
2. **What does it actually read?** One file, one metric, one process, one window, one allocator. Name it.
3. **Where do 1 and 2 differ?** That gap is the false assurance. Close it, or rewrite the claim to match the scope.
4. **Can it fail?** A guard that has never been shown red is an untested guard. Verify BOTH directions — clean passes, and a deliberate regression fails and names itself. Prefer a regression fixture that exercises the *widest* part of the claim (a second file, a second process, a second window), because the collapse-back-to-one is the failure mode.

**Corollaries:**

*   **A self-graded phase is not closed.** Marking your own work complete requires independent review; "I am grading my own work" in a review request does not excuse the grade.
*   **A cap enforced inside the thing it caps is not a cap.** Enforcement belongs outside the governed process, reading what the kernel judges by.
*   **Exempting your own component is the strongest smell in this list.** Sirsi's local model is the most likely offender on a developer's machine and must be the first thing named, never the one thing skipped.
*   **A cause established in one window is *a* cause.** Check whether the symptom recurred after the fix.

**Enforcement**: Ma'at and review treat an unscoped claim as a defect even when the code is correct, because the record is the thing later work depends on. Where a scope gap cannot be closed now, the claim MUST be narrowed in the same change, with the residual named.

## 3. Technology Stack

> **Platform scope (ADR-032 — Mac-first):** build targets are **Mac only** today (darwin/arm64 + darwin/amd64) in the order CLI → Menubar → TUI → GUI. The cross-platform language/build properties below are *latent capability*, not current targets — Windows/Linux are deferred 3–6mo and demand-gated. **Rule A3 carve-out:** cross-platform agent/CLI binaries are deferred until the fleet/Ra phase AND cross-platform demand.

| Layer | Technology | Decision |
| :--- | :--- | :--- |
| **Language** | **Go 1.22+** | Single static binary; cross-compile *capable* but **Mac-targeted today** (ADR-032), contributor-friendly |
| **CLI Framework** | **cobra** | Subcommands, auto-complete, help generation |
| **Terminal UI** | **lipgloss + table** (charmbracelet) | Styled CLI output (tables, headers, progress) for v0.23. New Mole-grade TUI follows under ADR-020 / Hybrid C. |
| **Interactive Surface** | **Mac-first surface ladder (ADR-032): CLI → Menubar → TUI → Mac desktop GUI** (built FROM the menubar); native macOS SwiftUI is the GUI path | v0.22 BubbleTea TUI removed in v0.23 per ADR-018; surface direction closed as Hybrid C per ADR-020 (2026-05-29). Mac-only build targets per ADR-032 (Windows/Linux TUI deferred). No `internal/tui/` code lands before `docs/TUI_DESIGN_PROOF.md` clears codex review. |
| **Agent Protocol** | **gRPC** (fallback: SSH+JSON) | Streaming results, bidirectional |
| **Config** | **yaml.v3** (structured YAML) | User-defined rules, profiles, budgets. (viper was listed aspirationally but never adopted — every config consumer uses gopkg.in/yaml.v3; ADR-034 Alt 5) |
| **Network Discovery** | **nmap** wrapper + native ARP/mDNS | Subnet/VLAN host discovery |
| **Docker** | **docker/client** SDK | Native Docker API |
| **Kubernetes** | **client-go** | Native K8s API |
| **SSH** | **golang.org/x/crypto/ssh** | Native Go SSH client |
| **Build** | **goreleaser** | Mac binary releases today (darwin arm64/amd64); multi-platform deferred per ADR-032 |
| **CI/CD** | **GitHub Actions** | Build, test, release |
| **Distribution** | **Homebrew tap** + GitHub Releases | `brew install sirsi-pantheon` |

---

## 4. Canonical Documents (sirsi-pantheon)

These documents are the source of truth for this repo:

> **Status marker.** A path below marked **(NOT YET WRITTEN)** does not exist in
> this repo. It is listed because canon says it *should* exist, not because it
> does. Rule 16 (Mandatory Canon Review) requires re-reading the relevant
> canonical documents before writing code — a list naming files that were never
> created makes that rule literally unfollowable, and silently teaches every
> agent to cite a document nobody can open. Create the file or delete the line;
> do not leave it ambiguous.


### 🏛 Governance (3)
1.  `PANTHEON_RULES.md` (this file — canonical; synced to `GEMINI.md` and `CLAUDE.md`)
2.  `docs/PROJECT_SCOPE.md` **(NOT YET WRITTEN)**
3.  `CONTRIBUTING.md`

### 🏗 Architecture & Design (4)
4.  `docs/ARCHITECTURE_DESIGN.md`
5.  `docs/TECHNICAL_DESIGN.md` **(NOT YET WRITTEN)**
6.  `docs/SAFETY_DESIGN.md`
7.  `docs/SCAN_RULE_GUIDE.md`

### ⚖️ Compliance & Security (3)
8.  `SECURITY.md`
9.  `docs/SECURITY_COMPLIANCE.md` **(NOT YET WRITTEN)**
10. `docs/RISK_MANAGEMENT.md` **(NOT YET WRITTEN)**

### 🚀 Operations (3)
11. `docs/DEPLOYMENT_GUIDE.md` **(NOT YET WRITTEN)**
12. `docs/QA_PLAN.md`
13. `docs/VERSIONING_STANDARD.md` **(NOT YET WRITTEN)**

### 🧠 Knowledge & Decisions (4)
14. `docs/ADR-INDEX.md`
15. `docs/ADR-TEMPLATE.md`
16. `CHANGELOG.md`
17. `VERSION`

### 🔧 CI/CD (2)
18. `.github/workflows/ci.yml`
19. `.github/workflows/release.yml`

### 📦 Configuration (3)
20. `configs/default_rules.yaml`
21. `configs/example_policy.yaml`
22. `configs/network_example.yaml` **(NOT YET WRITTEN)**

---

## 5. Brand Identity

| Element | Value |
|---------|-------|
| **Name** | Sirsi Anubis |
| **CLI** | `sirsi` |
| **Agent** | `sirsi-agent` |
| **Colors** | Gold (`#C8A951`) + Black (`#0F0F0F`) + Deep Lapis (`#1A1A5E`) |
| **Icon** | Jackal silhouette in Egyptian profile |
| **Motto** | *"Weigh. Judge. Purge."* |
| **Tagline** | *"The Guardian of Infrastructure Hygiene"* |

---

## 6. Interaction Protocol
*   **User**: "I want X."
*   **Agent Response**: "I see you want X. However, analyzing `ADR-001`, Y might be better because [Reason]. Should we do Y? If you insist on X, here is the risk."
*   **Artifacts**: Use `implementation_plan.md` to structure complex thought.

---

## 7. Agent Capabilities
*   **CLI Access**: Full CLI access to GitHub and local filesystem.
*   **Pipeline Visibility**: Full CI/CD pipeline access via `gh` CLI.
*   **Push Protocol**: ALWAYS run `git status` -> `git add` -> `git commit` -> `git push`.
*   **Identity**: `SirsiMaster` account exclusively.
*   **Build Verification**: After ANY code change, run `go build ./cmd/sirsi/` and `go test ./...` before committing.

---

## 8. Phased Roadmap

| Phase | Codename | Scope |
|-------|----------|-------|
| **1** | **Jackal** | Local CLI — workstation scan, clean, RAM guard, Spotlight fix |
| **2** | **Jackal+** | Container/VM scanning, AI/ML rules, offline disk scan |
| **3** | **Hapi** | VRAM management, storage optimization, resource flow balancing |
| **4** | **Scarab** | Agent-controller, VLAN/subnet discovery, fleet sweep |
| **5** | **Scarab+** | SAN/NAS/S3 scanning, storage backends |
| **6** | **Scales** | Policy engine, fleet-wide enforcement, reporting |
| **7** | **Temple** | Web dashboard / native SwiftUI GUI |

---
**Canonical source**: `PANTHEON_RULES.md`
**Auto-synced to**: `GEMINI.md`, `CLAUDE.md`
