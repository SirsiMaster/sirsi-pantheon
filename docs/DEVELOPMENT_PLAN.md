# 𓂀 Sirsi Anubis — Canonical Development Plan
**Version:** 2.0.0
**Date:** March 21, 2026
**Status:** LOCKED — Ship Week (March 21–28, 2026)

> **DEADLINE: Friday March 28, 2026.**
> Every feature ships this week. April investor demos require a complete,
> polished, demoable product. No feature deferred. No excuses.

---

## Product Tiers

| Tier | Codename | License | Distribution | Scope |
|:-----|:---------|:--------|:-------------|:------|
| **Anubis Free** | Open Source | MIT | Homebrew, GitHub Releases, `go install` | Single workstation — scan, clean, ghost hunt, RAM guard |
| **Anubis Pro** | Neural Edition | Freemium | `anubis install-brain` add-on | Semantic search, dedup, neural context sanitization |
| **Eye of Horus** | Subnet Edition | Licensed | Upgrade from Free/Pro | Local subnet scanning — VLAN sweep, LAN agents |
| **Ra** | Enterprise | Sirsi-only | Bundled with Sirsi Platform | Fleet-scale — multi-site, SAN/NAS, policy, dashboard |

### Tier Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       RA (Enterprise)                        │
│    Fleet policies, multi-site, SAN/NAS, Sirsi dashboard     │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │                 EYE OF HORUS (Subnet)                    │ │
│ │    VLAN sweep, LAN agents, local fleet orchestration     │ │
│ │ ┌──────────────────────────────────────────────────────┐ │ │
│ │ │               ANUBIS PRO (Neural)                    │ │ │
│ │ │    install-brain, semantic search, dedup, ANE/CUDA   │ │ │
│ │ │ ┌──────────────────────────────────────────────────┐ │ │ │
│ │ │ │            ANUBIS FREE (Open Source)              │ │ │ │
│ │ │ │  weigh, judge, ka, guard, sight, hapi, profiles  │ │ │ │
│ │ │ │  64+ rules, book-of-the-dead, JSON, stealth      │ │ │ │
│ │ │ └──────────────────────────────────────────────────┘ │ │ │
│ │ └──────────────────────────────────────────────────────┘ │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

---

## Platform Matrix

### Operating Systems

| OS | Binary | Status |
|:---|:-------|:-------|
| macOS (arm64) | `anubis-darwin-arm64` | ✅ Primary — shipping |
| macOS (amd64) | `anubis-darwin-amd64` | ✅ CI cross-compile |
| Linux (amd64) | `anubis-linux-amd64` | ✅ goreleaser builds |
| Linux (arm64) | `anubis-linux-arm64` | ✅ goreleaser builds |
| Windows (amd64) | `anubis-windows-amd64.exe` | 📋 Post-launch |
| Windows (arm64) | `anubis-windows-arm64.exe` | 📋 Post-launch |

### GPU / Accelerator Detection

| Accelerator | Detection | Module | Day |
|:------------|:----------|:-------|:----|
| Apple Metal / MLX | `system_profiler`, Metal API | `hapi/metal.go` | Wed 3/26 |
| NVIDIA CUDA | `nvidia-smi`, NVML | `hapi/cuda.go` | Wed 3/26 |
| AMD ROCm | `rocm-smi` | `hapi/rocm.go` | Wed 3/26 |
| Intel oneAPI | `xpu-smi` | `hapi/intel.go` | Wed 3/26 |

### AI Framework Detection — All ✅ Done

PyTorch, TensorFlow, HuggingFace, Ollama, MLX, ONNX, vLLM, JAX,
Stable Diffusion, LangChain — all 10 frameworks have scan rules shipped.

### IDE Detection — All ✅ Done

VS Code, JetBrains, Xcode, Android Studio, Claude Code, Gemini CLI,
Cursor, Windsurf, Zed, Neovim, Codex — all 11 tools have scan rules shipped.

---

## Ship Week Schedule

### Day 1: Friday March 21 ✅ DONE
**Sprint 1.0–1.7 Complete**

