# 𓁢 Investor Demo Script — 5-Minute Walkthrough

## Target Audience
Angel investors / VCs evaluating Sirsi Technologies for seed round.

## Setup
- Terminal open, maximized
- Clean macOS desktop (dark mode preferred)
- Anubis installed: `go install github.com/SirsiMaster/sirsi-pantheon/cmd/anubis@latest`

---

## Script (5 minutes)

### 0:00 — Hook (30 seconds)

"Let me show you something that every developer on Earth has right now, and doesn't know about."

```bash
anubis weigh
```

*(Wait for scan results to appear — typically shows 5-30 GB of waste)*

"See that? [X GB] of infrastructure waste. Ghost apps, stale model caches, zombie processes. Every existing cleaning tool misses this because they're built for consumers, not developers."

### 0:30 — The Problem (60 seconds)

"The developer workstation is the most expensive computer in any company, and it's running at 60-70% efficiency. Here's why:"

```bash
anubis weigh --ai
```

"AI engineers accumulate 50-200 GB of model caches from HuggingFace, Ollama, and MLX. Nobody cleans them."

```bash
anubis ka
```

"These are ghost apps — remnants of software you deleted months ago. They're still registered in Spotlight, still have LaunchAgents running, still consuming resources."

### 1:30 — The Solution (90 seconds)

"Anubis is the first infrastructure hygiene platform built for developers. 58 scan rules across 7 domains."

```bash
anubis weigh --json | head -20
```

"Everything is machine-readable. You can pipe this into dashboards, CI pipelines, Slack alerts."

```bash
anubis scales enforce
```

"Policy enforcement. Define thresholds — 'fail if waste exceeds 20 GB.' This is how fleet managers keep 1,000 developer workstations clean."

### 2:30 — File Deduplication (30 seconds)

```bash
anubis mirror --gui
```

"Mirror finds duplicate files 27x faster than brute force. Three-phase hashing — group by size, partial hash (first + last 4KB), then full SHA-256 only for survivors. 99% less disk I/O. Every cleanup goes to Trash first with a full audit trail."

### 3:00 — The AI Play (60 seconds)

"Here's the real differentiator. Every AI coding assistant — Claude, Cursor, Windsurf — indexes your workspace before helping you code. They don't know what's junk."

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"demo","version":"1.0"}}}' | anubis mcp --quiet
```

"Anubis ships as an MCP server. Your AI assistant can call `scan_workspace` before indexing. Context sanitizer for the AI era."

### 4:00 — Business Model (45 seconds)

"Four tiers:
- **Free** — single workstation, full CLI. Free forever. This is the viral play.
- **Pro** — $9/month. Neural classification, semantic search.
- **Eye of Horus** — $29/month. Sweep a subnet. 100 nodes.
- **Ra** — enterprise. Fleet enforcement, compliance, SAN scanning.

The agent binary is 2 MB. Deploy to any Linux VM, container, or bare metal server. Fixed command set — no shell access. CISOs love this."

### 4:45 — Closing (15 seconds)

"17 commands. 81 rules. 453 tests. ~8 MB binary. MIT licensed. Zero telemetry.

We're timing this for Product Hunt and Hacker News in April.

The question isn't whether developers waste space — it's whether anyone else is solving this. Right now, nobody is."

---

## Talking Points if Asked

**Q: Why open source?**
A: Viral distribution. CleanMyMac sells for $90/year. We give away the scanner and charge for fleet management.

**Q: Why Go?**
A: Single binary, cross-platform, no runtime. The agent is ~2 MB. Compiles for 6 platforms in 27 seconds. Deploy anywhere.

**Q: Competitive landscape?**
A: CleanMyMac (consumer), Mole (basic OSS), BleachBit (Linux). None understand developer workstations. None have MCP integration.

**Q: TAM?**
A: 27M professional developers worldwide. Average workstation has 10-50 GB of reclaimable infrastructure waste.

**Q: What's the moat?**
A: Rule database (81 rules, growing), neural classification model, MCP integration with Claude/Cursor/Windsurf, and the Egyptian brand identity (Anubis = infrastructure judgment).
