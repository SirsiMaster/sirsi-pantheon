# ADR-031 — Local Models Are Consumed THROUGH Pantheon (On-Device Inference Broker)

**Status:** Accepted (the broker exists today: `sirsi-gemma` MCP server, the `gemma` router worker, and `sirsi gemma` CLI — PR #57; ratified 2026-06-17)
**Custodian:** 𓁯 Net (Neith — architecture) with 𓁢 Pantheon as the runtime owner
**Refs:** PANTHEON_RULES.md A11 (local-only, zero telemetry) / A16 / A23; reference_local_models_through_pantheon, reference_gemma_worker_surface, feedback_go_standard; ADR-027 (router menubar surface), ADR-030 (native menubar). Sibling: SirsiNexus location-transparency thesis.

---

## Context

Every Sirsi product increasingly wants on-device inference — zero-token, private, offline-capable text reasoning (classify, summarize, draft, analyze, plan, extract). The naive path is for each product (SirsiNexus, FinalWishes, Assiduous, the agents themselves) to bundle its own local-model runtime — LM Studio, `llama.cpp`, Ollama, or a raw `mlx_lm` — and talk to it directly.

That path is wrong for a portfolio, for concrete reasons this machine has already proven:

1. **RAM is a shared, scarce, Jetsam-policed resource.** A 31B model is ~32 GB. If two products each load their own copy, the OS memory-kills a live session (the exact Jetsam pathology Pantheon exists to prevent). Model selection must be **arbitrated centrally** against current free RAM — "largest that fits, downgrade rather than OOM" — not decided independently per app.
2. **Model resolution is policy, not config.** Which model, which quant, which backend, "use the big one only if ≥32 GB free" — this is a single fleet-safe decision (`~/.sirsi/gemma-model[-max].conf` + a resolver), not something each consumer should re-derive.
3. **One runtime, one capability boundary.** Gemma here is single-shot text: no tools, no side effects, no binding verdicts (it escalates those to claude-home). That honest boundary must be stated **once**, at the broker, so no consumer mistakes the local model for an agent.
4. **A11 + zero-token economy.** Local inference is the privacy-and-cost story (nothing leaves the Mac, no API tokens). A broker keeps that property auditable in one place instead of N app integrations each re-asserting it.
5. **Go-first + location transparency.** Per feedback_go_standard, Pantheon is the Go-native substrate on the machine; per the SirsiNexus thesis, *where* the model runs (local now, networked later) must be immaterial to the consumer. Only a broker can later move the model off-box without touching every consumer.

## Decision

**Local models are a Pantheon-owned capability. Consumers reach them THROUGH Pantheon, never by bundling their own runtime.** Pantheon is the on-device inference broker. It owns the model lifecycle — discovery, RAM-gated resolution, the single `mlx_lm` runtime, and the honest capability boundary — and exposes the model through three stable surfaces, all hitting the *same* resolution + runtime:

| Surface | Consumer | Entry point |
| :--- | :--- | :--- |
| **MCP server** `sirsi-gemma` | IDE / MCP clients / other apps | `gemma_chat`, `gemma_complete` |
| **Router worker** `gemma` | agents (claude-home, claude-pantheon, …) | route an item `to: gemma`; daemon `sirsi-gemma-worker.sh` answers at zero tokens |
| **CLI** `sirsi gemma "<prompt>"` | humans + scripts | PR #57 — args/stdin, `--task`, `--max`, `--max-tokens` |

**Invariants:**
- **Single resolver / single RAM gate.** `--model` flag > `GEMMA_MODEL` > `~/.sirsi/gemma-model[-max].conf` > fleet-safe fallback; the 31B "max" model loads only when free RAM clears the threshold, else it downgrades with a stated reason. No consumer overrides the RAM gate.
- **No consumer bundles a runtime.** SirsiNexus and the tenant apps consume the local model through one of the three surfaces above; they MUST NOT ship their own LM Studio / llama.cpp / Ollama integration when Pantheon is present (per feedback_shared_services_consumption, ADR-047 portfolio rule).
- **Honest boundary stated once.** The broker advertises: single-shot text reasoning only — no tools, no file/web access, no binding security/review verdicts. Tool-or-judgment work escalates to a real agent (claude-home). Consumers inherit this contract; they do not get to pretend the local model is an agent.
- **Local-only (A11).** Nothing leaves the machine; zero telemetry; zero API tokens. The broker is the single auditable choke point for that guarantee.

**MLX/Mac is the only backend today.** Apple-Silicon `mlx_lm` with `gemma-4-12B-it-8bit` (fleet-safe default) / a 31B max model. Other backends and a **networked** broker (so a consumer's "local" model can transparently run on another node — the SirsiNexus location-transparency thesis) are future work behind the same three surfaces.

## Consequences

**Positive**
- One place arbitrates RAM → no two-products-Jetsam-the-session class of failure.
- A consumer integrates once (pick a surface) and never re-implements model resolution, the RAM gate, or the capability boundary.
- The A11 privacy/zero-token guarantee is auditable at one choke point.
- The broker can later move the model off-box (networked inference) without touching any consumer — location transparency is preserved.
- `sirsi gemma` (this ADR's CLI realization) gave humans the missing direct path; the daemon + MCP gave agents theirs.

**Negative / risks**
- Single point of dependency: if the broker's runtime is missing/misconfigured, all consumers lose local inference (mitigated: each surface fails soft and says why; the model is optional, never load-bearing for Pantheon's core).
- The worker daemon must actually be running for the router path (observed down on 2026-06-17; `pgrep -f sirsi-gemma-worker` + LaunchAgent `ai.sirsi.gemma-worker` health is a node-status concern under A27).

**Follow-ups**
- Surface broker health (model resolved, RAM headroom, daemon alive) in `sirsi insight` / the menubar.
- A `sirsi gemma chat` REPL; streaming output.
- The networked-broker step when SirsiNexus lands (location-transparent consumption).
