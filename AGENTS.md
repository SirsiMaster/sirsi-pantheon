# PANTHEON_RULES.md

## Sirsi Portfolio Manifesto

Before non-trivial work, read `$HOME/Development/SIRSI_MANIFESTO.md` (workspace-root-relative;
see `docs/WORKSPACE_PATH_CONVENTION.md` for gate-script resolution). It governs Pantheon's
portfolio role, Mac-first product and surface strategy, relationship to
Nexus/Hypergraph/SNE/I/O, product-design standard, public presentation, and integrated
completion. Pantheon-local canon governs mechanisms and must be amended where it conflicts.

## Development Workspace Router Law

This repo inherits the workspace-wide agent law at `/Users/thekryptodragon/Development/AGENTS.md`.

All AI agents, regardless of vendor or model family, must follow that Development-root law plus this repo's local rules. Before non-trivial work, read:

1. `/Users/thekryptodragon/Development/AGENTS.md`
2. this repo's `AGENTS.md`
3. `/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router/state.json`
4. `/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router/agents.json`
5. any router item addressed to this repo's registered agent id

Router etiquette is mandatory: use `/plan`, `/goal`, ETA fields, repo-segmented ownership, verification evidence, and router writeback. Agent type does not matter; Codex, Claude, Gemini, Gemma, Qwen, and future agents obey the same contract.

**Operational Directive for All Development Agents (sirsi-pantheon)**
**Version:** 3.0.0 (v0.23.8-beta Release)
**Date:** July 1, 2026

---

## Universal Idea Router Startup Protocol

Every AI agent working in this repository, including Codex, Claude, Gemini, Qwen, or future agents, MUST learn and use the Idea Router before starting non-trivial work.

1. Read this `AGENTS.md` first.
2. Use the repo-local router at `.agents/idea-router/`.
3. Read router `state.json`, `README.md`, and any pending item addressed to this repo's registered agent id before beginning unrelated work.
4. New work must include `/plan` and `/goal` and must continue until the `/goal` is met, blocked with evidence, or impossible with a stated reason.
5. Work is repo-segmented by default. Do not edit another repository unless a written super-agent mandate names the repos and grants that scope.
6. Router targets must use registered agent ids such as `codex-pantheon`, `claude-pantheon`, or another entry in `.agents/idea-router/agents.json`.

User shorthand: `ctr` means check the router.

## Router Addressing Law

Every router item must be addressed to exactly one repo-scoped agent unless a written super-agent mandate exists.

Use this addressing formula:

```text
<agent-family>-<repo-or-workstream>
```

Examples:

- FinalWishes repo review for Claude: `claude-finalwishes`
- FinalWishes repo review for Codex: `codex-finalwishes`
- Pantheon router/CLI work for Claude: `claude-pantheon`
- Sirsi Nexus work for Codex: `codex-nexus`
- Assiduous work for Claude: `claude-assiduous`

Do not address FinalWishes work to `claude-pantheon` just because the router lives in Pantheon. Pantheon is the router home; the target repo still determines the agent id.

### Choosing The Target Agent

1. Identify the repo that owns the implementation or review.
2. Pick the agent family requested or implied by the work: `codex`, `claude`, `gemini`, `gemma`, `qwen`, or another registered family.
3. Look up the exact id in `/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router/agents.json`.
4. Put the item under `pending.<agent_id>` only.
5. Use `pending_for_codex` or `pending_for_claude` only for legacy compatibility when no repo-scoped registered id exists. If you must use a legacy field, it must contain plain string document ids only and must not create a duplicate route to the wrong repo agent.

### State JSON Shape

`state.json` pending queues are machine-readable and must remain arrays of strings:

```json
{
  "pending": {
    "codex-finalwishes": ["20260520-example-doc-id"],
    "claude-pantheon": []
  },
  "pending_for_codex": [],
  "pending_for_claude": []
}
```

Never put metadata objects inside `pending`, `pending_for_codex`, or `pending_for_claude`. Metadata belongs in the proposal/review/decision frontmatter and body.

Invalid:

```json
{
  "pending": {
    "codex-finalwishes": [
      {
        "id": "20260520-example-doc-id",
        "eta_for_review": "2026-05-20T22:00:00-04:00"
      }
    ]
  }
}
```

That object-valued form breaks the Go router parser and stalls automation.

