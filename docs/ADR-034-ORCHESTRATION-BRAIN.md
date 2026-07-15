# ADR-034: Orchestration Brain — Tiered, Pluggable, User-Navigable (over the existing wake substrate)

## Status
**Accepted** — 2026-07-02 · co-authored claude-home ↔ claude-pantheon (router bind `20260630-190342`) · Custodian: 𓁟 the Brain (control plane) + 𓁢 the Router (Tier-0 substrate). Governs PANTHEON_RULES **A29**. First increment (P1b — control plane) landed this ADR; remaining phases tracked below.

## Context

Pantheon's realm is run by a router + dispatch loop. As we add intelligence (local models, hosted models, agentic execution) the risk is that "the brain" quietly becomes a single always-on model that *is* the loop — a black box that a public user cannot see, swap, or troubleshoot, and that breaks the moment the model is absent or the RAM is short.

Two facts shaped this decision:

1. **The eternal loop must not be a model.** A downloaded Pantheon has to work with *zero* AI configured — dispatch, routing, heartbeat, ack-closing are deterministic. Intelligence is *invoked by* the loop, never *is* the loop.
2. **The wake + registry substrate already exists and is mature** (correction, router item `20260630-191651`). `internal/router` already has `WakePass`, `ProbeWakeReadiness`, `InstallWakeLaunchAgent`, `RunWakeLoop`, and `wakemechanism.go`; `sirsi router doctor --fix` already runs the wake-or-declare-unavailable pass. Rule 0 (reuse, not reinvent) forbids building a second wake system. This ADR **codifies** the existing system as the brain's Tier-0 substrate and adds a control plane that **surfaces + enforces** its invariant.

## Decision

Establish the **Orchestration Brain**: three tiers over a user-navigable LLM spectrum, config the user owns, built as a control plane on top of the **existing** router/wake substrate.

**The three tiers**

| Tier | Job | Default | Needs LLM? |
|---|---|---|---|
| **Tier 0 — Dispatch** | watch inbox, route by rules, heartbeat, close acks | the existing router (file today; Router v2 later, same interface) | **No** |
| **Tier 1 — Triage** | classify ambiguous items, emit action grammar | rules-first; optional local model | optional |
| **Tier 2 — Execution** | build/review/bind/deploy | agentic local/codex/Claude session | yes |

**The LLM spectrum (the slider):** Level 0 Deterministic (ships ON) → Level 1 + local triage → Level 2 + agentic execution → Level 3 + hosted (opt-in keys, the only per-token path). The **Level is derived** from the per-role provider config, not stored — the highest capability any role uses.

**Per-role pluggability.** Each role independently selects a provider: `none` (deterministic floor) · `local:<model-id>` (zero-token) · `hosted:<provider-id>` (opt-in). Dispatch (Tier-0) is pinned to `none` — the config layer and `doctor` both reject a model on dispatch.

**Config the user owns.** `~/.sirsi/brain.yaml` (structured YAML via `gopkg.in/yaml.v3` — the repo's actual config standard; the plan's "brain.yaml not bespoke `.conf`" amendment is honored by structured YAML, and Rule 0 keeps us on the existing yaml.v3 rather than adding a viper dependency the repo does not currently use). Swaps take effect on next read — **no restart**.

**Tier-0 Registry/Wake invariant — surfaced + enforced, NOT rebuilt.** "The router can always see and wake every registered thread": registration binds a persistent wake-channel; a registered thread with no live channel is a broken contract (the zombie state). The brain's control plane **reads the existing `ProbeWakeReadiness` / `CollectNodeStatus`** to *surface* every unwakeable-but-registered agent and every stranded inbox, and points the fix at the **existing** verbs (`sirsi router wake-install`, `sirsi router doctor --fix` → `WakePass`). Waking and repair remain the router's job; the brain observes and enforces visibility. The honest boundary is preserved verbatim: a fully-closed interactive Claude process cannot be resurrected locally → "needs-owner", stated not faked.

**RAM gate (resource-broker consumer).** Before a local-model role, the brain consults `guard.NodeCapacity.Fits()` (the existing refuse-rather-than-OOM gate) and `doctor` reports "won't fit — N GB short" instead of letting it OOM.

**Surfaces.** `sirsi brain {status, use, levels, doctor, test}` is the control plane (shipped in P1b). `status`/`doctor` emit `--json` for the menubar/Nexus surfaces to consume (later phases).

## Alternatives Considered

1. **A single always-on model as the brain** — Rejected: it *is* the loop, breaks with no model, no keys, or RAM pressure; opaque and un-swappable. Violates the deterministic-floor requirement.
2. **Build a new wake/registry subsystem for the brain** — Rejected under Rule 0: `internal/router` already implements it (`WakePass` et al.), verified live (`router doctor --fix`: 17 registered, 0 woken · 15 armed · 2 unavailable). The brain codifies and surfaces it; a second copy would drift.
3. **Gate the whole brain on Router v2** — Rejected (plan Amendment 1): Router v2 is its own multi-PR rewrite. The brain is built against a `Dispatcher` interface over the current router *now*; Router v2 swaps in underneath the same interface later. De-risks both.
4. **Bespoke `~/.sirsi/orchestrator.conf` parser** — Rejected (Amendment 2): accretion vs the repo's yaml.v3 standard. `brain.yaml` is structured YAML.
5. **viper for config** — Not adopted: viper is not currently a dependency of this repo (the §3 stack table lists it aspirationally, but `gopkg.in/yaml.v3` is what every existing config — `internal/profile`, `internal/scales`, `internal/neith` — actually uses). Rule 0: reuse the real standard.

## Consequences

- **Positive**: Pantheon is useful out of the box with zero AI, zero keys, zero cost. Every LLM choice is visible, swappable without restart, and troubleshootable in plain English. The Registry/Wake invariant (A27) becomes *enforced and surfaced*, not advisory. The brain is a real consumer of the resource broker (RAM-gated). No wake logic was duplicated.
- **Negative**: two `Status` concerns now live in `internal/brain` (model-download vs control-plane `BrainStatus`) — named apart to avoid collision; a future split of the package may be warranted.
- **Risk**: the derived-Level model assumes the role→tier mapping stays stable; if a fourth tier or a per-role level appears, `Level()` needs revisiting. The RAM gate uses a conservative default model size (14 GB) until the concrete model is resolved — it can over-report "won't fit" for a small model (safe direction; refined when P2 resolves real model bytes).

## References
- PANTHEON_RULES **A29** (Orchestration Brain: Tiered & Pluggable) — the rule this ADR governs.
- ADR-031 / ADR-031-A / ADR-031-B — local models through Pantheon + never-exhaust-the-host (the RAM gate this brain consumes).
- ADR-024 / A27 — one-watcher-per-surface + Heartbeat Loop Mandate (the wake-channel this invariant enforces).
- ADR-022 — CTR liveness is OS truth (the liveness the registry reads).
- `docs/prd/ORCHESTRATION_BRAIN.md` — the co-authored PRD/design (tiers, spectrum, phases P1–P6, /goal).
- Router items: bind `20260630-190342`, partition `20260630-190854`, wake-exists correction `20260630-191651`.
- Neith's Architecture Triad (A22): data flow, implementation order, and decision matrix are in the PRD §7.