- [x] Foundation — CLI, engine, safety, 12 rules
- [x] Ka Ghost Hunter — 22 rules, Launch Services scanning
- [x] CI + Quality — 100+ tests, ADRs, docs, portfolio CI fix
- [x] Guard Module — RAM audit, process slayer, orphan detection
- [x] Scan Rule Expansion — 34 → 64 rules across all 7 categories
- [x] Sight Module — Launch Services rebuild, Spotlight reindex
- [x] Profiles + Config — 4 profiles, YAML config, ~/.config/anubis/
- [x] Distribution — goreleaser, SCAN_RULE_GUIDE, binary polish (4.1 MB)

**Status: 7 commands working, 64 rules, 100+ tests, CI green**

---

### Day 2: Saturday March 22
**Book of the Dead + Stealth + Initiate**

- [ ] `anubis book-of-the-dead` — hidden system autopsy command
  - Deep system report: disk, RAM, GPU, ghosts, processes, network
  - Styled terminal output (papyrus/hieroglyphic ASCII theme)
  - Hidden from `--help` (Cobra `Hidden: true`)
  - Upsell footer: "To perform this ritual across 100+ nodes, connect to Sirsi"
  - `--verbose` for expanded detail
  - `--json` for structured data export

- [ ] `anubis initiate` — batch macOS permission grant
  - Request Full Disk Access, Accessibility, Network
  - Guide user through System Preferences panels
  - Verify permissions after granting
  - "Ritual Initiation" branding

- [ ] `--stealth` / `--clean-exit` flag on all commands
  - Wipe `~/.config/anubis/cache/` after scan
  - Delete downloaded brain weights
  - Zero footprint mode: "Anubis comes, judges, and vanishes"

- [ ] `.anubisignore` support
  - Exclude paths from scanning (like .gitignore syntax)
  - Pre-indexing hook for AI editors
  - Default `.anubisignore` template

---

### Day 3: Sunday March 23
**Hapi Resource Optimizer**

- [ ] `internal/hapi/detect.go` — hardware detection engine
  - Apple Silicon: Neural Engine, Metal GPU, unified memory
  - NVIDIA: CUDA cores, VRAM, driver version via nvidia-smi
  - AMD: ROCm, VRAM via rocm-smi
  - Intel: integrated GPU, oneAPI
  - CPU-only fallback

- [ ] `internal/hapi/vram.go` — GPU/VRAM audit
  - Metal memory pressure (macOS)
  - CUDA VRAM usage per process (Linux/Windows)
  - Fragmentation detection

- [ ] `internal/hapi/dedup.go` — duplicate file detection
  - SHA-256 hash-based comparison
  - Size-first filter (skip if size differs)
  - Parallel hashing via goroutines
  - Report only (no auto-delete)

- [ ] `internal/hapi/snapshots.go` — APFS snapshot pruning
  - List local Time Machine snapshots
  - Calculate space used
  - Safe pruning with confirmation

- [ ] `cmd/anubis/hapi.go` — CLI command
  - `anubis hapi` — full resource audit
  - `anubis hapi --gpu` — GPU/VRAM focus
  - `anubis hapi --dedup` — find duplicate files
  - `anubis hapi --snapshots` — APFS snapshot management

---

### Day 4: Monday March 24
**Scarab Scout + Eye of Horus Foundation**

- [ ] `internal/scarab/discovery.go` — network host discovery
  - ARP table parsing
  - Subnet ping sweep (ICMP)
  - mDNS/Bonjour service discovery (macOS)
  - Port scanning (SSH, Docker API)

- [ ] `internal/scarab/sweep.go` — parallel fleet scanning
  - Fan-out scan across discovered hosts
  - Concurrent SSH connections
  - Result aggregation

- [ ] `internal/scarab/container.go` — container scanning
  - Docker socket detection
  - Container listing + size audit
  - Dangling images, stopped containers, unused volumes

- [ ] `cmd/anubis/scarab.go` — CLI command
  - `anubis scarab discover` — find hosts on network
  - `anubis scarab sweep` — scan all discovered hosts
  - `anubis scarab containers` — Docker/K8s audit
  - `--confirm-network` safety flag (Rule A4)

---

### Day 5: Tuesday March 25
**Neural Brain + Pro Architecture**

