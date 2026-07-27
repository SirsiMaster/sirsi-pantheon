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

## 1. The inversion — Sirsi is a plugin pretending to be a platform

This is the thesis. Everything else in this document is subordinate to it.

Sirsi does not host an agent. It is hosted *by* one. The installer says so in its
own words — `internal/setup/surface.go:279`, the message shown when Claude Code is
absent:

```go
res.Message = "no `claude` CLI found — add to your IDE: " + MCPConfigSnippet()
```

Sirsi's remedy for "no AI present" is **to register itself inside someone else's
agent**. That is the inversion, stated in code.

Measured consequences:

| capability | state |
|---|---|
| provider abstraction (swappable Claude / Codex / Gemma / OpenAI-compatible) | **none in Go** |
| inference client | **none** — the only `api.anthropic.com` reference is a DNS reachability check in `guard/network.go` |
| agent loop owned by Sirsi | **none** — the loop lives in Claude Code |
| install-time discovery of IDEs and AI resources | **none** — install offers to insert Sirsi into your IDE instead |
| conversation surface | `Ask Sirsi` — chat only, **no tool-calling** |

The intent already exists and is correct. `sirsi-brain.sh` opens:

> *"the PLUGGABLE orchestration brain for Sirsi Pantheon… It must be swappable
> painlessly — gemma is the zero-token DEFAULT, but a user can point it at Claude,
> Codex, Gemini, GLM, Kimi K2, or any OpenAI-compatible endpoint without changing
> anything else."*

That is the right design. It is also an **unversioned bash script in
`~/.local/bin`**, outside the repo, offering a single verb — `ask`: one prompt, one
answer. No tools. No loop. No conversation. No Go caller treats it as a provider.

### Why this obviates the local premise

If Sirsi is only useful when Claude Code is installed, then "local sovereignty" is
fiction: the product has taken a hard dependency on a cloud AI CLI in order to
deliver local independence. A developer with no AI in their IDE — the majority — gets
a status widget.

**Sirsi must make the IDE's AI moot.** Not integrate with it. Moot.

## 2. The acceptance test for the whole programme

> **Anything that can be done in Claude Code must be doable in Sirsi.
> If it can't be done in Sirsi, it can't be done.**

Concretely, this conversation is the test case. Today, through Claude Code, the
following happened: a machine-wide process census; an ancestry walk from a symptom
to a spawner; parsing a Jetsam report and *correctly discarding it* as the cause;
reading a heuristic out of a shipped application bundle to prove an error message was
a guess; a root-cause fix; a deploy; a live verification; and four rounds of the
owner challenging the conclusion and the conclusion changing twice.

**None of it was observable to Sirsi, and none of it is expressible in Sirsi today.**

That transcript is the Phase-4 acceptance bar. Not a demo of it — that exact class of
work, initiated and completed inside Sirsi, with a swappable model behind it.

## 2b. The recurring failure, stated numerically

The inversion is the thesis; this is the wound that proves the substrate is also
wrong. Two numbers, measured on this repo today:

| measurement | value |
|---|---|
| places that can spawn an OS process | **196** (`exec.Command`, 78 files) |
| places that enforce a memory or process budget | **0** |

There is no allocator. Every one of those 196 sites decides on its own that spawning
is fine. Individually reasonable; collectively a machine that periodically eats
itself.

| date | shape | scale |
|---|---|---|
| 2026-07-03/04 | runaway executor | **19,195 sessions spawned, 0 closed, 1.3 TB** orphaned |
| 2026-07-24 | leaked task-runner sessions | ~8/hr accretion, swap-death wedging gemma |
| 2026-07-27 | `thread discover` fork storm | **358 processes, 267 zombies, load 436, swap 48.5/49 GB** |

Ka and Anubis were created to clean up after this class. They are janitors hired
because the building has no doors.

**Hardware is not the lever** — M1/32 GB → M5/48 GB, failure unchanged, because
unbounded × bigger is still unbounded. Today's storm ate a 49 GB swapfile.