### Required Artifact Frontmatter

Each routed artifact should include:

```yaml
id: 20260520-agent-repo-topic
author: claude-finalwishes
addressed_to: codex-finalwishes
topic: finalwishes-tier1-ga
repo: /Users/thekryptodragon/Development/FinalWishes
agent_scope: repo-segmented
eta_for_review: 2026-05-20T22:00:00-04:00
next_check_at: 2026-05-20T22:00:00-04:00
estimated_duration: 1 hour
```

### Super-Agent Exception

A broad coordinator may route or edit across repos only when a router artifact explicitly names it as a super agent and lists:

- repositories in scope
- whether it may edit or only coordinate
- repo-scoped implementation owners
- verification evidence required before `/goal` completion

Without that mandate, route work to the repo owner agent and stop there.


## Thread Registration Law

Every agent thread or session must register with CTR at startup, not only the agent binary/profile.

An agent id answers "who can receive work?" A thread id answers "which open conversation or worker instance is alive right now?" Horus owns the per-desktop thread surface.

At startup, every Codex, Claude, Gemini, Gemma, Qwen, or future agent thread must:

1. determine its registered `agent_id`
2. create or refresh a `thread_id`
3. record repo path, workstream, start time, last heartbeat, wake mechanism, and inbox subscription
4. read pending CTR items addressed to its `agent_id`
5. either work the item, route it, or mark it blocked with evidence
6. keep heartbeat/status fresh until the thread exits or becomes inactive

Horus node status must show active threads, stale threads, blocked wake surfaces, and which inboxes each thread is watching. A thread that can receive notification but cannot wake or cannot write back is not healthy.
### Caffeinate Contract (universal)

Every AI agent thread must stay caffeinated for as long as its host process is alive. "Caffeinated" means CTR's `idle_seconds` for the thread stays near zero without depending on user prompts. The canonical pattern any agent can implement:

1. **On session start (one-shot):** call `sirsi thread register --agent <agent_id> --surface <kind> --repo <path>`. If a fresh active thread for `<agent_id>` already exists (idle < 5 min), adopt it instead of registering a duplicate.
2. **Immediate heartbeat:** `sirsi thread heartbeat --thread <thread_id> --quiet` so the thread shows idle 0 right away.
3. **Background caffeinator (one per session):** spawn a detached loop anchored to the host process PID:
   ```bash
   ( while kill -0 <host_pid> 2>/dev/null; do
       sirsi thread heartbeat --thread <thread_id> --quiet
       sleep 60
     done; rm -f /tmp/sirsi-caffeinate-<thread_id>.pid ) &
   disown
   ```
   Dedupe via `/tmp/sirsi-caffeinate-<thread_id>.pid` so multiple session-start signals don't stack loops.
4. **On host exit:** `kill -0` fails, loop exits, pidfile cleared. CTR sees the thread go stale on its own. Optionally call `sirsi thread close --thread <thread_id>` from a SessionEnd hook for explicit closure.

Reference implementation: `.claude/hooks/router_inbox_check.py` in `sirsi-pantheon`. Codex, Gemini, Gemma, Qwen, and future agents should implement an equivalent in whatever hook/automation surface their runtime exposes. The contract is in this section; the implementation is per-runtime.

Codex.app implementation note: until Codex exposes a managed SessionStart/SessionEnd hook or long-lived background-process API, the supported Codex variant is the `ctr-thread-wake` heartbeat automation. That automation is prompt-tick based rather than PID-anchored, so it must read the router queue, process Codex-addressed work, and stay quiet when there is no action. If Codex later exposes a durable per-thread hook, replace this note with the canonical PID-anchored caffeinator loop above.

Why this matters: orphan threads (host alive but CTR sees stale) hide live work, and ghost threads (host dead but CTR sees active) cause routing to wrong inboxes. The caffeinate loop closes both failure modes with one anchor: the host process PID.


## Ra Owns CTR

CTR and the Idea Router are owned by Ra and homed in this Pantheon repo. Horus owns the per-desktop node and local operator view. Other repos inherit router law through their `AGENTS.md` files, but they must not create competing router homes. Ra owns orchestration, dispatch, registry, work queue law, and super-agent authority. Horus owns this workstation's local agent/window visibility, daemon health, repo status, and desktop control surface. Thoth preserves memory, Ma'at validates governance, and Net keeps work aligned to portfolio goals.
## Net Goal-Weaving & Architecture Doctrine

