# SIRSI_PORTFOLIO_STANDARD.md
**Universal Governance Standard for All Sirsi Technologies Repositories**
**Version:** 2.0.0 (Pantheon Unified)
**Date:** March 23, 2026

---

## Purpose

This document defines the universal rules, required canon documents, and Pantheon tool requirements that MUST exist in every Sirsi portfolio repository. It replaces the previous `PORTFOLIO_CANONICAL_STANDARD.md` (v1.0.0, Feb 27 2026) with expanded scope: Pantheon integration, graduated rules from Anubis, and canon document templates.

Every AI agent (Gemini, Claude, Antigravity) and every developer can navigate any Sirsi repo using the same governance model.

---

## 1. Universal Rules

> These rules MUST appear in §1 of every repo's GEMINI.md/CLAUDE.md.
> They are identical across every Sirsi portfolio repo.

### Core Principles (0-4)
0.  **Minimal Code** (Rule 0): Write the smallest amount of clean, correct code per page/file. If you're layering fixes on top of hacks, **DELETE AND REWRITE**. Band-aids are technical debt. Simplicity is non-negotiable.
1.  **Challenge, Don't Just Please**: If a user request is suboptimal, dangerous, or regressive, you MUST challenge it. Provide the "Better Way" before executing the "Requested Way".
2.  **Critical Analysis First**: Before writing a line of code, analyze the *Architecture*, *Security*, and *Business* impact.
3.  **Solve the "How"**: The user provides the "What". You own the "How". Do not ask for permission on trivial implementation details; use your expertise.
4.  **Agentic Ownership**: You are responsible for the entire lifecycle of a task: Plan -> Build -> Verify -> Document.

### Sirsi Ecosystem (5-6)
5.  **Sirsi First** (Rule 1): Before building, check if it exists in the Sirsi ecosystem (UCS, shared services, other repos). We build assets, not disposable code.
6.  **Implement, Don't Instruct** (Rule 2): Build working code end-to-end. No "here's how to set it up" responses.

### Verification & Pipeline (7-9)
7.  **Test Before Declaring Done** (Rule 3): Verify zero errors in build output, browser DevTools, or test runner. If you haven't verified it technically, it's not done.
8.  **Follow the Pipeline** (Rule 4): Local -> GitHub -> Production. Never skip CI/CD.
9.  **Always Push & Verify** (Rule 5): ALWAYS push changes via git. Verify push status immediately.

### Governance (10-16)
10. **ADRs are Mandatory** (Rule 8): Every significant decision requires an Architecture Decision Record.
11. **Do No Harm** (Rule 14): You MUST NOT break any working process. A regression is worse than a missing feature.
12. **Additive-Only Changes** (Rule 15): You may ADD or IMPROVE functionality, but MUST NOT recode any module in a way that disrupts the current working state.
13. **Mandatory Canon Review** (Rule 16): Before writing code, re-read the rules file, relevant ADRs, and the files you intend to modify.
14. **Sprint Planning is Mandatory** (Rule 17): Before ANY code change, present a detailed sprint plan. No code is written until the USER approves.
15. **Living Canon** (Rule 18): These canonical documents are living documents. When new rules emerge, they MUST be codified immediately.
16. **Identity Integrity** (Rule 19): All GitHub, Firebase, and cloud identities MUST use the `SirsiMaster` account exclusively.

### Traceability & Documentation (17-19)
17. **Commit Traceability** (Rule A7): Every commit MUST include a `Refs:` footer linking to at least one canon document or ADR. Commit messages follow `type(module): description`. No orphan commits.
18. **Feature Documentation** (Rule A8): Every feature MUST have user-facing docs AND developer-facing README. A feature without documentation is an incomplete feature.
19. **Release Versioning** (Rule A13): SemVer (`MAJOR.MINOR.PATCH-channel`). `VERSION` file at root is source of truth. CHANGELOG.md updated on every release.

