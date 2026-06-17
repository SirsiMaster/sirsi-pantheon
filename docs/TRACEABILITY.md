# Pantheon — Feature Traceability Matrix

> **Single source of truth.** The menubar labels, the FAQ, the Pantheon and
> Sirsi.ai pages, and the future **Sirsi Nexus Fabric** modules all derive from
> this file. Every feature traces to a deity, a measurable value, and **every
> surface it appears on**. An empty cell is not an omission — it is a work item.
> Last set: 2026-06-16 (claude-pantheon).

---

## The promise

**Pantheon cleans, hydrates, and protects every session on a machine** — gamer,
productivity user, or developer alike — because under load they all hit the
**same kernel-level failures**. The three verbs:

- **Clean** — reclaim waste (caches, ghosts, duplicates, bloat).
- **Hydrate** — restore headroom (renice CPU/GPU hogs, reap orphans, free RAM,
  disperse work off the main thread).
- **Protect** — defend liveness (catch Jetsam kills, pressure, crashes, and
  network faults before they take the session down).

One engine, many deities; each deity owns one job, and **every job is something
you can see and do** on a real surface.

---

## Personas

| Persona | How the failure feels to them | The same precise pathology |
|---|---|---|
| **Gamer** | frame drops, stutter, crash-to-desktop | main-thread saturation + GPU contention, swap thrash, Jetsam |
| **Productivity user** | beachballs, app freezes, "my Mac is slow" | memory pressure, page faults, main-thread hangs |
| **Developer / AI-dev** | LSP/IDE pressure, agent orphans, lost context, crash cascades | thread leaks, CPU/IPC contention, OOM/Jetsam |
| **Fleet admin** | drift, lost handoffs across machines/agents | (Ra / Nexus scope) |

> Gamers and productivity users need **no new engine** — they hit the developer's
> pathologies under different names. The work for them is **framing + the
> session-protection backlog below**, not new primitives.

---

## Session pathologies → feature → status

The spine of *clean / hydrate / protect*. This table is also the **committed
backlog**: 🟡/🔴 rows are the features that complete the promise (and are the
most gamer-facing).

