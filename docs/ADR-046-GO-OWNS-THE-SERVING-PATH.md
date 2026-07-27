# ADR-046 — Go owns the serving path; Python becomes a called extension

- **Status:** Proposed
- **Date:** 2026-07-27
- **Author:** claude-nexus
- **Owner directive:** 2026-07-25 — *"Nexus has to rewrite or update MLX to fully GO. Python is
  bloated and slow where Go is quick. we can add python extensions and get the same benefit."*
- **Supersedes in part:** ADR-045 (the exec-in-place pid model, below)

## Context

`sirsi gemma serve` is already a Go verb, so it is tempting to call the serving path
"mostly Go." Reading it says otherwise. The Go code computes a memory budget, writes a
Python source string, and then hands the machine to CPython. Everything load-bearing
after that moment is Python:

| Concern | Today | Is it inference? |
|---|---|---|
| Memory ceiling | `mx.set_memory_limit` in the generated shim | No — supervision |
| Port-free wait before bind | 30s probe loop in the shim | No — supervision |
| No-zombie exit guarantee | `os._exit` in the shim | No — supervision |
| Tokenize / decode | `mlx_lm.server` | **Yes** |

Three of the four are supervision, and supervision is exactly what Go is good at and
CPython is bad at. Only the fourth is a genuine Python-only capability.

Two of these are not merely misplaced, they are **known-defective in their current form**:

1. **The memory ceiling is advisory, not a ceiling.** `mx.set_memory_limit` asks MLX to
   prefer staying under a number; it does not make allocation fail. On 2026-07-26 the
   Python side reached ~31 GB and the kernel's Jetsam killed processes across the
   machine. A cap that cannot refuse is not a cap.
2. **The resolver budgets on-disk bytes while real RSS runs ~2.5×.** The estimate that
   feeds the cap is measuring the wrong quantity, so even an enforced cap would be set
   from a wrong number.

## The blocker this ADR exists to decide

In launchd mode the Go process calls `syscall.Exec`, replacing itself with CPython.
ADR-045 chose this deliberately and for a good reason: the pid launchd supervises then
IS the serving process, so the pid file stays truthful for `--stop`, the load-bearing
guard (ADR-040) and Hapi governance (ADR-031-A).

But it has a consequence ADR-045 did not need to weigh and this directive does: **after
the exec there is no Go left on the machine.** Go cannot enforce a ceiling, cannot wait
for a port, cannot guarantee a clean exit, and cannot observe RSS, because Go is gone.
Every proposal to "move X into Go" is blocked by this one line until it is resolved.

## Decision

**Go stays resident as the launchd-supervised parent and runs the MLX worker as a child
process.** Python is reduced to what only Python can do: load the model and decode.

The pid-truthfulness that motivated ADR-045 is preserved rather than sacrificed. The
supervised pid is the Go supervisor; it writes its own pid, and `--stop` kills the
process group, so `--stop` still stops the whole thing and the guard still sees a real
pid. What changes is that the supervised pid becomes something that can *act* when the
worker misbehaves, instead of being the misbehaving thing itself.

### Staged, not big-bang

The owner asked for stages, and this path is load-bearing for the whole fleet — the
local-LLM sovereignty rule means an outage here takes all LLM work down with it.

- **S1 (this ADR).** Record the pid-model reversal. No behaviour change.
- **S2 — supervision moves to Go.** Port-free wait, process-group lifecycle, and clean
  exit become Go. The generated Python shim shrinks to a `runpy` call. Lowest risk:
  each item is a straight translation with no inference involved.
- **S3 — a real memory ceiling.** A Go watchdog samples the worker's actual RSS and
  terminates it before Jetsam can act, replacing an advisory request with an enforced
  one. Pairs with fixing the resolver to budget RSS rather than on-disk bytes; an
  enforced ceiling computed from the wrong number is a new failure mode, not a fix, so
  **S3 does not land without the resolver correction**.
- **S4 — Go owns the HTTP surface.** Go holds the listener and admission control;
  the Python worker becomes an internal decode extension behind it. This is where the
  owner's latency argument is actually collected, which is why it comes after the
  safety work rather than before it.

### Non-goals

Reimplementing MLX kernels or the tokenizer in Go. The owner pre-accepted the
Python-extension shape for genuine Python-only capability, and kernels are the clearest
case of it. "Fully Go" is a statement about who owns the *serving path*, not a demand to
rewrite Apple's inference stack.

## Consequences

**Good.** The memory ceiling becomes enforceable, which is the only one of these items
that has already cost a machine. Supervision lands in the language chosen for it.
Crashes become attributable to a supervisor that outlives them.

**Costs, stated plainly.** One more process in the tree. ADR-045's pid reasoning has to
be re-verified against `--stop`, ADR-040 and Hapi rather than assumed — S2 is not done
until that is demonstrated, not argued. And a supervisor is itself code that can fail:
if it exits and leaves an orphan worker, we have traded one failure mode for another,
so the supervisor must die *with* its child, not before it.

## The pid-file contract (added after independent review)

The HyperGraph custodian reviewed this ADR and found a concrete breakage that the
decision above would otherwise have shipped silently. It is recorded here rather than
fixed quietly, because the failure it describes is the interesting part.

**What breaks.** A governed conduit step runs every 15 minutes and verifies that the KV
bound is actually applied, by reading the pid file and inspecting that process's argv
for `--prompt-cache-bytes`. Under ADR-046 the pid file names the *supervisor*, and the
supervisor does not carry that flag — the worker does. The check therefore flips to a
**permanent false negative**: it would conclude "broker unbounded" on a correctly
bounded broker, and bounce a healthy load-bearing server on every run.

**Why this is worth writing down rather than patching.** This is the ADR-040 hazard in
its exact shape — acting on a pid whose argv does not describe what the reader thinks it
describes. ADR-045 said the supervised pid *is* the serving process, and treated that as
a fact about `--stop`. It was also load-bearing for **readers**, and nobody wrote that
down, so a reader-facing contract survived only as an implicit property of the
implementation. When the implementation changed, the contract broke with no test to
catch it.

**Decision.** The pid file at `~/.sirsi/gemma.pid` names the **supervisor**, and that is
now a stated contract rather than an emergent property.

**S2 additionally ships a documented surface for readers**, so that nothing has to grep
argv to learn the broker's state:

- a worker pid at `~/.sirsi/gemma-worker.pid`, and
- `sirsi gemma status`, reporting supervisor pid, worker pid, and the **effective** cache
  bound as the worker actually applied it.

Readers assert against that surface. Grepping another process's argv is the fragile
thing we are replacing, not a pattern to preserve — an effective bound reported by the
process enforcing it is strictly better evidence than a flag someone passed to it.

**S2 is not complete until the conduit check is migrated** to the new surface. A
supervisor that lands before its readers are migrated is exactly the silent breakage
this section exists to prevent.

## S3's real blocker, narrowed by measurement

Live figures from the reviewer's sweep: broker RSS **12.52 GB** for
`gemma-4-12B-it-8bit`, KV cache **2.17 GB** against a 4 GiB bound. The bound is holding.

That narrows S3 usefully and contradicts the obvious reading of the 2026-07-26 Jetsam:
MLX is not misbehaving, and the watchdog is not needed to catch a runaway cache. The
defect is the **input number** — the resolver budgets on-disk bytes while real RSS runs
~2.5×, so the ceiling is computed from the wrong quantity. That work is already routed
to claude-pantheon as a P0 (router item `20260726-235904`). It and S3's gate are the
same blocker, so they are coordinated rather than re-derived.