### Integrity & Ethics (20-22)
20. **Statistics Integrity** (Rule A14): Every public-facing number MUST be independently verifiable. Include the command to reproduce it. No projections presented as measurements.
21. **Session Definition** (Rule A15): A session = one AI conversation between continuation prompts. Not time gaps, not commit clusters. Session counts use this definition.
22. **Voice Rule**: Direct verbs only. "Built. Fixed. Refactored." Never "the user wanted" or "the user suggested." Transparency is the product.

### Pantheon Integration (23-26)
23. **𓁟 Thoth is Mandatory**: Every repo MUST have `.thoth/memory.yaml` with real project state (not skeleton). Read memory FIRST before any source files. Update after every session.
24. **🪶 Ma'at Governs Quality**: Every feature must link to canon. Canon linkage, coverage thresholds, and pipeline health are assessed. No unjustified code.
25. **Context Monitoring** (Rule A9): After every sprint, report context health (🟢/🟡/🔴). When 🟡 or 🔴: commit all, update CHANGELOG, generate fresh continuation prompt.
26. **Build-in-Public** (ADR-003 graduated): Every release updates VERSION, CHANGELOG, BUILD_LOG (if applicable), Thoth memory, and journal. Mistakes stay in the record.

---

## 2. Required Canon Documents Per Repo

> Every Sirsi repo MUST have these foundational documents.
> The content is specific to each repo, but the categories are universal.

### Tier 1: Mandatory (Every Repo)

| Document | Purpose | Location |
|:---------|:--------|:---------|
| **Rules File** | Universal rules (§1) + repo-specific rules (§2) | `GEMINI.md` or `<REPO>_RULES.md` |
| **CLAUDE.md** | Synced copy of rules for Claude agents | Root |
| **GEMINI.md** | Synced copy of rules for Gemini agents | Root |
| **README.md** | What it is, how to run, how to contribute | Root |
| **SECURITY.md** | Security policy, vulnerability reporting | Root |
| **CONTRIBUTING.md** | How to contribute, code standards | Root |
| **CHANGELOG.md** | Released changes per version | Root |
| **VERSION** | Current semver version | Root |
| **ARCHITECTURE_DESIGN.md** | System architecture, component diagram | `docs/` |
| **ADR-INDEX.md** | Index of all architecture decision records | `docs/` |
| **ADR-TEMPLATE.md** | Template for new ADRs | `docs/` |
| **CONTINUATION-PROMPT.md** | Session handoff document | `docs/` |
| **.thoth/memory.yaml** | Thoth project memory (REAL, not skeleton) | `.thoth/` |
| **.thoth/journal.md** | Engineering decision journal | `.thoth/` |
| **.agent/workflows/session-start.md** | Session startup workflow | `.agent/workflows/` |

### Tier 2: Required for Applications (SirsiNexusApp, FinalWishes, Assiduous)

| Document | Purpose | Location |
|:---------|:--------|:---------|
| **TECHNICAL_DESIGN.md** | Technical implementation details | `docs/` |
| **DEPLOYMENT_GUIDE.md** | How to deploy to production | `docs/` |
| **SECURITY_COMPLIANCE.md** | Compliance requirements (SOC2, HIPAA, etc.) | `docs/` |
| **RISK_MANAGEMENT.md** | Risk register and mitigations | `docs/` |
| **QA_PLAN.md** | Test strategy and quality plan | `docs/` |
| **PROJECT_SCOPE.md** | Scope definition, boundaries | `docs/` |

### Tier 3: Required for Tools (sirsi-anubis, sirsi-thoth)

| Document | Purpose | Location |
|:---------|:--------|:---------|
| **SAFETY_DESIGN.md** | Safety constraints for destructive operations | `docs/` |
| **BUILD_LOG.md** | Sprint chronicle for build-in-public | `docs/` |

---

## 3. Pantheon Tool Requirements

> Every repo MUST use these tools. The Pantheon is the governance layer.

### 𓁟 Thoth (Knowledge)
- **Files**: `.thoth/memory.yaml`, `.thoth/journal.md`
- **Rule**: Read memory FIRST in every session. Update memory at session end.
- **Efficiency target**: 95%+ context savings (memory replaces source reading)
- **Supersedes**: Manual "read these files first" instructions

