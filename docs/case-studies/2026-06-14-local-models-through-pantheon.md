# Local Models Through Pantheon — The On-Device Broker Case Study

**Date:** 2026-06-14  
**ADR:** [ADR-031 — Local Models Are Consumed THROUGH Pantheon](../ADR-031-LOCAL-MODELS-THROUGH-PANTHEON.md)  
**Deities:** Pantheon runtime owner, Hapi resource governance, Ra/CTR routing, Net architecture  
**Status:** Canonized

## Incident

Pantheon had three useful but fragmented local-model paths:

- `sirsi-gemma` MCP for IDE clients
- `gemma` router worker for zero-token draft/triage work
- `sirsi gemma` CLI for direct human/script use

The risk was letting each Sirsi product or agent wire its own local model window,
runtime, memory budget, and escalation rule. That would make local inference look
cheap while quietly reintroducing the same memory-death and coordination failures
Pantheon exists to prevent.

The decision: local models are consumed through Pantheon. Pantheon owns the
resolver, RAM gate, runtime boundary, broker identity, and escalation contract.

## Findings

### 1. MLX loads per invocation; steady-state free RAM is not the ceiling

The original worker used `mlx_lm.generate` per task: load weights, generate, exit.
Between calls, the model is not resident, and macOS can reclaim inactive/cache
pages. Sizing only to momentary "free" RAM underestimates what can run. The
correct sizing unit is transient peak versus total RAM minus fleet reserve.

### 2. Biggest on paper is a trap

Two failures made this concrete:

- a community fork could load but parse incorrectly and produce garbage;
- an 8-bit 31 GB variant could force enough eviction to Jetsam sibling agents.

The best model is not the largest artifact. It is the largest, newest, coherent
model that fits without harming the fleet.

### 3. The objective function is operational, not aesthetic

Pantheon chooses the model that:

- loads successfully;
- emits coherent output;
- fits the transient peak RAM budget;
- does not evict or kill sibling Claude/Codex/router sessions.

On the observed 48 GB Apple-Silicon host, that made the fleet-safe default more
valuable than a "bigger" quant that destabilized the machine.

### 4. Runtime currency lags model release

New model architectures can land before `mlx_lm` supports every layer/config
shape. The resolver must gate adoption on load success and a coherence smoke
test. A model that is newer but incoherent is not an upgrade.

### 5. Fix the machine, do not shrink ambition by reflex

If the host is under memory pressure, the first response is Pantheon hygiene:
diagnose, clean, exclude noisy development trees from Spotlight, and enforce the
Hapi broker boundary. Sizing down is legitimate only when the fleet-safe reserve
proves the larger model would harm live work.

### 6. The worker produces; it does not refuse

Gemma is a text producer, not a binding authority. It should always provide the
best draft, plan, summary, or analysis it can. If the ask reaches for a binding
verdict, security sign-off, deploy, merge, or tool action, Gemma flags
verification for Claude Home instead of refusing or pretending to act.

The rule is ask-based, not subject-based: `TASK: plan` about a security deploy
stays local; "approve and merge this PR" escalates.

## Canon Outcome

ADR-031 records the product rule:

- consumers call Pantheon, not a bundled model runtime;
- the resolver and RAM gate are shared;
- local inference is A11 local-only, zero telemetry, zero API tokens;
- capability boundaries are stated once at the broker;
- networked/location-transparent inference can arrive later behind the same
  consumer contract.

Follow-up work from this case study produced regression coverage for the worker
escalation rule and the shared Ask Sirsi identity context, so local models answer
as Pantheon product surfaces rather than generic pretrained models.
