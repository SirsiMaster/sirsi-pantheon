# Sirsi v2 — from utility to application

**Status:** PROPOSED (2026-07-27) · Author: claude-pantheon · Owner decision required
**Supersedes in scope:** the menubar-as-product assumption
**Related:** ADR-035 (runaway executor), ADR-030 (menubar surface), ADR-031/045/048 (local models)

---

## 0. The sentence this document exists to make false

> *"I'm cosplaying Sirsi. It's really just Claude with Sirsi verbs. Not an app, not
> releasable, not even fully functional. Every day we close the same issues — in fact
> the same issue that caused me to create Ka and Anubis."* — owner, 2026-07-27

That is an accurate description of the current state, and the recurrence is not a
discipline failure. It is an architectural property. This document names the
property, and proposes the smallest architecture that removes it.

---

## 1. The recurring failure, stated numerically

Two numbers, measured on this repo today:

| measurement | value |
|---|---|
| places in the codebase that can spawn an OS process | **196** (`exec.Command`, 78 files) |
| places that enforce a memory or process budget | **0** |

There is no allocator. Every one of those 196 sites decides, on its own, that
spawning is fine. Individually each is reasonable. Collectively they are a machine
that periodically eats itself.

The incident history is the same failure wearing different clothes:

| date | shape | scale |
|---|---|---|
| 2026-07-03/04 | runaway executor | **19,195 sessions spawned, 0 closed, 1.3 TB** orphaned trees |
| 2026-07-24 | leaked task-runner sessions | ~8/hr accretion, swap-death wedging gemma |
| 2026-07-27 | `thread discover` fork storm | **358 processes, 267 zombies, load 436, swap 48.5/49 GB** |

Ka (ghost scanner) and Anubis (hygiene) were both created to clean up *after* this
class. They are janitors hired because the building has no doors. The correct fix is
doors.

**Hardware is not the lever.** The owner moved from an M1/32 GB to an M5/48 GB and
the failure recurred unchanged, because unbounded × bigger is still unbounded. The
2026-07-27 storm consumed a 49 GB swapfile — it would have consumed 96 GB.

**The health surface cannot see it.** During the 2026-07-27 storm, with macOS
displaying *"your system has run out of application memory"*:

```
$ sirsi diagnose
  Status     🟢 Healthy
  Health     100/100
  16 signals checked
  Priority   1. No immediate action required.
```

Sixteen signals, and not one of them was *"is swap nearly exhausted"* or *"is a
process tree multiplying."* This is the green-surface-over-a-dead-thing class —
Pantheon's most frequently recorded bug family — pointed at Pantheon itself.

---

## 2. The constraint that defines the product

> *"If Sirsi can't work and be performant on an M5 Mac with 48 GB, how can it help
> 99% of developers with lesser specs?"*

This is the correct product constraint and it should be binding:

> **Sirsi targets a 16 GB machine. Every feature is designed against that budget.
> The 48 GB machine is the development box, never the target.**

The consequence is not cosmetic. It means **the resource governor is the product**,
not a subsystem of it. An AI operations console that cannot govern its own footprint
has no claim to govern anyone's machine. Today Sirsi is the largest single consumer
on the owner's workstation (gemma broker measured at **12.49 GB**, capped at
**20.8 GB** — 43% of a 48 GB machine, and 130% of the target machine).

---

## 3. What Claude Code has that Sirsi does not

Today's incident was diagnosed and fixed entirely through Claude Code. Sirsi
observed none of it. The gap is exactly four capabilities:

| capability | Claude Code | Sirsi today |
|---|---|---|
| **Actuators** | arbitrary shell | ~16 fixed checks, a handful of fixed levers |
| **Reasoning loop** | hypothesise → test → **discard when wrong** → narrow | fixed checklist, single pass |
| **Causal chaining** | walked pid → ppid → spawner to find the source | reports leaf metrics, stops |
| **Conversation** | challenged 3×, re-examined 3×, corrected twice | emits a verdict; a verdict cannot be argued with |

Sirsi has three things Claude Code does not: **persistence** (it runs when nobody is
watching), **four surfaces**, and **a resident local model**.

The product is the union. Nothing else in this document matters more than that
sentence.

Note the reasoning loop's most important property is *discarding*. During today's
diagnosis, two confident hypotheses (Jetsam; the session reaper) were tested and
**falsified** before the real cause was found. A system that cannot be wrong out
loud cannot diagnose.

---

## 4. Architecture — five pillars