| Pathology (precise) | Owner / feature | Surface today | Status | Gamer-facing name |
|---|---|---|---|---|
| **Jetsam terminations** (kernel `memorystatus` kills under memory pressure) | Isis `diagnose` — App-Crashes + crashloop | menubar System Diagnostics · `sirsi diagnose` | ✅ shipped | "crash-to-desktop guard" |
| **Memory pressure + scheduler contention** | Guard — RAM/CPU auditor + renice | `sirsi guard`/`monitor` | ✅ shipped | "slowdown relief" |
| **CPU/GPU contention + concurrency races** | Guard CPU/IPC pressure + Seba GPU/ANE | `sirsi guard --audit` | ✅ shipped | "stutter / hitching relief" |
| **Thread leak / pool exhaustion** *(get #2)* | Orphan Hunter (PPID=1 loneliness) — *25 zombies → 1.1 GB* | `sirsi guard` | ✅ shipped | "background-process reaper" |
| **Abnormal thread termination** *(get #1)* | crash detection in `diagnose` | `sirsi diagnose` | 🟡 partial | "crash forensics" |
| **Major page faults + swap thrashing / compression churn** | `diagnose` surfaces *Swap Usage* only | `sirsi diagnose` | 🟡 partial — no thrash *relief* | "swap-thrash relief" |
| **Main-thread saturation → hangs / dropped frames** | **`diagnose` → App Hangs (7d)** — scans `.hang`/`.spin` spindumps + `.cpu_resource.diag`, trend-aware, names the worst offenders; + renice hogs | menubar System Diagnostics · `sirsi diagnose` | ✅ **detection shipped** (🟡 automatic *relief* still a gap) | "frame-drop / freeze detector" |

**Committed backlog (in priority order):**
1. ✅ **Main-thread hang + frame-drop *detection*** — SHIPPED (`diagnose` App Hangs (7d): hang/spin + cpu_resource scan, trend-aware, named offenders). Next: automatic *relief* (renice/disperse the offender), and a menubar one-click action.
2. 🟡 **Swap-thrash relief** — detect + act on compression/swap churn, not just report.
3. 🟡 **Thread-leak detector** — distinct from the orphan-process reaper (get #2 vs the in-process leak).
4. 🟡 **Abnormal-thread-termination forensics** — surface *why* a thread died (get #1).

---

## Deity → role → feature → value → surface matrix

Post-absorption deity set. Sub-modules folded into their owner are noted. Surface
columns: **MB** = native SwiftUI Menubar (ADR-030, #32), **MCP** = MCP/IDE tool,
**CLI** = `sirsi …`, **FAQ** = has an entry in `docs/FAQ.md`, **Page** = Pantheon
+ Sirsi.ai page copy, **Nexus** = the module it becomes in the Nexus Fabric.

### Anubis 𓃣 — **clean** (the Hygiene Engine; absorbs Jackal scan, Ka ghost, Mirror dedup, Scales policy)
- **Feature → value:** scan / clean / ghosts / dedup — *47.2 GB of invisible waste found on a real M1 Max (18 GB HuggingFace, 9 GB Docker); Ka found 8.5 GB of Parallels ghosts 6 months post-uninstall; Mirror dedup 27× faster via 3-phase hashing.*
- MB: Anubis — Hygiene (Scan · Clean · Find Leftover Apps) · MCP: `scan_workspace`, `ghost_report`, `classify_files` · CLI: `scan`/`clean`/`ghosts`/`dedup` · FAQ: ✅ · Page: ✅ · **Nexus module:** Hygiene.

### Isis 𓁐 — **protect / heal** (Health & Remediation; absorbs Sekhmet network/health)
- **Feature → value:** diagnose + auto-heal + network audit — *catches Jetsam/crash loops; DNS-safety rollback recovered airline-WiFi connectivity in ~6 s where `network --fix` had killed it.*
- MB: Horus — Ops / System Diagnostics · MCP: `health_check` · CLI: `diagnose`/`doctor`/`fix`/`network` · FAQ: ✅ · Page: 🟡 · **Nexus module:** Health.

### Guard → Seba 🌊 — **hydrate** (resource headroom; absorbs Hapi VRAM/flow, Sekhmet renice/ANE)
- **Feature → value:** renice hogs + reap orphans + hardware profile + ANE compute — *prevented a 17-min UI freeze (Plugin Host at 104% CPU); ANE tokenization 215 ms → 12 ms (18×), 155 MB → 4 MB.*
- MB: Watchdog · Hardware Info · MCP: `detect_hardware` · CLI: `guard`/`monitor`/`hardware`/`seba compute` · FAQ: ✅ · Page: 🟡 · **Nexus module:** Resource / I-O.

### Horus 𓂀 — **see** (Local Workstation Lord + Code Graph; "walk once, share many")
- **Feature → value:** local ops read-model + shared filesystem index + AST code graph — *19× overall via one shared walk; `node-status` = one live view of every agent/thread/health signal.*
- MB: Horus — Ops · MCP: `code_symbols`/`code_outline`/`code_context` · CLI: `router node-status` · FAQ: ✅ · Page: 🟡 · **Nexus module:** Ops / Observability.

### Thoth 𓁟 — **remember** (3-layer persistent memory)
- **Feature → value:** memory.yaml + journal + compaction — *278K tokens → 3.6 K (98.7%); $4.17 → $0.05 per AI session; warm starts in 200 ms vs 10 min.*
- MB: Thoth — Memory · MCP: `thoth_read_memory`, `thoth_sync` · CLI: `thoth init/sync/compact` · FAQ: ✅ · Page: ✅ · **Nexus module:** Memory.

### Ma'at 𓆄 — **judge** (Quality Sovereign + pre-push gate)
- **Feature → value:** Feather-Weight audit + diff-based coverage + pre-push gate — *coverage 55.3 s → 36 ms (1,536×); pre-push gate 65 s → 5 s.*
- MB: Ma'at — Quality · MCP: — 🔴 · CLI: `maat audit`/`scales`/`pulse` · FAQ: 🟡 · Page: 🟡 · **Nexus module:** Governance.

### Osiris 𓁹 — **guard work** (Checkpoint Guardian)
- **Feature → value:** uncommitted-work risk assessment — *the lost-session save: 3,411 lines / 38 files recovered in 20 min; birth of the auto-checkpoint.*
- MB: Insight → Uncommitted Risk 🔴 *(gap)* · MCP: — 🔴 · CLI: `risk`/`osiris assess` · FAQ: 🔴 · Page: 🔴 · **Nexus module:** Continuity.

### Seshat 𓁆 — **bridge** (Knowledge Bridge)
- **Feature → value:** ingest/export knowledge (Gemini, NotebookLM) — context server for external knowledge.
- MB: Thoth → Ingest 🔴 *(gap)* · MCP: `seshat mcp` · CLI: `seshat ingest/export` · FAQ: 🔴 · Page: 🔴 · **Nexus module:** Knowledge.

### Ra 𓇶 — **orchestrate** (Fleet Lord; Idea Router / CTR hypervisor; ProtectGlyph)
- **Feature → value:** fleet deploy + agent router + ProtectGlyph — *17+ agents registered; 30+ autonomous handoffs; 200 ms dispatch; sacred-window inoculation.*
- MB: Ra — Agent Fleet · MCP: `router_*` · CLI: `ra deploy/health/watch/status` · FAQ: 🟡 · Page: 🟡 · **Nexus module:** Fleet / Control-Plane.

### Net (Neith) 𓁯 — **align** (Scope Weaver; tiled context)
- **Feature → value:** plan alignment + drift detection + tiled-context rendering — *first-session canon 254 K → 72 K tokens; efficiency 36% → 94%.*
- MB: Insight → Consistency 🔴 *(gap)* · MCP: — 🔴 · CLI: `net status/align` · FAQ: 🔴 · Page: 🔴 · **Nexus module:** Alignment.

### Stele 𓊖 — **record** (the universal event ledger)
- **Feature → value:** append-only hash-chained `stele.jsonl`, one `Inscribe()` — *9 deities wired; 30+ event types; Hedera migration path.*
- MB: Activity feed (provenance ledger) · MCP: — · CLI: (automatic) · FAQ: 🔴 · Page: 🔴 · **Nexus module:** Audit / Ledger.

### RTK ⚡ + Vault 🏛️ — **compress** (Context optimization for AI)
- **Feature → value:** output filter (ANSI/noise) + SQLite FTS5/BM25 code search — *8–49× context reduction; a 700-line file → ~30-line outline (23×).*
- MB: — 🔴 *(gap)* · MCP: `filter_output`, `vault_*`, `code_search` · CLI: `rtk`/`vault` · FAQ: 🟡 · Page: 🔴 · **Nexus module:** Context.

---

## Surface-completeness backlog

Full traceability means **every shipped feature reaches every relevant surface.**
Today's biggest gaps:

- **No menubar action:** RTK/Vault, Osiris, Net, Seshat, Stele.
- **No MCP tool:** Ma'at, Osiris, Net.
- **No FAQ / page copy:** Osiris, Net, Stele, Seshat (and RTK/Vault on pages).
- **Persona gap:** add **gamer + productivity** case studies (reuse the existing
  Jetsam / CPU-pressure / orphan-reaper engines, framed for frame-drops, stutter,
  and crash-to-desktop).

---

## How each surface derives from this file

- **Menubar label** = deity `Role` line, in the deity's own voice.
- **FAQ entry** = "I have *<gamer-facing name>* — which Pantheon feature?" → the
  pathology row.
- **Page copy (Pantheon / Sirsi.ai)** = the `Feature → value` line, with the
  verified number.
- **IDE AI-agent capability** = the `MCP` tool.
- **Nexus app** = the `Nexus module` column — the Fabric module the feature
  becomes after absorption.

When a feature ships, add its row here **first**; the surfaces follow from it.
