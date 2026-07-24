# ADR-046 — Local Sovereignty: the T0 Floor and API-Offline Takeover

**Status:** Accepted (design) · 2026-07-23 · implementation phased
**Extends:** ADR-031 (broker governance), ADR-040 (do no harm), ADR-042/044 (local frames)
**Amends:** PANTHEON_RULES A30 (model tiering) with the offline clause
**Owner directive (verbatim intent):** "out of everyone, the local LLM should
survive and reboot everyone else… when I'm offline the local LLM should take
over ALL LLM work."

## Context

The stack grew cloud-first: supervision ran through cloud `claude -p` ticks,
and A30 tiers judgment to frontier models without defining what happens when
frontier is unreachable. The 2026-07-23 kernel panic proved the inversion:
the local broker died at reboot and a **cloud** agent revived it. The owner's
ruling is that today's capability ordering (cloud smarter and cheaper) is
temporary; the architecture must already be shaped local-first.

Sovereignty is three layers, strongest at the bottom:

| Layer | Guarantee | Depends on | Status |
|---|---|---|---|
| **L0 — T0 floor** | The local broker survives reboot and crash unattended | launchd only | SHIPPED (plist re-ensure, PR #286; proven: 100s unattended revival) |
| **L1 — deterministic supervision** | The fabric heals itself and reports to the owner with **no LLM at all** | launchd + the Go supervisor | SHIPPED (PR #287: kickstart duty, heal collector, run report, API probe) |
| **L2 — offline takeover** | When the cloud API is unreachable, the local model takes over ALL LLM work | L0 + L1 | THIS ADR (design) |

The load-bearing insight from L1: *survival must not require any model.*
The local LLM is the fallback **brain**; launchd and deterministic Go are the
fallback **spine**. A design that made gemma responsible for its own revival
would repeat the inversion one layer down.

## Decision — L2 design

### Detection: the offline flip is a state machine, not a flap

The supervisor's existing per-pass Anthropic reachability probe (any HTTP
response = reachable, 5s bound) feeds a persisted state:

- `ONLINE → OFFLINE` after **3 consecutive** failed probes (~6 min at the
  2-min tick) — never on a single timeout.
- `OFFLINE → ONLINE` after **2 consecutive** successes, followed by queue
  drain (below).
- State + transition timestamps live beside the run report; every transition
  is a run-report line ("cloud unreachable — local AI taking over" /
  "cloud restored — queued judgment work released").

### Behavior in OFFLINE mode

The gemma-worker (already consuming `to: gemma` tasks) widens its intake to
**every routed lane**:

1. **Mechanical work** (triage, classification, summaries, drafting,
   formatting, status rollups): done locally, closed normally, marked
   `completed-offline` in provenance.
2. **Judgment work** (binding verdicts, PR review approvals, architecture
   decisions, anything ADR-041 gates): the local model **drafts** its best
   answer but the item is parked in a `judgment-queue` with the draft
   attached — **the local model never binds** (A30 amendment below). The
   owner can read drafts through the normal surfaces and may act personally.
3. **Heals**: unchanged — L1 owns them and needs no model.
4. Run reports continue every pass; the menubar "Last check" line carries
   the offline banner, so the owner always knows which brain is on duty.

### Recovery

On `OFFLINE → ONLINE`: parked judgment items re-enter their original lanes
with the local draft attached as context (a warm start, not wasted work);
`completed-offline` closures stand — cloud agents spot-audit rather than
redo. The transition is one run-report line, not a re-triage storm.

### A30 amendment (the offline clause)

> Generation down-tiers, judgment up-tiers, bind ALWAYS frontier — **and when
> no frontier model is reachable, the local model executes all mechanical
> lanes and drafts (never binds) judgment lanes, with binding deferred to
> frontier return or explicit owner action.**

Owner override stays absolute: the owner may bind anything personally at any
time; offline mode never gates the human.

### Constraints carried forward

- RAM guard (ADR-031) and graceful-bounce-only (ADR-040) are unchanged; an
  offline flip never forces a model load the guard refuses.
- The 16 GB fleet reserve holds; offline mode adds no resident processes —
  it re-points existing consumers.
- Model identity stays behind `~/.sirsi/gemma-model.conf` + resolver; the
  takeover logic never pins a model id (the triage-probe lesson, #285).

## Implementation phases (each its own reviewed PR)

1. **State machine + persistence** in the supervisor (Go, seam-tested):
   probe history, flip thresholds, run-report lines. No behavior change yet.
2. **Worker intake widening** behind the state: gemma-worker consumes all
   lanes when OFFLINE; `judgment-queue` parking + draft attachment.
3. **Recovery drain** + cloud spot-audit convention; conduit SKILL.md picks
   up the queue on its first post-restore run.
4. **Kill test**: block api.anthropic.com at the firewall for 30 minutes on
   a live queue; the acceptance proof is the run-report transcript showing
   takeover, local completions, parked judgment, and clean drain.

## Consequences

- The owner's absence (or an Anthropic outage) degrades quality, never
  liveness: everything mechanical continues, judgment queues with drafts.
- The direction of travel is explicit: as local models close the capability
  gap, lanes migrate from "judgment" to "mechanical" by reclassification —
  a config change, not an architecture change. That is the elevation path
  the owner named, pre-built.