Net 𓁯, The Weaver, keeps every workstream aligned to its full `/goal`, product surface target, and architecture decisions. Agents must work independently where possible and must include `eta_for_review`, `next_check_at`, or `estimated_duration` in router handoffs.

- Simplify vendor surface area: prefer GCP-native, Firebase-native, or open-source tools before paid third-party vendors.
- Prefer PostgreSQL/Cloud SQL/AlloyDB for canonical business truth in product repos; use SQLite for Pantheon local state, router ledgers, caches, and offline indexes.
- Use Firestore for real-time UX state and notifications in product repos; use Cloud Storage for files and exports.
- Do not add a paid vendor without an ADR explaining why GCP/open-source is insufficient, data exposure, cost risk, exit path, and replacement plan.
- Do not reopen language choices without a measured blocker and ADR. Pantheon remains Go for CLI/TUI/daemon/router work. Rust is reserved for future native desktop/local hardened components when justified.
- Product surface target: local desktop and web.

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

### Deity Hierarchy (ADR-015)

**Horus 𓂀 — Local Workstation Lord (Free Tier)**
Everything on ONE machine reports to Horus. The dashboard IS Horus.

| Module | Codename | Archetype | Reports To | Role |
| :--- | :--- | :--- | :--- | :--- |
| Local Scanner | **Jackal** 🐺 | The Hunter | Horus | Patrols and cleans individual machines |
| Ghost Hunter | **Ka** 𓂓 | The Spirit | Horus | Detects dead app remnants and residual hauntings |
| Policy Engine | **Scales** ⚖️ | The Judgment | Horus | Weighs findings against defined policies |
| Resource Optimizer | **Hapi** 🌊 | The Flow | Horus | Controls VRAM, GPU memory, and storage flow |
| Output Filter | **RTK** ⚡ | The Sieve | Horus | Strips noise from tool output before it hits AI context |
| Context Vault | **Vault** 🏛️ | The Keeper | Horus | Sandboxes large output in SQLite FTS5, indexes code for BM25 search |
| Code Graph | **Horus** 𓂀 | The All-Seeing | — | Structural code symbols, live file watching, workstation dashboard |

**Ra 𓇶 — Fleet Lord (Enterprise Tier)**
Receives reports from all Horus instances. Orchestrates across all endpoints.

| Module | Codename | Archetype | Role |
| :--- | :--- | :--- | :--- |
| Fleet Sweep | **Scarab** 🪲 | The Transformer | Rolls across VLANs, subnets, domains |
| Fleet Neith | **Net** 𓁯 | The Weaver | Cross-endpoint alignment and drift detection |

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
*   **Scope**: This rule applies to ALL Sirsi repositories and ALL AI agents (Antigravity, Codex, Gemini, Cursor, Windsurf). It is not project-specific.
*   **Enforcement**: An AI agent that guesses instead of asking (Axiom 3), or codes before understanding (Axiom 4), has violated Truth Vector. The resulting work must be reviewed before canonization.
*   **Custodian**: The user is the sole arbiter of Truth Vector compliance.

### 2.21 Ra Scope Autonomy (Rule A24)
> Established April 3, 2026, after 4 Ra-deployed agents blocked indefinitely waiting for sprint plan approval that could never arrive in non-interactive mode.

