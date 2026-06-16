# Pantheon — Vision

> Status: canonical north-star. Anchors product, messaging, and roadmap.
> Last set: 2026-06-16.

## The thesis

**Pantheon is the operating system for AI-augmented computing** — a self-healing
compute substrate that keeps your *machine*, your *memory*, and your *agents*
alive, private, and governed, from laptop to sovereign hardware. It maintains
itself and hosts the intelligence running on top of it — across **workstations,
networks, and IDEs**.

Today's framing ("infrastructure hygiene") undersells it. CleanMyMac is hygiene.
The pinnacle is a *living substrate* that maintains itself and hosts the
intelligence running on top of it. That reframe is the whole game: the difference
between a utility you open and a layer you forget is there because it never lets
anything break.

## Three pillars, one loop

### Pillar I — It heals the machine (the substrate stays alive)
The deities stop being a menu of tools and become a single control loop — the
OODA loop for your computer:

**Sense → Understand → Decide → Act → Remember → Learn**

- **Sense** (Horus + Seba) — continuous telemetry, not on-demand scans. The
  sensory cortex. At its pinnacle it *predicts*: "7 Jetsam kills/7 days, trending
  — you will OOM in ~2h" before the crash, not after.
- **Understand** (Gemma + Thoth) — local reasoning over the signal *plus memory
  of your machine*. Not "swap is high" but "swap is high because the same app
  leaks every afternoon — third time this week."
- **Decide** (Ma'at) — the governance gate: is this action safe and permitted at
  your trust level? The part nobody else builds, and the reason autonomy is
  trustable.
- **Act** (Anubis + Isis) — execute, always trash-first and reversible. Anubis
  reclaims, Isis heals system/network/IDE domains.
- **Remember** (Thoth) — record cause + outcome, so the next decision is smarter.
- **Learn** — the loop tunes its own thresholds to *your* machine.

"Health → cause → one-click remediation" is just the *manual* version of this
loop. The pinnacle closes it.

### Pillar II — It hosts the intelligence (private, on-device, free)
The real moat. Pantheon owns the machine's local AI:

- **Gemma as the local brain** — Pantheon manages the models, schedules them onto
  the Neural Engine (Seba), and frees RAM to fit the biggest model that runs. Any
  app, and every Sirsi product, gets private local inference *through* Pantheon
  via MCP. Pantheon is the on-device AI layer.
- **Thoth as living memory** — not a write-once log but an *actively maintained
  substrate*: stale facts decay, contradictions reconcile, hot context surfaces.
  A knowledge graph of machine + code + decisions that warm-starts every agent.
- **The agent host (router/CTR)** — Pantheon registers agents, heartbeats them,
  reaps the dead, routes peer messages, governs their actions. Pantheon is where
  AI agents *live* on your machine.

### Pillar III — It governs the action (the trust layer)
Autonomy is worthless without trust, and trust is earned mechanically:

- **Everything reversible** — trash-first, snapshots, a time machine for system
  state. Every remediation has an undo.
- **Everything has provenance** — a ledger: what changed, why, what triggered it,
  what the outcome was.
- **Ma'at as policy-as-code** — continuous compliance, the autonomous Ma'at→Isis
  heal cycle, the pre-push gate fleet-wide. The conscience that says what the loop
  may do on its own.

## The keystone: the autonomy ladder
You set a trust level per action class — the dial is the product.

| Rung | Behavior |
|---|---|
| Observe | Watches, surfaces, never acts. |
| Recommend | One-click reversible fixes with provenance. |
| Auto-heal blessed classes | Silently reclaims safe caches, heals binary drift, clears crash backlog — tells you after. |
| Self-governing | Ma'at-gated autonomy across most domains; review the ledger weekly, not the actions. |

## Positioning: the OS that agent teams run on
Anthropic shipping Claude Code **agent teams** (ephemeral, in-session, single-repo,
ungoverned, cloud-model peers that "wake with zero context") validates the
agent-host pillar — and hands Pantheon its wedge:

> **Agent teams are the app. Pantheon is the OS they run on** — it remembers them
> (Thoth), supervises them (CTR/reaping), governs them (Ma'at), powers them
> locally (Gemma), and orchestrates them across the fleet (Ra). Anthropic ships
> the ephemeral team; Pantheon makes it persistent, private, and governed.

| Dimension | Agent teams (the app) | Pantheon (the substrate) |
|---|---|---|
| Lifespan | One session, then gone | Persistent — threads, reaping, resume |
| Memory | Zero context on wake | Thoth warm-start knowledge graph |
| Scope | One repo, one task | Cross-repo fleet (Ra), whole machine |
| Governance | None | Ma'at policy gate, reversible, provenance |
| Models | Cloud, metered | Local Gemma broker — private, free — + cloud |
| Supervision | In-process, lead-spawned | OS-level liveness, PID-aware reaping |
| Host health | N/A | Self-healing host underneath |

**The frontier:** a *self-modifying substrate* — peer agents that review and fix
each other's work, hosted on a substrate that remembers what worked (Thoth) and
governs what they may change (Ma'at). The substrate that rewrites itself is a
level beyond the substrate that heals itself.

## Roadmap: today → pinnacle
- **Phase 0 — Surfaces solid** *(now)*: menubar actionable, CLI complete,
  FDA/signing fixed, uniform `CommandResult` contract (`{summary, evidence,
  next_actions}`) as the spine.
- **Phase 1 — Close the loop (assisted)**: every finding → one-click *reversible*
  remediation + provenance ledger. The `next_actions` in the JSON *are* the fix
  buttons.
- **Phase 2 — Memory & explanation**: Thoth graph + Gemma explains every finding
  in plain language; warm-start.
- **Phase 3 — Graduated autonomy**: the trust ladder; auto-heal blessed classes;
  full undo.
- **Phase 4 — Local AI broker**: Pantheon serves inference to all apps via MCP;
  Seba schedules RAM/NPU.
- **Phase 5 — Fleet & substrate**: Ra heals across machines; the Cube as the
  sovereign on-prem TPU node.
- **Phase 6 — Distribution**: notarized brew cask, clean upgrades.

## North star — a day in the life
> You open your Mac at 9am. The glyph is green; it has been for weeks. Overnight
> Pantheon reclaimed 14GB of regenerable caches (trash-first, undo in the ledger),
> healed a drifted `sirsi` binary before it SIGKILL'd, and noticed your language
> server leaked 6GB three afternoons running — so it filed that as a *pattern* and
> Gemma drafted a one-line note in your Thoth journal. You click the glyph:
> **"Nothing needs you. Here's what I handled, and one thing I'd like permission
> for."** You approve the one thing and close it. You never think about your
> machine's health again — which is the point.

## Design principles
- **It disappears.** Ambient, proactive; you forget it's there.
- **It earns trust.** Reversible, explainable, provenance for everything.
- **It compounds.** Memory + learning means it gets better at *your* machine.
- **It's sovereign.** Everything on-device; cloud optional, never required.
- **One substrate, many windows.** CLI, menubar, Horus, MCP, iOS — consistent.
- **Mole-quality bar.** Every screen actionable; nothing half-built ships.