### 🪶 Ma'at (Quality)
- **Governs**: Canon linkage, coverage, pipeline health
- **Rule**: Every feature links to canon. Ma'at assessments guide quality.
- **Supersedes**: Manual "check that all tests pass" instructions
- **Note**: Ma'at currently runs as `anubis maat` in sirsi-anubis. For other repos, quality governance is applied through the rules (§1.24) until Ma'at becomes a standalone tool.

### Pre-Push Gate
- **File**: `.githooks/pre-push`
- **Rule**: Every push is gated by linting/formatting checks
- **Go repos**: `gofmt + go vet + golangci-lint + go build`
- **Node repos**: `eslint + prettier --check + build`
- **Setup**: `git config core.hooksPath .githooks`

### Session Workflows
- **File**: `.agent/workflows/session-start.md`
- **Rule**: Every AI session starts by reading Thoth memory
- **Supersedes**: Manual instructions to "read file X, then file Y"

---

## 4. Repo-Specific Rule Categories

> These categories ONLY appear in the repo's §2 (repo-specific rules).

| Category | Example Repos | Examples |
|:---------|:-------------|:---------|
| **Design system** | FinalWishes, Assiduous | Color palettes, typography, component rules |
| **Legal/compliance** | FinalWishes | Legal document fidelity, PII siloing |
| **Safety protocols** | sirsi-anubis | Dry-run requirements, protected paths |
| **Domain logic** | Each repo | Business rules specific to the problem domain |
| **Stack rules** | Each repo | Framework-specific conventions |

---

## 5. Canon Document Gap Matrix

> This matrix shows which Tier 1 docs exist in each repo.

| Document | sirsi-anubis | SirsiNexusApp | FinalWishes | assiduous | sirsi-thoth |
|:---------|:------------|:--------------|:------------|:----------|:------------|
| Rules file | ✅ ANUBIS_RULES | ✅ SIRSI_RULES | ✅ GEMINI.md | ❌ | ❌ |
| GEMINI.md | ✅ | ✅ | ✅ | ❌ | ❌ |
| CLAUDE.md | ✅ | ✅ | ✅ | ❌ | ❌ |
| README.md | ✅ | ✅ | ✅ | ✅ | ✅ |
| SECURITY.md | ✅ | ✅ | ❌ | ❌ | ❌ |
| CONTRIBUTING.md | ✅ | ✅ | ❌ | ❌ | ❌ |
| CHANGELOG.md | ✅ | ❌ | ✅ | ❌ | ❌ |
| VERSION | ❌ | ❌ | ❌ | ❌ | ❌ |
| ARCHITECTURE_DESIGN | ✅ | ✅ | ✅ | ❌ | ❌ |
| ADR-INDEX | ✅ | ✅ | ✅ | ❌ | ❌ |
| ADR-TEMPLATE | ✅ | ✅ | ✅ | ❌ | ❌ |
| CONTINUATION-PROMPT | ✅ | ✅ | ✅ | ❌ | ❌ |
| .thoth/memory.yaml | ✅ full | ⚠️ skeleton | ⚠️ skeleton | ⚠️ skeleton | ⚠️ minimal |
| .thoth/journal.md | ✅ | ❌ | ❌ | ❌ | ❌ |
| .agent/workflows | ✅ | ✅ | ✅ | ✅ | ❌ |
| Pre-push hook | ✅ | ❌ | ❌ | ❌ | ❌ |

---

## 6. Versioning This Standard

- **Location**: This file lives in every repo as `docs/SIRSI_PORTFOLIO_STANDARD.md`
- **Source of truth**: sirsi-anubis (where it was first codified)
- **Updates**: When a rule is added or modified, ALL repos must be updated
- **Ma'at enforcement**: In the future, Ma'at will verify that portfolio standards are consistent across repos

---

**This standard was codified in Session 9 (March 23, 2026) to ensure that the Pantheon (Thoth, Ma'at, and future deity agents) governs every Sirsi repository uniformly.**