*   **Rule**: Ra scope configs (`configs/scopes/*.yaml`) define **pre-approved sprint plans**. Agents spawned by `sirsi ra deploy` MUST execute scopes without asking for human approval. The Neith loom (`internal/neith/loom.go`) injects a **Ra Autonomy Directive** at the top of every woven prompt that overrides Rule 17 (Sprint Planning is Mandatory).
*   **Scope Authoring**: Scopes MUST be written as directive, numbered task lists — not vague descriptions. Each task must name specific files, paths, or concrete actions. Vague scopes cause agents to ask clarifying questions, which hang forever in `--print` mode. See `configs/scopes/README.md` for the full authoring guide.
*   **Prompt Structure**: The autonomy directive and scope of work are placed at the **top** of the woven prompt and are **never truncated**. Canon context (AGENTS.md, Thoth memory, ADRs) fills the remaining token budget and may be truncated.
*   **Permission Model**: Ra agents run with `--dangerously-skip-permissions` because the scope is pre-approved. This flag MUST NOT be used outside of Ra-deployed agents.
*   **Streaming Output**: Ra agents MUST use `--output-format stream-json --verbose` with `--print`. Default `--print` mode buffers ALL output until the session completes, making agents appear lifeless for 10+ minutes. The stream-json output is piped through a python filter (`terminal.go`) that extracts human-readable text and tool-use summaries, writing to both the terminal (live progress) and the log file (Ra monitoring).
*   **Evidence**: Session where `sirsi ra deploy` spawned 4 windows; all 4 agents asked for approval and blocked. Root causes: (1) AGENTS.md Rule 14 conflict, (2) vague scope descriptions, (3) directive placed after canon context and truncated, (4) `--print` default text mode buffered all output making agents appear dead.

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
*   **Ra/Horus Ownership**: The Idea Router belongs to 𓇶 Ra and is homed in Pantheon. Ra owns agent registry, work queue law, dispatch protocol, relay, portfolio authority, and super-agent mandates. Horus owns the per-desktop runtime node: local daemon health, local agent/window visibility, local repo status, and workstation operator surface. Thoth preserves router memory; Ma'at validates router governance.
*   **Goal Flag**: Every workstream MUST define a `/goal` flag in the plan. The `/goal` is the explicit completion condition, including required verification, tests, review, and handoff artifacts. Agents continue working until the `/goal` is met, blocked by safety/user approval, or impossible with a stated reason.
*   **Repo Segmentation**: Work on repositories MUST be segmented. Each repository requires its own agent/workstream. A single agent MUST NOT modify multiple repositories unless it is explicitly designated as a **super agent** with a written cross-repo mandate in the `/plan`.
*   **Super Agent Mandate**: A super agent may coordinate multiple repo agents, compare evidence across repos, and write cross-repo decisions, but MUST avoid direct code edits across repos unless the mandate explicitly permits those paths.
*   **Parallel Agents**: When enough context and token budget exist, spawn multiple repo-scoped agents rather than serializing unrelated repo work. Each agent owns one repo and one bounded task set.
*   **Idea Router Handoff**: Proposals, reviews, decisions, and completion notes MUST be written to `.agents/idea-router/`. A submission by Codex should create a pending item for Claude; a submission by Claude should create a pending item for Codex.
*   **Completion Relay**: Agents MUST continue the relay until the `/goal` is met. If the current environment cannot automatically wake the other agent, the submitting agent MUST leave an explicit pending router item and a concise next-action instruction.
*   **ETA-Driven Router Checks**: Router handoffs MUST include `eta_for_review`, `next_check_at`, or a clearly stated estimated duration. Codex/universal responder checks should be scheduled near those timestamps, not by fixed high-frequency polling. Missing ETAs are incomplete handoffs unless the item is urgent or already complete.
*   **Interim Universal Responder**: Until the multi-agent response fabric is implemented, Codex is the universal responder for router requests. Claude agents and other workstream agents may address review, triage, and routing questions to Codex. This grants coordination authority, not unbounded cross-repo edit authority; implementation remains repo-segmented unless a super-agent mandate exists.
*   **Codex Startup Router Check**: Every Codex workstream that starts in this repo MUST read `.agents/idea-router/state.json`, `.agents/idea-router/README.md`, and any pending items addressed to its registered agent id before beginning unrelated work. If addressed work exists, it must either act on it, mark it blocked with evidence, or explicitly defer it in a router artifact.
*   **Registered Agent Addressing**: Router v3 work MUST target concrete registered agent ids such as `codex-pantheon`, `claude-pantheon`, or another entry in `.agents/idea-router/agents.json`. Generic `codex` and `claude` targets are legacy compatibility only and must migrate to registered ids.
*   **Launch Prompt Contract**: Any router-launched Codex agent MUST receive a prompt that names its `agent_id`, repo root, topic, `/plan`, `/goal`, expected writeback artifact, and completion criteria. The agent must write back before considering the job complete.
*   **Thoth Router Memory**: Thoth compact/sync workflows MUST preserve router state: active topics, pending items by agent id, `/goal` status, next required agent/action, dispatch ledger state, and blockers. Context compaction that drops unresolved router work is a governance failure.
*   **No Silent Cross-Repo Drift**: Any claim about repo state, completion, test status, or deployment status must name the repo and cite evidence gathered in that repo.
*   **Enforcement**: Ma'at treats unmandated cross-repo edits, missing `/plan`, missing `/goal`, or unclosed router handoffs as governance failures.
*   **Automation Boundary (superseded by ADR-057)**: the durable router store is dispatch authority. Files are audit/export views, not execution authority. Provider adapters may wake Codex, Claude, Gemini, Gemma, Qwen, MCP, API, webhook, or future workers, but a wake is successful only after a real source-store claim/audit action acknowledges it.

