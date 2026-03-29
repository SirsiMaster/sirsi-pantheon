# 𓁟 thoth-init

**Persistent knowledge for AI-assisted development.** Give your AI coding assistant structured memory across sessions.

## Quick Start

```bash
npx thoth-init
```

That's it. Thoth will:
1. **Scan** your project (language, framework, stats)
2. **Create** `.thoth/memory.yaml` — compressed project state (~100 lines)
3. **Create** `.thoth/journal.md` — timestamped decision log
4. **Inject** rules into all supported IDEs so the AI reads Thoth first

## What It Solves

Every time you start a new AI session, your coding assistant re-reads thousands of lines of source code, losing 10-30 minutes of context-building. Thoth compresses your project's identity, architecture, decisions, and limitations into ~100 lines that the AI reads *first*.

**Measured impact:** ~98.7% reduction in context tokens needed to start working.

## Supported IDEs

- **Claude Code / Antigravity** — `CLAUDE.md` + `.agent/workflows/session-start.md`
- **Cursor** — `.cursorrules`
- **Windsurf** — `.windsurfrules`
- **Gemini** — `.gemini/style.md`
- **GitHub Copilot** — `.github/copilot-instructions.md`

## Non-Interactive Mode

```bash
npx thoth-init -y
```

## How It Works

```
Source Code (30,000+ lines)
        ↓ compressed by Thoth
.thoth/memory.yaml (~100 lines)
        ↓ read first by AI
AI has context in ~2 seconds instead of ~10 minutes
```

### Three Layers

| Layer | File | Purpose |
|:------|:-----|:--------|
| **L0 Memory** | `memory.yaml` | Identity, architecture, stats, decisions |
| **L1 Journal** | `journal.md` | Timestamped reasoning — the WHY |
| **L2 Artifacts** | `artifacts/` | Deep analyses, benchmarks, audits |

## Part of Pantheon

Thoth is a deity in the [Sirsi Pantheon](https://github.com/SirsiMaster/sirsi-pantheon) — a unified DevOps intelligence platform. While `thoth-init` works standalone in any project, the full Pantheon CLI adds auto-sync, governance, and more.

```bash
# Full Pantheon (includes Thoth + 12 more deities)
brew tap SirsiMaster/tools && brew install sirsi-pantheon

# Auto-sync memory from source code
pantheon thoth sync
```

## License

MIT — [Sirsi Technologies](https://sirsi.ai)
