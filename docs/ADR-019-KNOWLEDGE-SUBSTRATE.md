# ADR-019: Knowledge Substrate — Three-Tool Local Layer + Hypergraph Direction

## Status
**Accepted (local layer) · Hypergraph direction SUPERSEDED** — 2026-05-26; superseded 2026-07-23

> **Substrate ownership moved.** The "Hypergraph Direction" half of this ADR is superseded by
> **[sirsi-hypergraph](https://github.com/SirsiMaster/sirsi-hypergraph)**, which now owns the
> event-sourced cross-repo substrate (canon: that repo's `docs/ADR-003-NEURAL-FRACTAL-ARCHITECTURE.md`;
> Horus = machine scope, Ra = fleet scope). One implementation of the pattern, per LEAN: Pantheon
> ships **no** hypergraph package or `sirsi hypergraph` verb — use the `hypergraph` binary, whose
> union bus lives at `~/.sirsi/hypergraph/events.jsonl` (a superset of the per-repo bus this ADR
> once described).
>
> **Still current and Pantheon-owned:** the three-tool LOCAL layer below — Thoth memory, Seba
> architectural mapping, and the Understand-Anything knowledge graph. Those feeders stay sovereign
> here and are consumed BY the hypergraph, not replaced by it.
**Deciders:** Cylton Collymore (architect-led decision), claude-pantheon (proposal + implementation), codex-pantheon (review pending — router item `20260527-174419-claude-pantheon-codex-pantheon-completion-understand-anything-install-hypergraph-vision-cod.md`)
**Related:** [ADR-004 (Ma'at QA/QC Governance)](ADR-004-MAAT-GOVERNANCE.md), [ADR-007 (Unified Findings Portal)](ADR-007-UNIFIED-FINDINGS-PORTAL.md), [ADR-008 (Shared Filesystem Index)](ADR-008-SHARED-FS-INDEX.md), [ADR-014 (Stele Ledger)](ADR-014-STELE-LEDGER.md), [ADR-015 (Deity Hierarchy)](ADR-015-DEITY-HIERARCHY.md)

> **Canonical companion:** `~/Development/HYPERGRAPH_VISION.md` (workspace-root, audience: future Sirsi Nexus builder agents). This ADR ratifies the local layer; the vision doc defines the substrate it feeds.

---

## Context

By mid-2026, Pantheon (and the broader Sirsi portfolio) has accumulated three independent but overlapping mechanisms for capturing what the codebase knows about itself:

1. **Thoth memory** (`.thoth/memory.yaml` + `journal.md`) — hand-curated and AI-curated narrative state. Captures *why* and *what next*. ~100 lines compressed; auto-loaded at AI session boot per CLAUDE.md / AGENTS.md.
2. **Seba** (`internal/seba/`) — the deity-owned **architectural mapping** module per the Deity Registry (Rule A25). Sovereignty over canonical topology.
3. **Tribal knowledge in agent memory + idea-router items** — decisions, handoffs, sprint deltas, distributed across `.agents/idea-router/` and individual AI agents' private memory files.

What has been missing is a **semantic ground-truth** layer derived directly from source code — a graph of files, functions, classes, imports, and architectural layers that does not require a human or an AI to keep current. Reading a 367-Go-file codebase to answer "what imports `internal/mcp/tools.go`?" costs tokens that don't scale.

On 2026-05-26, the Understand-Anything Claude Code plugin (`Lum1104/Understand-Anything` v2.7.5) was installed and run end-to-end on sirsi-pantheon at commit `22ec913`, producing `.understand-anything/knowledge-graph.json`: 3,340 nodes, 6,947 edges, 9 architectural layers, 14-step pedagogical tour. The artifact is portable JSON, query-able by `jq` alone, and consumed by the same plugin's chat / explain / onboard / diff skills.

This forced a question Cylton had already been carrying: **what is the relationship between Thoth, Seba, and a tool like Understand-Anything?** And, transitively: **what is the long-term knowledge architecture for the Sirsi portfolio?**

User answer (verbatim, 2026-05-26): *"the reason this works is that there will be an agent knowledge graph called the hypergraph in the future based on Hedera graph technology... understand just maps my repo."*

And, separately: *"i also like the reliance on json which is machine readable architectural code."*

This ADR codifies the three-tool split *and* the destination direction so neither over-investment nor redundancy accumulates in the local layer.

## Decision

### 1. The Knowledge Substrate is a three-node-type abstraction with two scopes

| Scope | Memory nodes (why) | Structure nodes (what exists) | Routing events (who/when) |
|---|---|---|---|
| **Local (today)** | `.thoth/memory.yaml` + `.thoth/journal.md` per repo | `.understand-anything/knowledge-graph.json` per repo | `.agents/idea-router/items/*.md` per repo |
| **Global (future)** | Memory shards ingested into the hypergraph | Structure subgraphs unioned across repos | Event log on Hedera Consensus Service |

The three node types **must remain separated** in the local layer because they map cleanly to three node types in the future hypergraph. Pre-merging them today would forfeit that natural shape.

### 2. Tool sovereignty within Pantheon

| Tool | Sovereignty | What it owns |
|---|---|---|
| **Thoth** (𓁟) | Memory + intent | The *why* and *what next*. Curated. Auto-loaded at session boot. Lives in `.thoth/`. |
| **Seba** (𓇽) | Architectural mapping | The canonical *layered topology*. Deity-owned per ADR-015 / Rule A25. Sovereignty unchanged by this ADR. |
| **Understand-Anything** (external plugin) | Semantic verification | The *auto-derived ground truth* — files, functions, classes, imports, layers, tour. Lives in `.understand-anything/`. |

These three do not compete. They sit at different levels of abstraction (memory / map / proof) and answer different questions (why / where in the topology / what literally exists).

### 3. JSON is the canonical interchange format

Every artifact that crosses tool / agent / repo boundaries MUST be JSON or trivially convertible to JSON (YAML 1.2 is a JSON superset and is acceptable for human-curated files like `memory.yaml`). Rationale: JSON is machine-readable architectural code — portable, language-agnostic, agent-agnostic, diff-able, and ingestable by any future hypergraph projection without translation.

See `~/.claude/projects/.../memory/feedback_json_arch_code.md` for the apply-to-the-code form of this principle. Anti-patterns: unsafe language-native serializations, proprietary binary formats without an open schema, YAML anchors/aliases that break JSON round-trip, "we'll convert to JSON later."

### 4. Bidirectional sync between Thoth and Understand-Anything

After every `/understand` run in any repo with a `.thoth/`:

1. Update the `## Knowledge Graph (Understand-Anything)` block in `.thoth/memory.yaml` (`last_analyzed_commit`, `last_analyzed_at`, node/edge counts, layer counts).
2. Append a journal entry to `.thoth/journal.md` summarizing the **delta** — new packages, layer shifts, edge-count changes. One paragraph is sufficient.
3. If `memory.yaml` lacks a Knowledge Graph block, create one matching the schema in `~/Development/sirsi-pantheon/.thoth/memory.yaml`.

This contract is encoded in `~/CLAUDE.md` (global) so it applies to every AI session in every Thoth-enabled repo without per-project hooks.

### 5. The hypergraph is the long-term destination, not active work

The substrate is **Hedera Consensus Service** (HCS) — aBFT finality, ~3–5s settlement, fractions of a cent per message. Graph topology materializes off-chain from the HCS event stream (event sourcing); replay always converges. Not religiously locked to Hedera; the architectural primitive needed is "ordered, durable, multi-writer event log." Hedera is the strongest fit today.

Detailed design principles, phasing sketch, and anti-patterns for future builders are in `~/Development/HYPERGRAPH_VISION.md`. Pointer added to `~/Development/AGENTS.md` § Knowledge Substrate so every agent at session boot discovers it.

**Builders implementing the hypergraph MUST read `HYPERGRAPH_VISION.md` before designing schemas or restructuring the local layer.**

### 6. CLI surface — the "switch within Sirsi overall"

> **SUPERSEDED 2026-07-23 — never implemented in Pantheon, and deliberately so.** The verbs below
> were a 2026-05 stub design. The substrate now lives in
> [sirsi-hypergraph](https://github.com/SirsiMaster/sirsi-hypergraph) and its own `hypergraph`
> binary carries the surface (`ingest` / `replay` / `query` / `nodes`). Pantheon ships no
> `sirsi hypergraph` verb — one implementation of the pattern (LEAN). The block is kept for
> provenance: it records what was designed and why the ownership moved.

```
sirsi hypergraph status               # Show local layer state: which artifacts exist, last analyzed
sirsi hypergraph status --json        # Machine-readable for CI / agent consumption
sirsi hypergraph refresh              # Re-run /understand incrementally (uses fingerprint baseline)
sirsi hypergraph refresh --full       # Force full rebuild
sirsi hypergraph chat <question>      # Q&A against the knowledge graph (wraps understand-chat skill)
sirsi hypergraph explain <path>       # Deep-dive on a file (wraps understand-explain)
sirsi hypergraph diff <ref>           # Impact analysis vs a git ref (wraps understand-diff)
sirsi hypergraph layers               # List architectural layers + file counts
sirsi hypergraph tour                 # Print the pedagogical tour
sirsi hypergraph export <format>      # Emit graph for external tooling (json | dot | mermaid)
```

A configuration entry under `configs/hypergraph.yaml` controls behavior:

```yaml
# configs/hypergraph.yaml
enabled: true                         # Master switch — set false to disable all hypergraph commands
auto_refresh:
  on_commit: false                    # Run /understand after every git commit (incremental via fingerprints)
  on_branch_switch: true              # Refresh when switching to a long-lived branch
exclude:                              # Mirrors .understand-anything/.understandignore (single source of truth)
  - "docs/case-studies/**"
output_language: en
sync:
  thoth: true                         # Auto-update .thoth/memory.yaml + journal.md after refresh
  hcs_publish: false                  # Future: publish events to Hedera Consensus Service
hcs:                                  # Future, inert until projection layer exists
  network: testnet                    # testnet | mainnet
  topic_id: ""                        # HCS topic for this repo's event stream
```

The switch is honest: when `enabled: false`, every `sirsi hypergraph *` command prints a one-line "knowledge substrate is disabled in `configs/hypergraph.yaml`" and exits non-zero. When the binary is compiled without the `--tags hypergraph` build tag, the subcommand is absent entirely — no hidden runtime cost in fleet binaries that don't want it.

### 7. Distribution surface

The capability is documented at the same surface as every other Pantheon deity:

- **User guide:** `docs/user-guides/knowledge-substrate.md`
- **Web page:** `docs/pantheon/knowledge-substrate.html` → published to `sirsi.ai/pantheon/knowledge-substrate`
- **Case study:** `docs/case-studies/2026-05-26-knowledge-substrate-day-1.md`
- **Index update:** `docs/pantheon/index.html` (deity grid gains a knowledge-substrate card)
- **Architecture mention:** `docs/ARCHITECTURE_DESIGN.md` § Knowledge Substrate (cross-references this ADR + workspace HYPERGRAPH_VISION.md)
- **Changelog:** under `[Unreleased]` → `Added`

## Alternatives Considered

1. **Build a Pantheon-native knowledge graph from scratch instead of using Understand-Anything.** Rejected. The plugin is open-source (`Lum1104/Understand-Anything`), produces portable JSON, and works today. Building in-house duplicates effort with no clear differentiation. If we outgrow the plugin's output, we can fork or replace it — the JSON is the contract, not the tool.
2. **Merge Thoth and the knowledge graph into a single artifact.** Rejected. They serve different audiences (humans + sessions vs. queries), have different volatility profiles (curated vs. auto-derived), and map to different node types in the future hypergraph. Merging would create a single artifact that fails both jobs.
3. **Make the hypergraph an active workstream now.** Rejected. The local layer is not yet stable across repos (only sirsi-pantheon has been indexed; SirsiNexusApp / FinalWishes / Assiduous / Ask Eliot have not). Building the substrate before the feeders are clean would be premature optimization. ADR captures direction; implementation deferred.
4. **Choose Ethereum / Avalanche / IOTA for the hypergraph substrate.** Considered. Hedera HCS wins on (a) latency for high-volume knowledge events, (b) cost per message, (c) aBFT consensus profile, (d) prior team familiarity (Cylton's Zimbali Networks received a $500K Hedera grant — operational knowledge of the SDK and economics exists in-house). Not religiously locked; if a better primitive emerges, the event-sourcing design ports.
5. **Commit `.understand-anything/` to git vs. `.gitignore` it.** Open question, deferred. Current state: untracked. Decision deferred to codex-pantheon review (see router item). Tradeoff documented in handoff.

## Consequences

### Positive

- **Token cost of repo onboarding drops.** AI agents in sirsi-pantheon now answer "what imports X" by `jq`-ing 2.9 MB of JSON instead of reading 367 Go files. Measured savings to be reported in the case study after one week of use.
- **Future hypergraph is now feasible without rework.** Local artifacts are JSON-shaped, repo-portable, and standardized — an ingestor can be built without translation logic per source.
- **Seba's deity sovereignty is preserved.** Architectural mapping remains deity-owned; Understand-Anything is the verifier, not the architect. The two now have an explicit functional split rather than implicit overlap.
- **Documentation surface unified.** The knowledge substrate has a canonical web page, user guide, case study, ADR, and architecture mention — discoverable via the same paths as every other Pantheon capability.

### Negative

- **Per-repo discipline required.** Every Thoth-enabled repo now carries the Knowledge Graph block + sync protocol. Repos that have not run `/understand` will have stale blocks until the first run. Mitigated by the bidirectional sync rule in `~/CLAUDE.md` — agents detect the gap and refresh.
- **Plugin dependency.** Understand-Anything is a third-party plugin (`Lum1104`). Risk: upstream abandonment. Mitigation: the produced JSON is the contract, not the tool. If the plugin disappears, the JSON survives and can be regenerated by a replacement that targets the same schema.
- **Surface area expansion.** New CLI subcommand + new config file + new docs to maintain. Mitigated by feature-flag gating (`enabled: false` is a one-line disable) and build-tag exclusion for binaries that don't need it.
- **Swift / Kotlin coverage incomplete in v1.** The plugin's tree-sitter doesn't bundle Swift/Kotlin grammars; iOS and Android code is file-level only in the graph. Acceptable for v1; flagged upstream.

### Neutral

- **Hedera commitment is directional, not contractual.** ADR-019 commits to the *substrate primitive* (ordered durable multi-writer event log). Hedera is the strongest current candidate. If Hedera changes terms or a better primitive emerges, the local layer is unaffected.

## Verification

- ✅ `.understand-anything/knowledge-graph.json` exists in sirsi-pantheon (3,340 nodes / 6,947 edges, 2.9 MB).
- ✅ `.thoth/memory.yaml` § Knowledge Graph block added and synced.
- ✅ `.thoth/journal.md` § 2026-05-26 entry written.
- ✅ `~/Development/HYPERGRAPH_VISION.md` written and referenced from `~/Development/AGENTS.md`.
- ✅ Global `~/CLAUDE.md` Thoth ↔ Understand sync rule added.
- ⏳ `sirsi hypergraph` subcommand — design in this ADR; implementation pending Codex review.
- ⏳ `configs/hypergraph.yaml` — schema in this ADR; file creation pending implementation.
- ⏳ Propagation to other Sirsi repos (SirsiNexusApp, FinalWishes, Assiduous, Ask Eliot) — pending.

## Refs

- `ANUBIS_RULES.md` § 0 (Identity), § 2.22 (Deity Registry & Attribution), § 2.23 (Idea Router Workstream Protocol)
- `docs/ARCHITECTURE_DESIGN.md` § Knowledge Substrate (added 2026-05-26)
- `~/Development/AGENTS.md` § Knowledge Substrate
- `~/Development/HYPERGRAPH_VISION.md` (full builder doc)
- `~/.claude/projects/.../memory/project_hypergraph_vision.md` (claude-side memory)
- `~/.claude/projects/.../memory/feedback_json_arch_code.md` (JSON preference, codified)

**Changelog:** v0.22.0-beta → `Added` — ADR-019 Knowledge Substrate.
