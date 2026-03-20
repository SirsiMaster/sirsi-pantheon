# 𓂀 Sirsi Anubis

**The Guardian of Infrastructure Hygiene**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-C8A951?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.1.0--alpha-1A1A5E?style=flat)](VERSION)

> *"Weigh. Judge. Purge."*

Sirsi Anubis is a free, open-source infrastructure hygiene platform. It scans, judges, and purges waste across workstations, containers, VMs, networks, and storage backends.

**No cleaning tool understands what developers and AI engineers leave behind.** Anubis does — with 60+ scan rules across 7 domains, from stale `node_modules` to orphaned CUDA training processes to ghost apps polluting your Spotlight.

---

## 🐺 Why Anubis?

Every existing cleaning tool — commercial or open source — treats your machine as a **consumer device**. But if you're a developer or AI engineer, your machine is a **workstation**. It accumulates a completely different class of junk:

| What Existing Tools Miss | What Anubis Finds |
|-------------------------|------------------|
| Virtualization remnants (Parallels, VMware) | ✅ 91 ghost apps, 12+ subsystem directories, package receipts |
| AI/ML model caches (HuggingFace, Ollama, MLX) | ✅ 5-200 GB of stale model weights |
| IDE workspace sprawl (LSP servers, stale sessions) | ✅ 17+ GB of zombie language servers |
| Orphaned developer processes (Node, Docker) | ✅ 38 zombie processes eating 5.7 GB RAM |
| GPU/VRAM fragmentation | ✅ Metal/MLX unified memory, CUDA VRAM optimization |
| Fleet-wide infrastructure waste | ✅ Sweep VLANs, subnets, containers, SANs |

---

## ⚡ Quick Start

### Install via Homebrew
```bash
brew tap SirsiMaster/tools
brew install sirsi-anubis
```

### Install from Source
```bash
go install github.com/SirsiMaster/sirsi-anubis/cmd/anubis@latest
```

### Scan Your Workstation
```bash
# Weigh everything — see what Anubis finds
anubis weigh

# Weigh specific domains
anubis weigh --dev        # Developer frameworks (Node, Rust, Go, Python)
anubis weigh --ai         # AI/ML caches (MLX, CUDA, HuggingFace, Ollama)
anubis weigh --vms        # Virtualization (Parallels, Docker, VMware)
anubis weigh --ides       # IDEs (Xcode, VS Code, Claude Code, Gemini CLI)

# Judge — clean with dry-run first
anubis judge --dry-run
anubis judge --confirm

# Guard — manage RAM pressure
anubis guard                    # Audit RAM usage
anubis guard --slay node        # Kill zombie Node processes
anubis guard --slay lsp         # Kill stale language servers

# Sight — fix ghost apps in Spotlight
anubis sight
anubis sight --fix
```

---

## 🏛 Architecture

Anubis is built on four modules, each named after Egyptian mythology:

| Module | Codename | Role |
|--------|----------|------|
| 🐺 **Jackal** | Local Scanner | Patrols and cleans individual machines |
| 🪲 **Scarab** | Fleet Sweep | Rolls across VLANs, subnets, and domains |
| ⚖️ **Scales** | Policy Engine | Weighs findings against defined policies |
| 🌊 **Hapi** | Resource Optimizer | Controls VRAM, GPU memory, and storage flow |

### Two Binaries

| Binary | Size | Purpose |
|--------|------|---------|
| `anubis` | ~20 MB | Full CLI — scan, clean, manage fleet, generate reports |
| `anubis-agent` | <10 MB | Lightweight agent — deployed to VMs, containers, remote hosts |

---

## 📦 Scan Domains (60+ Rules)

| Domain | Examples |
|--------|----------|
| 🖥️ **General Mac** | Caches, logs, crash reports, browser junk, downloads |
| 🐳 **Virtualization** | Parallels, Docker, VMware, UTM, VirtualBox |
| 📦 **Dev Frameworks** | Node/npm, Next.js, TanStack, Rust/Cargo, Go, Python/conda, Java/Gradle |
| 🤖 **AI/ML** | Apple MLX, Metal, NVIDIA CUDA, HuggingFace, Ollama, PyTorch |
| 🛠️ **IDEs & AI Tools** | Xcode, VS Code, JetBrains, Claude Code, Codex, Gemini CLI, Android Studio |
| ☁️ **Cloud/Infra** | Docker, Kubernetes, nginx, Terraform, gcloud, Firebase |
| 📱 **Cloud Storage** | OneDrive, Google Drive/Workspace, iCloud, Dropbox |

---

## 🗺️ Roadmap

| Phase | Codename | Status |
|-------|----------|--------|
| 1 | **Jackal** — Local CLI scanner + cleaner | 🔨 In Progress |
| 2 | **Jackal+** — Container/VM scanning, AI/ML rules | 📋 Planned |
| 3 | **Hapi** — VRAM management, storage optimization | 📋 Planned |
| 4 | **Scarab** — Agent-controller, fleet sweep | 📋 Planned |
| 5 | **Scarab+** — SAN/NAS/S3 scanning | 📋 Planned |
| 6 | **Scales** — Policy engine, fleet enforcement | 📋 Planned |
| 7 | **Temple** — Web dashboard / SwiftUI GUI | 📋 Future |

---

## 🤝 Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Adding scan rules is easy** — create a Go file implementing the `ScanRule` interface in `internal/jackal/rules/`. See `docs/SCAN_RULE_GUIDE.md` for details.

---

## 📄 License

MIT License — free forever. See [LICENSE](LICENSE).

---

## 🏢 Part of the Sirsi Nexus Platform

Sirsi Anubis is a sub-component of the [Sirsi Nexus](https://github.com/SirsiMaster/SirsiNexusApp) platform.

| Product | Role |
|---------|------|
| **Sirsi Nexus** | AI infrastructure platform |
| **Sirsi Anubis** | Infrastructure hygiene |
| **Sirsi Rook** *(reserved)* | Database & storage orchestration |
| **Sirsi Rogue** *(reserved)* | Cybersecurity sweeper |

---

*𓂀 The jackal sees everything. Nothing escapes the Weighing.*