### Pillar 1 — Ma'at Governor: one allocator, admission control, backpressure

*The pillar that ends the recurring failure. Everything else is downstream.*

`internal/govern` becomes the only path to an OS process or a memory reservation.

**Admission control.** Before work begins, ask whether the machine can afford it:

```
govern.Admit(ctx, Request{
    Class:      ClassAgentSession,   // agent | model | build | probe | ui
    EstRSS:     512 * MB,
    Lineage:    parentToken,
}) → (Lease, error)
```

Denied when any of: RAM headroom (free+inactive) below floor · swap headroom below
floor · class quota exhausted · process count over ceiling · **lineage depth or
fan-out exceeded**.

The lineage check is what makes a fork storm *structurally impossible* rather than
merely detected: a process spawned under lease L inherits L's lineage, and a tree
cannot exceed configured depth/fan-out. Today's storm was depth-5 fan-out-N; it
would have been refused at the third generation.

**Backpressure, not denial-by-crash.** Over budget, work queues rather than
spawning. The fabric slows down; the machine stays alive. This is the single
behavioural difference between a tool that works on 16 GB and one that doesn't.

**Enforcement is mechanical.** A CI gate fails the build on any `exec.Command`
outside `internal/govern` — the same guard shape proven repeatedly this week
(`check-font-scaling.sh`, `TestRegistryWakeCoverage`,
`TestDiscoverNeverForksAWatcher`), pointed at the 196.

Migration is mechanical and reviewable: 196 sites, 78 files, in package batches
(`cmd/sirsi` 30, `internal/seba` 20, `internal/vitals` 15, `internal/setup` 15,
`internal/router` 13, `internal/platform` 10, tail ~93).

**Budget derivation** — from the machine, with an explicit floor profile:

| profile | RAM | agent sessions | model resident | build concurrency |
|---|---|---|---|---|
| floor | 16 GB | 2 | ≤6 GB (1 model) | 2 |
| standard | 32 GB | 4 | ≤12 GB | 4 |
| dev | 48 GB+ | 6 | ≤16 GB | 6 |

### Pillar 2 — The Loop: sensors, hypotheses, tools, verification

`internal/reason` — the diagnostic loop as a first-class subsystem, not a chat box.

```
Observe   typed sensor readings (never free text)
Hypothesise ranked set with priors, each with a discriminating test
Test      a tool call that can FALSIFY, not confirm
Narrow    discard falsified; re-rank
Act       permissioned, audited, reversible-by-default
Verify    assert the artifact, not the exit code
Explain   the transcript IS the output
```

**The honest split of labour.** A 12B local model will not invent this loop. It is
scaffolded in Go — the hypothesis registry, the tests, the tool schemas, the
verification. Gemma does what it is genuinely good at: ranking hypotheses against
evidence, reading logs and stack traces, and writing the explanation. That division
is why this is buildable now rather than aspirational.

**Seed the hypothesis registry from the incident record.** Every `reference_*` memory
in this fabric is a hypothesis with a known test — the SIGKILL/endpoint-security
heuristic, docker-healthy-while-JVM-dead, free-vs-available memory, orphaned-process
storms, footprint-vs-RSS. That corpus is a moat: it is diagnostic knowledge nobody
else has, already written down.

**Tools are typed and permissioned**, in three tiers:

| tier | examples | gate |
|---|---|---|
| observe | process census, swap, logs, jetsam reports, git state | none |
| repair | restart a broker, reap orphans, evict a model, shrink a cache | confirm, reversible |
| destructive | kill non-orphan, delete data, change system config | owner only, never autonomous |

The tier boundary is the ADR-035 autonomy question, answered once, for everything —
rather than re-litigated per feature.

### Pillar 3 — One conversation, four renderers

`internal/session` owns the conversation: turns, tool calls, results, verdicts,
persisted and resumable. TUI, menubar, CLI and app **subscribe to it**. They render;
they do not implement.

The test that this is real: start a diagnosis in the menubar, continue it in the CLI,
read the transcript in the app. Same session id, same history.

Today the four surfaces are four codebases with four notions of state. That is the
reason a fix lands in one and not the others.

### Pillar 4 — Gemma as a governed, visible faculty

Currently Gemma is plumbing. To learn its memory cap today required reading a
positional byte argument from a Python wrapper's command line
(`gemma-capped-server.py 22320611328`); to learn it was alive required reading
`~/.sirsi/gemma-server.port` and issuing a curl by hand. There is no model, RSS,
throughput, cap, or restart control in any surface.

