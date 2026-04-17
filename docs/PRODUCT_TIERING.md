# 𓉴 Pantheon Product Tiering & Harsh Re-Evaluation
**Date:** March 30, 2026  
**Version:** v0.8.0-beta (honest)  
**Assessment:** Post-remediation  

---

## 🛑 Harsh Re-Evaluation: What Actually Works

I am evaluating every feature by one criterion: **does it produce value for a user right now, without crashes, without facade output, and without requiring a developer to understand the internals?**

### Features That Actually Work (Verified via Smoke Test)

| Feature | Command | What It Does | Verified |
|:---|:---|:---|:---|
| **Filesystem waste scan** | `anubis weigh` | Scans 60+ locations for caches, logs, temp files. Reports real sizes. | ✅ Found 10.2 GB |
| **Safe cleanup (dry-run)** | `anubis judge --dry-run` | Identifies deletable artifacts, respects protected paths, shows what would be freed | ✅ 114 artifacts |
| **Safe cleanup (execute)** | `anubis judge --confirm` | Actually moves waste to Trash. Safety module blocks protected paths. | ✅ Tested |
| **Ghost app detection** | `anubis ka` | Finds orphaned app residuals (LaunchServices, Spotlight phantoms) | ✅ Functional |
| **Duplicate file finder** | `anubis mirror` | Three-phase dedup with partial hashing. 27x faster than naive. | ✅ Functional |
| **RAM pressure monitor** | `anubis guard` | Reports RAM usage 和 pressure level | ✅ Functional |
| **Quality assessment** | `maat audit` | Measures test coverage across all modules (was blind, now fixed) | ✅ 2.9s, honest |
| **Policy enforcement** | `maat scales` | Evaluates workstation against hygiene policy (waste thresholds, ghost counts) | ✅ Functional |
| **Metrics dashboard** | `maat pulse --skip-test` | Source lines, file counts, binary size, deity count — instant | ✅ Functional |

### Features That Are Half-Baked (Exist But Incomplete)

| Feature | Command | Problem |
|:---|:---|:---|
| **Auto-healing** | `maat heal` | Depends on Isis which can detect gaps via AST but cannot auto-generate fixes. The "heal" is really just a "diagnose." |
| **Knowledge sync** | `thoth sync` | Syncs memory.yaml and journal.md from source. Works mechanically but the data it writes is not meaningfully actionable. |
| **Hardware profiling** | `hapi profile` | Detects GPU/accelerator presence. Stable on macOS. Linux/Windows = stub. No actual ML acceleration occurs. |
| **Build log audit** | Neith (internal only) | Parses BUILD_LOG.md for session counts. Useful internally but not a user-facing feature. |

### Features That Are Facades or Stubs

| Feature | Problem |
|:---|:---|
| **Seshat (Gemini Bridge)** | ~182 lines of JSON marshaling. Not a real knowledge extraction engine. |
| **Seba (Architecture Mapping)** | Generates static Mermaid diagrams. Not a live monitoring system. |
| **Osiris (Checkpoint Guardian)** | Git status parser. Works but adds no value over `git status` itself. |
| **MCP Server** | Exists but the tools it exposes are thin wrappers around the same CLI commands. |
| **VS Code Extension** | Memory monitoring and LSP renice. Works in isolation but not integrated with the core product. |

---

## 💰 Product Tiering: What Should Be Free, Pro, and Ra

### Tier 1: Anubis Free (Open Source, $0)
**Value proposition:** "Clean your Mac in 30 seconds. No account required."

This is the **acquisition funnel**. It must be genuinely useful on first run with zero configuration.

| Feature | Justification |
|:---|:---|
| `anubis weigh` (scan) | Core hook. User sees "you have 10GB of waste" and is hooked. |
| `anubis judge --dry-run` (preview) | Shows what would be cleaned. Builds trust. |
| `anubis judge --confirm` (clean, limited) | Clean **3 categories max** per run (e.g., caches, logs, crash reports). Enough to be useful but limited. |
| `anubis guard` (RAM check) | One-shot RAM report. No monitoring. |
| `sirsi version` | Shows version. |

**What's NOT free:**
- Unlimited category cleaning
- Ghost detection (ka)
- Duplicate detection (mirror)
- Continuous monitoring
- Policy enforcement

### Tier 2: Anubis Pro ($9.99/year)
**Value proposition:** "Full workstation hygiene for developers."

$9.99/year is fair because:
- CleanMyMac charges $35/year and does less technical scanning
- The 60+ scan rules genuinely cover developer-specific waste (node_modules, Docker, ML caches, IDE caches)
- The safety module with 35 protected paths is a real differentiator

| Feature | Justification |
|:---|:---|
| `anubis judge` (unlimited categories) | Full cleanup across all 7 categories |
| `anubis ka` (ghost hunter) | Real value — finds orphaned app residuals most users don't know exist |
| `anubis mirror` (dedup) | Revenue feature. 27x optimization is genuinely impressive. |
| `anubis guard --watch` (continuous monitoring) | Background RAM monitoring with alerts |
| `maat scales` (policy enforcement) | Tells developers when their workstation is drifting |
| `maat audit` (quality check) | Coverage assessment for their own projects |
| Scan profiles (aggressive, cautious, AI-focused) | Pre-configured scan policies |

### Tier 3: Ra / Pantheon Enterprise (Future, $TBD)
**Value proposition:** "Fleet-wide DevOps intelligence across your org."

This tier is **not ready** and should not be marketed until the following are built:

| Feature | Current State | Needed For Ra |
|:---|:---|:---|
| Fleet scanning | Not started | Central dashboard aggregating scans from N machines |
| Cross-platform | macOS only | Linux + Windows must work |
| Webhook/Slack alerts | Policy stubs exist, no transport | Real notification infrastructure |
| Thoth Knowledge Sharing | Single-machine only | Org-wide knowledge persistence |
| Compliance reporting | Not started | SOC 2/HIPAA waste audit trails |
| Hapi ML Orchestration | ANE detection only | Actual tri-silicon mesh scheduling |

---

## 📊 Honest Pricing Rationale

| Competitor | Price | Scope |
|:---|:---|:---|
| CleanMyMac X | $34.95/yr | General Mac cleaning |
| DaisyDisk | $9.99 (one-time) | Disk visualization only |
| DevCleaner for Xcode | Free | Xcode-only |
| Docker Desktop | Free tier | Container management only |

**Anubis Pro at $9.99/yr** is positioned as the **developer-specific** alternative to CleanMyMac, covering territory that no competitor touches (ML model caches, IDE caches across 7 IDEs, Kubernetes cache, Terraform state, ghost app detection). The price is accessible enough for individual developers while leaving headroom for the Ra enterprise tier.

---

## ⚖️ Final Honest State

| Metric | Value |
|:---|:---|
| **Honest version** | v0.8.0-beta |
| **Features that work** | 9 |
| **Features half-baked** | 4 |
| **Features that are stubs** | 5 |
| **Ma'at quality score** | 63/100 (warning) |
| **Smoke test** | 5/5 passing |
| **Cross-platform** | macOS only (honest) |
| **Ready for users?** | Free tier: YES. Pro tier: Almost. Ra: No. |

> [!IMPORTANT]
> The Free tier can ship **today**. It scans, it reports, it cleans safely. That's a real product.
> Pro requires gating logic (license check) and the mirror/ka features need 2 more weeks of integration testing.
> Ra is 3-6 months away minimum.
