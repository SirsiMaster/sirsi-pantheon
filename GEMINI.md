# ANUBIS_RULES.md
**Operational Directive for All Development Agents (sirsi-anubis)**
**Version:** 1.0.0
**Date:** March 20, 2026

---

## 0. Identity
This is the **sirsi-anubis** repository — Sirsi Technologies' infrastructure hygiene platform.
An open-source CLI tool that scans, judges, and purges infrastructure waste across workstations, containers, VMs, networks, and storage backends.

- **GitHub**: `https://github.com/SirsiMaster/sirsi-anubis`
- **Local Path**: `/Users/thekryptodragon/Development/sirsi-anubis`
- **CLI Binary**: `anubis`
- **Agent Binary**: `anubis-agent`

**This repo is NOT SirsiNexusApp. This repo is NOT FinalWishes. This repo is NOT Assiduous.**
Rules, design tokens, and business logic from other repositories do NOT apply here unless explicitly inherited through Universal Rules (§1).

### Portfolio Position
| Repo | Type | Description |
| :--- | :--- | :--- |
| **SirsiNexusApp** | Platform Monorepo | Core infrastructure, shared services, UCS components |
| **FinalWishes** | Tenant Application | Estate planning platform (Royal Neo-Deco) |
| **Assiduous** | Tenant Application | Real estate platform (Assiduous Modern) |
| **sirsi-anubis** (this repo) | **Infrastructure Tool** | Infrastructure hygiene CLI + fleet management |
| **sirsi-rook** (reserved) | **Database Tool** | Database & storage orchestration |
| **sirsi-rogue** (reserved) | **Security Tool** | Cybersecurity sweeper |

### Internal Modules
| Module | Codename | Archetype | Role |
| :--- | :--- | :--- | :--- |
| Local Scanner | **Jackal** 🐺 | The Hunter | Patrols and cleans individual machines |
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

### 2.4 Commit Convention
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

Refs: ANUBIS_RULES.md, ADR-001
Changelog: v0.1.0 — Parallels scan rule
```

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
| **Distribution** | **Homebrew tap** + GitHub Releases | `brew install sirsi-anubis` |

---

## 4. Canonical Documents (sirsi-anubis)

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
| **CLI** | `anubis` |
| **Agent** | `anubis-agent` |
| **Colors** | Gold (`#C8A951`) + Black (`#0F0F0F`) + Deep Lapis (`#1A1A5E`) |
| **Icon** | Jackal silhouette in Egyptian profile |
| **Motto** | *"Weigh. Judge. Purge."* |
| **Tagline** | *"The Guardian of Infrastructure Hygiene"* |

---

## 6. Interaction Protocol
*   **User**: "I want X."
*   **Agent Response**: "I see you want X. However, analyzing `ADR-001`, Y might be better because [Reason]. Should we do Y? If you insist on X, here is the risk."

---

## 7. Agent Capabilities
*   **CLI Access**: Full CLI access to GitHub.
*   **Push Protocol**: ALWAYS run `git status` -> `git add` -> `git commit` -> `git push`.
*   **Identity**: `SirsiMaster` account exclusively.

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
**Canonical source**: `ANUBIS_RULES.md`
**Auto-synced to**: `GEMINI.md`, `CLAUDE.md`