- [ ] `internal/brain/downloader.go` — on-demand model fetcher
  - Download CoreML/ONNX model to `~/.anubis/weights/`
  - Progress bar display
  - Checksum verification
  - Version management (update/remove)

- [ ] `internal/brain/inference.go` — model inference wrapper
  - ONNX Runtime Go bindings (ort-go)
  - CoreML bridge via CGO (macOS)
  - CPU fallback for cross-platform
  - Batch inference for file classification

- [ ] `anubis install-brain` — download neural weights
  - `anubis install-brain` — install default model
  - `anubis install-brain --remove` — self-delete weights
  - `anubis install-brain --update` — fetch latest
  - Size budget: < 100 MB for quantized model

- [ ] `anubis uninstall-brain` — clean removal

---

### Day 6: Wednesday March 26
**IDE Integrations + MCP Server**

- [ ] `internal/mcp/server.go` — MCP (Model Context Protocol) server
  - Anubis as context sanitizer for Claude/Cursor/Windsurf
  - Tools: `scan_workspace`, `clean_workspace`, `ghost_report`
  - Resources: scan results, ghost list, health status
  - Runs as local stdio server

- [ ] `anubis mcp` — start MCP server mode
  - Integrates with Claude Code, Cursor, Windsurf
  - Pre-scan workspace before AI indexing
  - "I'm running Anubis to weigh the heart of this directory..."

- [ ] VS Code Extension scaffold
  - `extensions/vscode/` — extension manifest
  - "Eye of Horus" sidebar health meter concept
  - Status bar icon (red/green based on workspace health)
  - Command palette integration

- [ ] `.anubis/` workspace config
  - Per-project Anubis configuration
  - Custom scan rules for specific repos
  - Integration with `.anubisignore`

---

### Day 7: Thursday March 27
**Scales Policy Engine + Agent Hardening**

- [ ] `internal/scales/policy.go` — YAML policy parser
  - Policy definitions for scan thresholds
  - Auto-remediation rules (with approval)
  - Notification targets (Slack, Teams, webhook)

- [ ] `internal/scales/enforce.go` — policy enforcement
  - Evaluate scan results against policies
  - Generate verdicts (pass/warn/fail)
  - Recommended actions

- [ ] `cmd/anubis/scales.go` — CLI command
  - `anubis scales enforce` — run policies
  - `anubis scales validate` — check policy syntax
  - `anubis scales verdicts` — show results

- [ ] Agent hardening
  - `cmd/anubis-agent/` — implement scan/report/clean
  - Fixed command set (no shell access)
  - JSON stdout for controller communication
  - Self-update mechanism stub

---

### Day 8: Friday March 28
**Polish, README, Release Prep**

- [ ] Update README.md — accurate feature list, all commands documented
- [ ] Update CHANGELOG.md — v0.2.0-alpha complete entry
- [ ] Update VERSION file
- [ ] Final test suite run (target: 70%+ coverage on core)
- [ ] Binary size audit (controller < 15 MB, agent < 5 MB)
- [ ] gofmt + go vet + golangci-lint clean
- [ ] Tag v0.2.0-alpha
- [ ] goreleaser snapshot build (verify all platforms)
- [ ] GitHub Release draft
- [ ] Product Hunt / Hacker News launch copy draft
- [ ] Investor demo script (5-minute walkthrough)
- [ ] Continuation prompt for next sprint cycle

---

## Binary Size Budget

| Binary | Current | Target | Strategy |
|:-------|:--------|:-------|:---------|
| `anubis` | 4.1 MB | < 15 MB | All modules, no brain weights |
| `anubis-agent` | 1.6 MB | < 5 MB | Scan-only, no UI, `CGO_ENABLED=0` |
| Brain weights | 0 MB | < 100 MB | On-demand download, self-deletable |

---

## Scan Rule Count

