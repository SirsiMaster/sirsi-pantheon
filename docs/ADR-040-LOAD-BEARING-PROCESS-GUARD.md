# ADR-040 — Load-Bearing Process Guard (don't SIGKILL what you should re-size)

**Status:** Accepted
**Date:** 2026-07-14
**Deciders:** claude-home (conduit), owner (Cylton)
**Supersedes / relates:** Generalizes Rule A5 (Hapi GPU inference protection) to the RAM slayer; relates to ADR-009 (injectable providers), ADR-033 (honest fixes / real levers), ADR-031-A/B/C (broker RAM governance).

## Context

A live incident (2026-07-14) exposed a class of unsafe reclaim. The owner asked the tool to "reclaim RAM" from a process holding **25.8 GB (53% of a 48 GB machine)**, ranked #1 by RSS and shown only as a nameless `Python`. A prior session had concluded it was "a stuck/leaked MLX process holding a model resident without serving" and was about to kill it.

Verifying the full argv (which every RSS-sorted view had truncated) revealed the truth:

```
~/.sirsi/gemma-capped-server.py 27204354048 --model mlx-community/gemma-4-12B-it-8bit --port 11434 …
```

It was **not a leak**. It was the launchd-managed local model server, actively serving (`/health` OK, `R` state, ~19% CPU), holding the model resident **by design** (the `27204354048` = 25.3 GB RAM cap it was told to reserve). Killing it would have:

1. **Reclaimed nothing durable** — `PPID=1` + a `KeepAlive` LaunchAgent means launchd respawns it in seconds and reloads the 12B model, trading resident RAM for a burst of pageins + a cold reload.
2. **Severed a live capability** — the local-inference server the fleet's zero-token triage depends on.
3. **Acted on a misdiagnosis** — "resident without serving" was false; it *was* serving.

Two root gaps in `internal/guard`:

- **Acquisition truncated identity.** `getProcessListWith` ran `ps -axo …,comm`, capturing only the executable path (`.../Python`), never the argv — so the tool was *structurally blind* to what the process actually was.
- **The kill gate had no "load-bearing" concept.** `isProtectedProcessWith` protected by system name (`launchd`, `kernel_task`, …) and `PID<=1`. A launchd *daemon* named `Python`/`bash` with `PID>1` sailed through — and the orphan-kill path (`KillTrueOrphans`) is *worse*, since a launchd daemon legitimately has `PPID=1` and looks like a true orphan.

Rule A5 already encodes exactly the right instinct — *"MUST NOT kill GPU processes that are actively training or inferencing"* — but only for GPU processes in Hapi. The RAM slayer had no equivalent.

## Decision

Generalize A5's "don't kill what's actively serving" from GPU to the RAM slayer, in three additive parts (Rule A15 — no working path removed):

1. **See the full identity.** `getProcessListWith` now captures `command` (full argv), not `comm`. `Name` is the basename of the executable token only (never a `/` inside a `--model path/arg`); `Command` is the full argv. This strictly improves `classifyProcess` too (it can now match on args).

2. **A load-bearing gate** (`internal/guard/loadbearing.go`). `isLoadBearingWith(p, proc)` classifies a process as a local model/inference server from its argv — an explicit signature list (`gemma-capped-server`, `ollama`, `llama-server`, `mlx_lm`, `vllm`, …) plus a generic heuristic (serves a port/`serve` **and** references a model: `--model`/`.gguf`/`.safetensors`/`mlx`). Loopback binding is deliberately *not* a decision input, so a plain dev server on `127.0.0.1` stays killable. When the audit's `Command` is a bare interpreter, `fullCommand` fetches the argv on demand (`ps -o command=`) — "know what you'd kill before you kill it," made mechanical. The returned reason names the **real lever**: lower the RAM cap, run a smaller/more-quantized model, or evict-when-idle — never SIGKILL.

3. **One chokepoint, both paths.** `isProtectedProcessWith` consults the gate, so both `SlayWith` *and* `KillTrueOrphans` are protected in one place. `SlayResult.Protected []ProtectedProcess` (additive) carries the spared PID + reason; the dashboard `/api/guard/slay` response surfaces it so the menubar explains *why* a kill was declined and points at the lever.

All system side effects stay behind the injectable `platform.Platform` (Rule A16), so the gate is fully unit-testable and was proven live against the running server.

## Consequences

**Positive.** A launchd-managed, actively-serving local model server can no longer be silently slain to "reclaim RAM." The user sees the true identity and a real remediation lever instead of a futile kill. The discipline is enforced by tests (Rule A17), not left to an operator remembering to check argv.

**Negative / trade-offs.** The generic heuristic can over-protect a non-model process that both serves a port and references a `.gguf`/`--model` token — but over-protecting (user kills it manually) is the safe failure direction; the alternative (false-kill of a load-bearing server) is what this ADR exists to prevent. The extra on-demand `ps -o command=` fires only for bare-interpreter targets that reach the gate, not for every process.

**Follow-ups.** (a) Wire the same load-bearing reason into the TUI Health/Waste screens' reclaim flow. (b) Consider a `sirsi guard --resize <server>` lever that edits the LaunchAgent RAM cap — the constructive counterpart to the declined kill. (c) Teach the Hapi/broker path to read the same signatures so sizing advice is consistent across GPU and RAM.

Refs: PANTHEON_RULES.md A5, A12, A15, A16, A17; ADR-009, ADR-031-A/B/C, ADR-033; forensic origin 2026-07-14 (gemma-capped-server.py PID 1210).