**The health surface cannot see it.** During the storm, with macOS displaying *"your
system has run out of application memory"*: `sirsi diagnose` → **🟢 Healthy,
100/100, 16 signals, "No immediate action required."** None of the 16 was *"is swap
exhausted"* or *"is a process tree multiplying."*

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

## 4. Architecture — six pillars

### Pillar 0 — Sirsi is the agent host

*The pillar that makes Sirsi a product instead of a plugin. Without it, nothing else
is worth building.*

Three parts, none optional.

**`internal/agent` — Sirsi owns the loop.** Not "Sirsi is called by a loop." The
conversation, the tool dispatch, the transcript, the verification and the turn state
live in Sirsi. This is the piece that currently exists only inside Claude Code.

**`internal/provider` — intelligence is swappable.** A single Go interface with an
honest capability model, because the backends are not equivalent:

```go
type Provider interface {
    Name() string
    Caps() Caps          // tool-calling? streaming? context window? cost class?
    Complete(ctx, Request) (Response, error)
}
```

Implementations, in order of build: **local (gemma / any OpenAI-compatible endpoint —
the zero-token default)**, **Anthropic API**, **OpenAI-compatible cloud**, and
**delegating CLIs** (`claude`, `codex`) for users who already have them.

This is `sirsi-brain.sh`'s stated intent, promoted from an unversioned bash script
into versioned Go with tools and a loop attached. The design was right; it was never
given a body.

Capability degradation must be explicit: a provider without tool-calling drives the
loop through a constrained decision tree instead, and the transcript says so. Silent
degradation is how a local-first product becomes a cloud product nobody noticed.

**Install inverts.** Today install detects Claude Code and offers to insert Sirsi
into it. It must instead:

1. **discover** — IDEs (VS Code, Xcode, JetBrains, Cursor), AI CLIs (`claude`,
   `codex`, `ollama`, LM Studio), API keys in env, local model runtimes;
2. **report** — "here is the intelligence available on this machine";
3. **route** — register each as a *provider* selectable inside Sirsi;
4. **offer, not require** — MCP registration into an IDE becomes an *export*, one
   option among several, rather than the fallback when Sirsi finds itself unwanted.

A machine with **no** AI resources must still yield a working Sirsi, on the local
model alone. That is the entire local premise, and it is testable: disconnect the
network, uninstall `claude`, and the diagnostic conversation must still work.

### Pillar 1 — Ma'at Governor: one allocator, admission control, backpressure

*The pillar that ends the recurring failure — and the one that makes the 16 GB target reachable.*

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

### Phase 0 — Sirsi can think without Claude Code (weeks)

`internal/provider` (local first, then Anthropic, then delegating CLIs) ·
`internal/agent` loop with tool dispatch · install-time discovery and routing ·
`sirsi ask` becomes a conversation with tools, not a chat box.

**Bar:** with the network disconnected and the `claude` CLI removed from `PATH`, ask
Sirsi *"why is my machine slow?"* and get a correct causal answer with a named
remedy. If that fails, the local premise is fiction and nothing later matters.

### Phase 0b — Instrument the truth (days)
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
- **Not an MCP server with better manners.** The current shape is Sirsi-inside-your-agent.
  The product is agent-inside-Sirsi. These are opposite directions, not degrees.
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

1. **Ratify the inversion.** Sirsi hosts the agent; models are swappable backends;
   IDEs and AI CLIs are discovered resources routed *through* Sirsi. MCP-into-your-IDE
   becomes an export option, never the fallback.
2. **Ratify the 16 GB target.** Everything downstream depends on it.
3. **Phase order** — Agent Host (Phase 0) before everything is non-negotiable if the
   local premise is real; Governor before Loop thereafter is the recommendation (the Loop needs
   actuators the Governor makes safe). Reversing them is defensible if a
   demonstrable diagnostic conversation matters more than stability.
4. **Autonomy posture** for the repair tier: confirm-each, confirm-once-per-class, or
   fully manual.
5. **The gemma broker cap** — currently 20.8 GB on a 48 GB box, 130% of the target
   machine. Interim fix regardless of this plan's fate.
