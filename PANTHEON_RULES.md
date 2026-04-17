# PANTHEON_RULES.md
**Operational Directive for All Development Agents (sirsi-pantheon)**
**Version:** 2.0.0 (The Pantheon Unification)
**Date:** March 24, 2026

---

## 0. Identity
This is the **sirsi-pantheon** repository — Sirsi Technologies' infrastructure hygiene platform.
An open-source CLI tool that scans, judges, and purges infrastructure waste across workstations, containers, VMs, networks, and storage backends.

- **GitHub**: `https://github.com/SirsiMaster/sirsi-pantheon`
- **Local Path**: `/Users/thekryptodragon/Development/sirsi-pantheon`
- **CLI Binary**: `sirsi`
- **Agent Binary**: `sirsi-agent`

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

### Internal Modules
| Module | Codename | Archetype | Role |
| :--- | :--- | :--- | :--- |
| Local Scanner | **Jackal** 🐺 | The Hunter | Patrols and cleans individual machines |
| Ghost Hunter | **Ka** 𓂓 | The Spirit | Detects dead app remnants and residual hauntings |
| Fleet Sweep | **Scarab** 🪲 | The Transformer | Rolls across VLANs, subnets, domains |
| Policy Engine | **Scales** ⚖️ | The Judgment | Weighs findings against defined policies |
| Resource Optimizer | **Hapi** 🌊 | The Flow | Controls VRAM, GPU memory, and storage flow |

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

*   **Cross-Platform Safety (Rule A3)**: Agent binaries (`anubis-agent`) must be statically compiled with `CGO_ENABLED=0` and zero external dependencies. They run on untrusted targets (customer VMs, containers, remote hosts). The agent MUST NOT execute arbitrary commands received from the controller — it implements a fixed, auditable command set.

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
    3. **Build** — `go build ./cmd/anubis/` and `go build ./cmd/anubis-agent/` must succeed.
    4. **Binary Size Guard** — Warning if `anubis` > 25MB or `anubis-agent` > 12MB.
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

Refs: ANUBIS_RULES.md, ARCHITECTURE_DESIGN.md, ADR-001
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

All terminal output MUST use the Anubis brand language:
*   **Colors**: Gold (`#C8A951`) for highlights, White for body text, Red for errors, Green for success. No raw unstylized output in interactive mode.
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
*   **Mandatory Canon Review**: Before writing code, re-read `ANUBIS_RULES.md`, relevant ADRs, `SAFETY_DESIGN.md`, and the files you intend to modify.
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

---

## 3. Technology Stack

| Layer | Technology | Decision |
| :--- | :--- | :--- |
| **Language** | **Go 1.22+** | Single static binary, cross-compile, contributor-friendly |
| **CLI Framework** | **cobra** | Subcommands, auto-complete, help generation |
| **Terminal UI** | **lipgloss + table** (charmbracelet) | Gold + black Egyptian theme |
| **Interactive TUI** | **bubbletea** (optional) | Rich interactive mode for guided cleanup |
| **Agent Protocol** | **gRPC** (fallback: SSH+JSON) | Streaming results, bidirectional |
| **Config** | **viper** (YAML) | User-defined rules, profiles, budgets |
| **Network Discovery** | **nmap** wrapper + native ARP/mDNS | Subnet/VLAN host discovery |
| **Docker** | **docker/client** SDK | Native Docker API |
| **Kubernetes** | **client-go** | Native K8s API |
| **SSH** | **golang.org/x/crypto/ssh** | Native Go SSH client |
| **Build** | **goreleaser** | Multi-platform binary releases |
| **CI/CD** | **GitHub Actions** | Build, test, release |
| **Distribution** | **Homebrew tap** + GitHub Releases | `brew install sirsi-pantheon` |

---

## 4. Canonical Documents (sirsi-pantheon)

These documents are the source of truth for this repo:

### 🏛 Governance (3)
1.  `ANUBIS_RULES.md` (this file — canonical; synced to `GEMINI.md` and `CLAUDE.md`)
2.  `docs/PROJECT_SCOPE.md`
3.  `CONTRIBUTING.md`

### 🏗 Architecture & Design (4)
4.  `docs/ARCHITECTURE_DESIGN.md`
5.  `docs/TECHNICAL_DESIGN.md`
6.  `docs/SAFETY_DESIGN.md`
7.  `docs/SCAN_RULE_GUIDE.md`

### ⚖️ Compliance & Security (3)
8.  `SECURITY.md`
9.  `docs/SECURITY_COMPLIANCE.md`
10. `docs/RISK_MANAGEMENT.md`

### 🚀 Operations (3)
11. `docs/DEPLOYMENT_GUIDE.md`
12. `docs/QA_PLAN.md`
13. `docs/VERSIONING_STANDARD.md`

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
21. `configs/default_policies.yaml`
22. `configs/network_example.yaml`

---

## 5. Brand Identity

| Element | Value |
|---------|-------|
| **Name** | Sirsi Anubis |
| **CLI** | `sirsi` |
| **Agent** | `anubis-agent` |
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
*   **Build Verification**: After ANY code change, run `go build ./cmd/anubis/` and `go test ./...` before committing.

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