| Category | Shipped | Notes |
|:---------|:--------|:------|
| General Mac | 9 | caches, logs, crash, downloads, trash, browser, TM, mail, fonts |
| Virtualization | 4 | Parallels, VMware, UTM, VirtualBox |
| Dev Frameworks | 10 | node, go, python, rust, docker, npm, gradle, maven, composer, ruby |
| AI/ML | 11 | MLX, Metal, HF, Ollama, PyTorch, TF, ONNX, vLLM, JAX, SD, LangChain |
| IDEs & AI Tools | 11 | Xcode, VS Code, JetBrains, Android, Claude, Gemini, Cursor, Windsurf, Zed, Neovim, Codex |
| Cloud & Infra | 6 | K8s, Terraform, gcloud, Firebase, nginx, AWS |
| Cloud Storage | 4 | OneDrive, Google Drive, Dropbox, iCloud |
| **Total** | **55 registered + sub-rules = 64+ effective** | |

---

## CLI Command Map (Ship Week)

| Command | Day | Status |
|:--------|:----|:-------|
| `anubis version` | Day 1 | ✅ Shipped |
| `anubis weigh` | Day 1 | ✅ Shipped |
| `anubis judge` | Day 1 | ✅ Shipped |
| `anubis ka` | Day 1 | ✅ Shipped |
| `anubis guard` | Day 1 | ✅ Shipped |
| `anubis sight` | Day 1 | ✅ Shipped |
| `anubis profile` | Day 1 | ✅ Shipped |
| `anubis book-of-the-dead` | Day 2 | 📋 Saturday |
| `anubis initiate` | Day 2 | 📋 Saturday |
| `anubis hapi` | Day 3 | 📋 Sunday |
| `anubis scarab` | Day 4 | 📋 Monday |
| `anubis install-brain` | Day 5 | 📋 Tuesday |
| `anubis mcp` | Day 6 | 📋 Wednesday |
| `anubis scales` | Day 7 | 📋 Thursday |
| **Release v0.2.0-alpha** | Day 8 | 📋 Friday |

---

## Integration Map

| Integration | Method | Day |
|:------------|:-------|:----|
| Claude Code | MCP Server (stdio) | Day 6 |
| Cursor | MCP Server (stdio) | Day 6 |
| Windsurf | MCP Server (stdio) | Day 6 |
| VS Code | Extension scaffold | Day 6 |
| Codex / Antigravity | Pre-flight hook via JSON | Day 6 |
| Sirsi Nexus | Ra API (future) | Post-launch |

---

## Investor Demo Script (5 min)

1. `anubis version` — branding moment (10s)
2. `anubis weigh` — live scan, show 69 GB found (30s)
3. `anubis guard` — show RAM audit, orphan processes (20s)
4. `anubis ka` — ghost app hunt (20s)
5. `anubis sight` — Launch Services ghost scan (15s)
6. `anubis hapi --gpu` — GPU/hardware detection (15s)
7. `anubis book-of-the-dead` — full system autopsy (30s)
8. `anubis judge --dry-run` — show what would be cleaned (20s)
9. `anubis scarab discover` — show network hosts (20s)
10. `anubis profile list` — show developer profiles (10s)
11. Close: "From workstation to enterprise. Anubis → Eye of Horus → Ra" (30s)

---

## Decision Log

| Decision | Rationale | Ref |
|:---------|:----------|:----|
| Go 1.22+ | Static binary, cross-platform, contributor-friendly | ADR-001 |
| Agent-controller model | Fleet scalability without SSH key sprawl | ADR-001 |
| MIT open source | Community adoption, Anubis is preview/marketing for Sirsi | ADR-001 |
| Ka ghost detection | No competitor does cross-referenced ghost hunting | ADR-002 |
| 4-tier licensing | Free → Pro → Eye of Horus → Ra growth path | v2.0.0 |
| On-demand brain | Keep base binary < 15 MB, neural weights downloaded separately | v2.0.0 |
| MCP integration | Position Anubis as "Context Sanitizer for AI era" | v2.0.0 |
| Ship week deadline | April investor demos require complete product | v2.0.0 |
| Book of the Dead | Hidden command differentiator, memorable demo moment | v2.0.0 |
| Ephemeral mode | "Zero footprint" narrative — Anubis comes, judges, vanishes | v2.0.0 |

---

> **This plan ships by Friday March 28, 2026.**
> One command per day. No deferrals. CI green at every commit.
> April investors see a complete, polished, demoable product.