`internal/localai` becomes a **model manager**:

- list / load / unload / switch models; per-model cap
- live RSS, tokens/sec, queue depth, context utilisation
- **governor-integrated**: under pressure the governor may evict the model, and the
  loop must degrade honestly ("local model evicted to protect the machine") rather
  than silently failing
- tool-calling — the capability that turns chat into the loop
- surfaced identically in all four renderers

Model eviction under pressure is the direct answer to today's 12.49 GB.

### Pillar 5 — The application shell

A real macOS application: a window, not a popover. Multi-pane, resizable,
keyboard-navigable, with a conversation as the primary surface and the deities
(Horus, Anubis, Ma'at, Thoth, Ra) as inspectable panes.

The menubar item survives as a **status affordance and a way in** — which is what a
menubar item should be. It stops being the product.

---

## 5. Phasing — each phase independently shippable, each with a falsifiable bar

### Phase 0 — Instrument the truth (days)
Sensors that would have caught today: swap headroom, process-count delta and fork
rate, lineage depth, footprint-vs-RSS divergence, per-class RSS.

**Bar:** replay 2026-07-27 from the process/jetsam record; `sirsi diagnose` must
report **critical**, name the fork storm, and identify the spawner. Same for the
2026-07-03/04 incident. *A health surface that scores 100/100 during a live OOM is
the specific thing being retired.*

### Phase 1 — The Governor (weeks)
`internal/govern`; migrate all 196 exec sites; CI gate on direct `exec.Command`;
lineage depth/fan-out limits; backpressure queue; budget profiles.

**Bar:** a deliberate fork-bomb test (a verb that tries to spawn 500 children) is
**refused at the lineage limit**, the machine stays responsive, and the refusal is
explained in the transcript. Run on a 16 GB VM, not the dev box.

### Phase 2 — The Loop (weeks)
`internal/reason`; hypothesis registry seeded from the `reference_*` corpus; typed
tool tiers; Gemma tool-calling; verification-by-artifact.

**Bar:** given the 2026-07-27 machine state, the loop reaches "fork storm in
`thread discover`, spawner is `registry-police → discover`" **and explicitly
discards Jetsam and the session reaper along the way**, with the discard visible in
the transcript.

### Phase 3 — One conversation (weeks)
`internal/session`; the four surfaces become renderers.

**Bar:** a diagnosis begun in the menubar is continued in the CLI and read in the
app — one session id, one transcript.

### Phase 4 — The application (weeks)
The window. Conversation-first. Deity panes. Model manager.

**Bar:** a developer on a 16 GB machine installs Sirsi, asks *"why is my machine
slow?"*, and gets a correct causal answer with a reversible remedy — without opening
an IDE or a terminal.

### Phase 5 — Autonomy, re-argued (owner-gated)
Only now does the quarantined worker question reopen, against the governor and the
tool tiers rather than against hope.

---

## 6. What this is not

- **Not more checks.** 16 signals scored 100/100 during an OOM. The problem is not
  check count; it is that nothing was governing, and nothing could reason.
- **Not a bigger menubar.** A popover cannot hold a diagnostic conversation.
- **Not "Gemma will figure it out."** The loop is scaffolded in Go. The model ranks,
  reads and explains. Anything else is wishing.
- **Not autonomy first.** Governor and tool tiers precede autonomy. ADR-035 exists
  because that order was once reversed.

## 7. Risks, stated plainly

| risk | mitigation |
|---|---|
| 196-site migration is large and touches everything | mechanical, per-package batches, CI gate makes regression impossible after each batch |
| a 12B local model under-performs at ranking | the loop degrades to a deterministic decision tree; the scaffold works without the model, just less well |
| the governor throttles real work and annoys the owner | budget profiles are explicit and adjustable; every refusal is explained, never silent |
| this is a large plan and the fabric has a habit of shipping surfaces before substrates | Phase 0's bar is a *replay of a real incident* — it cannot be satisfied by a nice screen |

## 8. Owner decisions required

1. **Ratify the 16 GB target.** Everything downstream depends on it.
2. **Phase order** — Governor before Loop is the recommendation (the Loop needs
   actuators the Governor makes safe). Reversing them is defensible if a
   demonstrable diagnostic conversation matters more than stability.
3. **Autonomy posture** for the repair tier: confirm-each, confirm-once-per-class, or
   fully manual.
4. **The gemma broker cap** — currently 20.8 GB on a 48 GB box, 130% of the target
   machine. Interim fix regardless of this plan's fate.
