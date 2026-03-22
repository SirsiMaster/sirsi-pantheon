# 𓁟 Thoth — Persistent Knowledge System for AI-Assisted Development

> *"Thoth invents writing so that knowledge survives the death of the scribe."*

[![License: MIT](https://img.shields.io/badge/License-MIT-C8A951?style=flat)](LICENSE)

## The Problem

Large Language Models have **no persistent memory**. Every conversation starts blank. When an AI assistant works on a codebase, it must re-read thousands of lines of source code to rebuild context — wasting tokens, time, and money. Worse, the *reasoning* behind decisions is lost between sessions.

This creates a cycle of re-discovery. The AI makes the same mistakes, asks the same questions, and rediscovers the same architecture every time it touches the project.

## The Solution

Thoth is a **three-layer knowledge system** that gives AI assistants persistent, structured memory across sessions:

| Layer | File | Purpose | Read When |
|:------|:-----|:--------|:----------|
| **𓁟 Memory** | `.thoth/memory.yaml` | Compressed project state (~100 lines) | Every session, first |
| **𓁟 Journal** | `.thoth/journal.md` | Timestamped reasoning and insights | When WHY matters |
| **𓁟 Artifacts** | `.thoth/artifacts/` | Deep analysis, audits, benchmarks | On-demand by topic |

### Why Three Layers?

```
Layer 1: Memory (WHAT)
  ├─ Project identity, version, stats
  ├─ Architecture quick reference
  ├─ Critical design decisions
  ├─ Known limitations
  └─ File map of important paths
  → ~100 lines. Replaces reading 10,000+ lines of source.

Layer 2: Journal (WHY)
  ├─ Timestamped decision entries
  ├─ Context that triggered each decision
  ├─ Alternatives considered
  ├─ Results and outcomes
  └─ Meta-observations and patterns
  → Running commentary. Institutional knowledge.

Layer 3: Artifacts (DEEP)
  ├─ Benchmark results
  ├─ Platform audits
  ├─ Security reviews
  ├─ Design documents
  └─ Historical walkthroughs
  → Reference material. Read when diving deep.
```

### Measured Impact

Benchmarked across 4 real production codebases (428,000+ lines of code):

| Project | Source Lines | Thoth Lines | Token Savings |
|:--------|:------------|:------------|:-------------|
| sirsi-anubis (Go) | 14,589 | 85 | **99.4%** |
| assiduous (React) | 160,897 | 100* | **99.9%** |
| FinalWishes (Next.js) | 96,514 | 100* | **99.9%** |
| SirsiNexusApp (TS) | 155,817 | 100* | **99.9%** |
| **Total** | **427,817** | **~385** | **99.9%** |

*\* Starter memory — grows to ~100 lines after first session.*

**Practical note**: AI assistants don't read ALL source lines in a session — they typically read 2,000–5,000 lines initially. Even against this practical baseline, Thoth saves **~98% of initial context loading** (100 lines vs 5,000 lines).

**In dollar terms** (at ~$3/million tokens for Claude Sonnet):
- Without Thoth: ~50,000 tokens per session start = $0.15/session
- With Thoth: ~1,000 tokens = $0.003/session
- **50x cost reduction** on initial context loading

## Quick Start

### For any project — add Thoth in 30 seconds:

```bash
mkdir -p .thoth/artifacts
touch .thoth/memory.yaml
touch .thoth/journal.md
```

Then populate `memory.yaml` with your project's identity:

```yaml
# Project Memory — read this FIRST before any source files
project: my-project
version: 0.1.0
language: TypeScript
# Add architecture, decisions, limitations, file map...
```

### For Sirsi ecosystem projects:

```bash
# Uses the canonical Sirsi Thoth template
cp -r /path/to/sirsi-anubis/.thoth-template .thoth
```

### For AI workflows:

Add to your `.agent/workflows/session-start.md`:
```markdown
## Step 1: Read Thoth memory (ALWAYS)
Read .thoth/memory.yaml first. This gives you full project context.

## Step 2: Read journal (when reasoning matters)
Read .thoth/journal.md for decision history.
```

## Integration with Anubis MCP

When running as an MCP server, Anubis exposes Thoth as a tool:

```json
{
  "name": "thoth_read_memory",
  "description": "Read the project's Thoth memory file for instant context"
}
```

AI IDEs (Claude Code, Cursor, Windsurf) can call this tool at conversation start to automatically load project context.

## Availability & Distribution

### Available now:
- **As files**: Copy `.thoth/` directory into any project. Zero dependencies. Works with any AI assistant.
- **Inside Anubis**: `thoth_read_memory` MCP tool available via `anubis mcp`.
- **As an AI skill**: Installed in Claude Code / Antigravity skill directories. Auto-detected.
- **As a workflow**: `.agent/workflows/session-start.md` template included.

### Planned distribution:
- **VS Code extension**: `Thoth Memory` — sidebar panel showing memory + journal, auto-prompting AI to read on session start
- **npm package**: `npx thoth-init` — scaffolds `.thoth/` in any project
- **Claude Code MCP**: Standalone MCP server (no Anubis required) that serves Thoth memory
- **Cursor / Windsurf plugin**: Rules file injection that instructs the AI to read `.thoth/memory.yaml`
- **GitHub Action**: Auto-validates that Thoth memory is updated when source changes

### Independent of Anubis:
Thoth works without Anubis. It's just files — YAML and Markdown. Any AI assistant that can read files can use Thoth. The Anubis MCP integration is a convenience layer, not a dependency.

## Philosophy

1. **Memory is cheaper than re-discovery.** 100 lines of structured YAML saves reading 10,000+ lines of source.
2. **Reasoning outlives the decision.** The Journal captures WHY, not just WHAT. Future sessions (human or AI) benefit from understanding the thought process.
3. **Depth on demand.** Not every session needs benchmark data or platform audits. Artifacts exist for when you need to go deep.
4. **The AI is the scribe.** After every significant change, the AI updates Memory and writes a Journal entry. The knowledge base grows automatically.
5. **Human-readable, AI-optimized.** YAML and Markdown are readable by both humans and LLMs. No proprietary formats.
6. **Zero dependencies.** No CLI required. No server required. Just files in your repo.

## License

MIT License — free and open source. Use Thoth in any project, commercial or otherwise.

## Origin

Thoth was developed during the Sirsi Anubis project (March 2026) when we discovered that AI assistants were spending ~80% of their context window re-reading unchanged source files. The three-layer system emerged from a conversation about LLM memory limitations and was immediately validated by cutting session startup context by 98%+ across 4 production codebases totaling 428,000 lines of code.

Named after the Egyptian god of knowledge, writing, and wisdom — the keeper of all records and inventor of hieroglyphics.

---

*𓁟 Knowledge that survives the death of the scribe.*