### 2.24 Heartbeat Loop Mandate (Rule A27)
> Established June 1, 2026. A registered router thread that is not looping is invisible to its own inbox. Extends A26 (Completion Relay): registration means "alive and watching," not merely "known."

*   **Rule**: Every agent thread that registers with the router (`sirsi thread register`) MUST run a persistent heartbeat loop — a wake-loop that watches its inbox — from registration until it de-registers (`sirsi thread close`). Registered-but-not-looping is a node-health failure under Horus `router node-status` and a governance failure under Ma'at.
*   **The store mutation is the work heartbeat (ADR-057)**: provider-specific loops are delivery adapters only. Every worker follows `pull → lease → bind → execute → record evidence → close → pull again`. Session heartbeat and process existence prove liveness only. Horus classifies work from leases and recent task-record mutations, and the supervisor re-wakes or escalates lanes that are idle with durable work.
*   **Loop scope**: The heartbeat loop is a *watcher*, not a work driver. Its job is minimal and bounded: pull the inbox, act on or queue new items, emit `sirsi thread heartbeat`, sleep. Prefer event-driven waking (file Monitor on `items/`) over fixed polling, with a long fallback tick so a missed event never strands the thread.
*   **Lifecycle binding**: One loop per thread. Start it at register, stop it ONLY at `thread close`. De-registration is the single clean way to end the loop — never abandon a registered thread with no loop.
*   **Why**: Without a live loop, items addressed to a registered thread sit unread until a human types `ctr`. Codex already approximates this via its heartbeat automation (`ctr-thread-wake`); this rule gives every Claude thread the same parity so the multi-agent relay (A26) actually completes without manual nudging.
*   **Resident UI surfaces are nodes too** (added 2026-06-01): An interactive surface that can initiate work or take operator interaction — **menubar, TUI, IDE plugin, SwiftUI/macapp** — is a router-registered thread, not merely a renderer. It registers bound to its **own process PID** and heartbeats from its **native runloop on a bounded interval (≥60s)** — never on a frequent render/stats tick, which floods the registry and feeds Spotlight `mds_stores` (the 2026-06-01 lockup). The heartbeat proves liveness to Horus/Ra; a surface that does not act on inbox items need not run the full watcher loop. Close on graceful shutdown (SIGTERM/quit); hard kill falls back to OS-truth reaping (ADR-022). Registration MUST be idempotent on `(agent_id, pid)` so surface restarts never accumulate duplicate active records. Surface ids: `menubar`, `tui`, `vscode`/`jetbrains`/`cursor`, `macapp`.
*   **Reference**: `.agents/idea-router/README.md` § "Heartbeat Loop (mandatory from register → close)".

---

## 3. Technology Stack

| Layer | Technology | Decision |
| :--- | :--- | :--- |
| **Language** | **Go 1.22+** | Single static binary, cross-compile, contributor-friendly |
| **CLI Framework** | **cobra** | Subcommands, auto-complete, help generation |
| **Terminal UI** | **lipgloss + table** (charmbracelet) | Styled CLI output (tables, headers, progress) for v0.23. New Mole-grade TUI follows under ADR-020 / Hybrid C. |
| **Interactive Surface** | **New Mole-grade TUI** (in design) first on macOS/Windows/Linux + CLI on all platforms; native macOS SwiftUI as a later-phase polish-bar upgrade | v0.22 BubbleTea TUI implementation removed in v0.23 per ADR-018; surface direction reopened and closed as Hybrid C per ADR-020 (2026-05-29). No `internal/tui/` code lands before `docs/TUI_DESIGN_PROOF.md` clears codex review. |
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
1.  `ANUBIS_RULES.md` (this file — canonical; synced to `GEMINI.md` and `AGENTS.md`)
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
**Auto-synced to**: `GEMINI.md`, `AGENTS.md`

