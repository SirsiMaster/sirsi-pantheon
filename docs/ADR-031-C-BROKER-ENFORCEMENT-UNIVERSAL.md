# ADR-031-C — Broker Enforcement Must Be Universal (No Bypass Paths)

**Status:** Accepted (both fixes shipped and verified live on the owner's machine, 2026-07-03)
**Amends:** ADR-031-A (never exhaust the host — the invariant + 4 layers), ADR-031-B (the numbers become node-dynamic)
**Custodian:** 𓁢 Pantheon (runtime) · 🌊 Hapi (resource governance) · binding review claude-home
**Refs:** ADR-031-A, ADR-031-B; `docs/case-studies/2026-06-18-pantheon-did-not-prevent-oom.md` (§6 addendum, same commit); PANTHEON_RULES.md A1, A5; the 2026-07-03 owner observation *"this situation is exactly what the pantheon and router are supposed to prevent and then remedy."*

---

## Context

On 2026-07-03 the owner reported the machine repeatedly "churning and effectively shutting down"
while running `codex`. Independent diagnosis (claude-home) found the host at 96% swap and ~1 GB
free RAM out of 51.5 GB — a symptom nearly identical to the 2026-06-18 incident ADR-031-A/B exist
to prevent. Codex itself turned out to be a mostly cloud-API-driven CLI; it was not the actual RAM
driver, just what the owner happened to be running when the machine tipped over.

This was **not** a failure of ADR-031-A/B's design. `guard.NodeCapacity.Fits()`, `DynamicReserve()`
(which already accounts for live Claude/Codex RSS — exactly the cross-agent protection ADR-031-A
promised), the 2×model serial budget, the cold-path file lock, and Hapi's governed-PID
suspend/kill ladder were all read from `origin/main` and confirmed correct and present. The gate
works. **Two pieces of local automation were never wired to call through it:**

1. **`~/.local/bin/sirsi-gemma-worker.sh`** — the LaunchAgent-run daemon that completes every
   router-item triage/classification task (`ai.sirsi.gemma-worker`) — called `mlx_lm.generate`
   directly on its default ("fleet-safe" 12B) path. No RAM check, no file lock, no warm-server
   preference. It had its own bespoke `free_gb()` heuristic, but that only gated the opt-in 31B
   "MODEL: max" path — the path that runs on *every single triage ping* had zero admission
   control and never touched the warm broker at all.
2. **The `ai.sirsi.gemma` LaunchAgent itself** — the plist that starts the "warm server" invoked
   raw `mlx_lm.server` directly (`ProgramArguments` = `mlx_lm.server --model ... --port 11434`).
   `git grep -n "ai.sirsi.gemma" -- '*.go'` returns zero hits in the entire codebase. It predates
   `sirsi gemma serve` (which writes a port-state file `gemmaServerBase()` reads to detect an
   existing warm broker) and was never migrated onto it. Net effect: **the broker's own
   warm-server detection could never see the warm server that was actually running** — every
   caller fell to the cold path regardless, and the resident 12B model was serving no one.

Both gaps share one shape: **automation that predates or lives outside the broker's own call
surface, bypassing every layer of ADR-031-A/B by construction — not because a layer failed, but
because the layers only protect callers that go through them.** A gate with a door beside it is not
a gate.

## Decision — the invariant, extended

> **No process on this machine may invoke `mlx_lm.generate` or `mlx_lm.server` directly. Every
> local-model dispatch — daemon, LaunchAgent, script, or future caller — goes through `sirsi
> gemma` (cold, RAM-gated) or `sirsi gemma serve` (warm, RAM-gated at start). There is exactly
> ONE front door to local inference on this box, and it is the one with the gate on it.**

This is not a new mechanism — ADR-031-A/B's four layers are correct and stay exactly as they are.
This ADR closes the coverage gap: the invariant must hold for every *caller*, not just the ones
that were written after the gate existed.

## What shipped (verified live, not just claimed)

1. **`sirsi-gemma-worker.sh` fixed.** `gen()` now calls `sirsi gemma --model "$m" --max-tokens
   "$MAXTOK"` (prompt via stdin) instead of shelling `mlx_lm.generate`. Verified live: with the
   machine still at ~1 GB free, the new path correctly **refused** — *"not enough RAM to load
   Gemma cold (~11GB model + ~11GB dynamic reserve > 8GB free) — start the warm broker (`sirsi
   gemma serve`) or free memory. Refusing rather than OOM the machine"* — instead of blindly
   cold-loading the way the old code would have.
2. **`ai.sirsi.gemma` LaunchAgent retargeted.** `ProgramArguments` now runs `sirsi gemma serve
   --port 11434`; `KeepAlive` changed `true` → `false` (this command is a one-shot ensure-warm
   launcher — it forks a detached, Hapi-governed child in its own process group and exits; it is
   not itself a long-running process, so `KeepAlive` fought its actual lifecycle). `RunAtLoad`
   stays `true`. The old raw server was stopped cleanly via `launchctl unload` before the
   retarget. Verified live: `sirsi gemma serve --port 11434` (and again with `--concurrency 1`)
   correctly **refused to start** — *"Pantheon refuses to start the broker: a ~11 GB model + ~12
   GB dynamic reserve (OS + live agents + margin) > 34 GB free"* — the 2×model + DynamicReserve
   math from ADR-031-B's refuse-threshold decision, holding at the boundary exactly as designed.
   It will self-start the next time the box has genuine headroom (or the owner runs `sirsi gemma
   serve` manually once memory clears); it was **not** force-started past its own gate tonight.

Both refusals are the correct outcome, not a residual bug — proof the gate is now in the path it
was missing from, on both the cold and warm sides, at exactly the moment enforcement mattered.

## Regression guard (recommended, not yet built)

ADR-031-A/B each shipped a `Test*NeverAgainInvariants` guard against their specific regression
class. This ADR's equivalent does not yet exist: a CI/lint check (or a `sirsi doctor` finding) that
greps the repo **and** the known `~/.local/bin/` automation surface for direct `mlx_lm.generate`
/`mlx_lm.server` invocations outside `cmd/sirsi/gemma.go` / `cmd/sirsi/gemma_serve.go`, and fails
loud if one is found. Without this, a *third* bypass script is exactly as easy to write tomorrow as
the first two were. Tracked as a follow-up; not blocking this ADR's acceptance since both known
bypasses are fixed and verified.

## Consequences

- No change to ADR-031-A/B's mechanism — this is a coverage fix, not a design fix.
- Any future local-model integration (a new daemon, a new LaunchAgent, a new CLI tool) MUST route
  through `sirsi gemma`/`sirsi gemma serve`. This is now the binding contract, not a convention.
- The warm server currently sits stopped-until-headroom rather than forced-on — an honest state,
  not a regression. `sirsi gemma`'s cold path remains fully functional (RAM-gated) in the
  meantime, so triage/worker automation is not blocked, only slower per-call until the warm
  broker can safely start.
- Recommend a broader audit pass (claude-pantheon) of `~/.local/bin/` and any other repo's
  automation for the same bypass shape — this ADR only closes the two instances found in tonight's
  incident, not a guarantee no others exist.