## Lean Engineering Doctrine

These principles are derived from a multi-day collapse of overengineered router infrastructure (push-model daemons, dispatch ledgers, snowflake IDs, polling timers, agent registries, NOTIFY env gates) into ~150 LOC of one Go package + four CLI verbs + one launchd plist. Net -454 LOC across the refactor. Apply these BEFORE proposing architecture, not as a post-hoc trim. These principles are universal across vendor and model family (Codex, Claude, Gemini, Gemma, Qwen, future agents).

1. **Question polling before tuning it.** Before adding a "every N seconds" timer, ask: is there an event source — `WatchPaths`, `inotify`, FSEvents, git hook, Claude Code hook event, webhook, MCP notification — that fires only when state actually changes? Polling is the right shape ONLY when the source of truth is remote (HTTP API) or you genuinely need continuous samples (CPU, RAM, frame timing). For local file state, event-driven wins on lean every time. If you find yourself tuning an interval, you are usually one layer too deep — the question is whether the loop should exist at all.

2. **No belt-and-suspenders.** If the primary mechanism already fails loud (cobra error → stderr → log, non-zero exit, exception bubbles up), do not add a second-tier validator/canary/guard on top. Each extra "safety check" is noise that drifts, rots, and obscures the actual failure when something goes wrong. One loud failure path beats two quiet ones.

3. **Replace, don't accrete.** When a new mechanism subsumes an old one, default to deletion of the old, not coexistence. "Additive-only" is a safety rule for in-flight refactors, not a permanent policy. Once the new path is verified end-to-end, the old code is dead weight. Track destruction in proportion to addition: a refactor PR that adds 500 LOC without deleting any is a smell.

4. **Smallest package wins.** One file beats two. One config beats two configs plus a wrapper script. One CLI verb beats four verbs that compose to the same operation. When the answer is "add a launchd plist," it is 30 lines of XML, not a new Go subcommand wrapping launchctl. When the answer is "register a thread," it is `sirsi thread register`, not a registry-orchestrator framework.

5. **Three options that look the same is no choice.** When presenting alternatives to the user, the options must differ on a load-bearing axis: deployment, durability, blast radius, latency, failure mode. If they differ only on cosmetics or framing, collapse to ONE recommendation. Asking the user to pick among indistinguishable shapes wastes their attention and signals architectural confusion.

6. **Question the model, not just the parameter.** When the user asks for "every 4 minutes," they may actually mean "wake on activity, not on a schedule." Anchoring on the literal parameter and tuning it is a common failure mode. The lean answer is usually one architectural layer up from where the question landed. Solve the underlying need, not the literal request, then report what you did.

7. **Direct communication.** Lead with the verdict or the result, not the preamble. End-of-turn summary is one or two sentences. No over-apology, no recap, no thanks-for-the-question, no "Great!" or "Perfect!". Brief beats verbose. Silent beats brief when there is nothing to add. The user reads the diff, not the celebration.

8. **Polling is for remote sources only.** A local file-based queue, a local config file, a local lock file — all of these have an event source already. If a process exists solely to read a local file on an interval, it is the wrong shape. Replace with `WatchPaths`, a git hook, or fold the read into the consumer's wake path.

9. **Identity by string, not by registry.** When designing multi-actor protocols, default to "any string id can participate" rather than "named entities must register first." Registration is optional metadata for human readability, not a precondition for participation. The router collapse proved that an `agents.json` registry was not load-bearing for the actual file-based queue — any agent id that writes a file gets routed.

10. **Atomicity at the dispatch-store boundary (ADR-057 amendment).** Executable item/task/requirement transitions and their wake events commit in one durable store transaction. Filesystem documents remain canonical for authored prose where declared, but file frontmatter is not a lease, acknowledgment, or completion authority.

11. **Wake mechanisms do not own delivery semantics (ADR-057 amendment).** The durable queue is source of truth; wake adapters are observers. Delivery is acknowledged only by a fenced store action correlated to the source work (or the next same-lane claim for a synthetic continuation). Failed delivery is bounded, durable, and escalated; no process, heartbeat, or adapter-success signal can acknowledge work.

These principles are referenced as `AGENTS.md §Lean #<n>` in commit messages, ADRs, and router proposals. Cite, do not paraphrase.
