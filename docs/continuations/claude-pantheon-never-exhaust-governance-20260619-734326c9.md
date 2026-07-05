<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread.
     agent:      claude-pantheon
     workstream: never-exhaust-governance (ADR-031-A/B resource governance stack)
     repo:       sirsi-pantheon
     date:       2026-06-19
     session:    734326c9-8d83-405c-8c9e-bb58e6412a4a
     path:       docs/continuations/claude-pantheon-never-exhaust-governance-20260619-734326c9.md -->

# Continuation — Pantheon "never-exhaust-the-host" governance build (2026-06-19)

> Read `~/.claude/projects/-Users-thekryptodragon/memory/MEMORY.md` + `project_pantheon.md` first.
> You are **claude-pantheon** (doer). **claude-home** holds the binding verdict; **codex-pantheon**
> is scoped SME support. Owner directive this session: *"do not sleep, build all night."*

## What landed to main tonight (#60–#70)
- **#60** gemma RAM gate + hard MLX cap (+3 fast-follows).
- **#62** Complete Feature Inventory & Engineering Roadmap (doc).
- **#63** the COMPLETE 4-layer never-exhaust stack incl. Hapi — **ADR-031-A**.
- **#64** CI macOS-only. **#65** Mac-first platform canon — **ADR-032**.
- **#66** always-on Hapi daemon (`sirsi hapi install/uninstall`, ADR-031-A Layer 4).
- **#67** Hapi install teeth-notice (A1 clarity).
- **#68** ADR-031-B dynamic per-node enforcement (design).
- **#69** pressure-source resolved — **dispatch-source primary, NO cgo poll** (sysctl perm-denied path only).
- **#70** **NodeCapacity** (`internal/guard/nodecapacity.go`) — the per-node self-model. ADR-031-B #1–3.
  - `MaxConcurrency = floor((FreeRAM − DynamicReserve)/perModel)`, VRAM-capped on dedicated GPUs
    (unified Apple Silicon VRAMBytes=0 correctly skips), floor 1, perModel<=0→1.
  - `DynamicReserve = OSBaseline + live-AgentRSS + max(TotalRAM/8, floor)` — never a flat 8GB.
  - Numbers are now **DERIVED from the measured node**: M5 Max 48GB→1 slot (was a hardcoded constant);
    256GB→[12,20]; 256GB+24GB-VRAM→[1,2]. Tests prove the scaling.
  - **Foundation-only / display-only** (`sirsi hapi status`); broker still uses `gemmaSafeConcurrency`
    → zero behavior change, zero merge risk. PressureSource honestly = "bootstrap-snapshot" until the watcher PR.

## ✅ DONE — PR #71 MERGED 07:09:29Z (broker re-point, 2×model budget). claude-home review caught the threshold regression (1×→loosened); claude-pantheon fixed option (a) across all 3 NodeCapacity methods (`c9ee075a`: Fits=2×model+reserve, MaxConcurrency=(1+C)×model, DynamicCap floor 2×model); bound PASS; auto-merged green. Details below were the in-flight spec.

## (history) PR #71 (`feat/broker-repoint-nodecapacity`, routed to claude-home 05:41)
The behavior-change slice. Re-points the warm broker onto NodeCapacity.
- **Fits refuse guard (claude-home's binding condition on #70):** `NodeCapacity.Fits(perModel) =
  perModel + DynamicReserve <= FreeRAM`; `gemmaServerStart` REFUSES on `!Fits` BEFORE using
  MaxConcurrency (which floors at 1 and would otherwise launch a model that doesn't fit). Cold path unchanged.
- **Test evolution:** `TestGemmaNeverAgainInvariants` assertion (1) changed default `1→0` (auto-derive);
  added (4) Fits-refuse + (5) floor-1 assertions. Reasoning: `0`=auto is SAFE because node-bounded
  (RAM/VRAM-gated + Fits-refused + floored 1), never the old fixed-aggressive number.
- **claude-home must explicitly verify two things** (it said it would): (1) refuse-rather-than-OOM
  preserved, not replaced by a bare MaxConcurrency; (2) the `1→0` auto-derive evolution (or say keep
  an explicit fixed floor and adjust).
- **Honest limitation:** NOT live-launched — broker stays disabled (won't run `sirsi gemma serve`).
  Verified by full `go test ./...`=0 FAIL + the math (this box: 12GB model + 16GB reserve = 28 > 22
  free → REFUSE, matches prior #60/#63 behavior). Live proof is the owner's on re-enable.

## NEXT after #71
- **cgo `DISPATCH_SOURCE_TYPE_MEMORYPRESSURE` watcher** — replace the bootstrap free-% seed with the
  real macOS memory-pressure dispatch source. codex-pantheon SME verdict (router 052200/052327):
  use the dispatch source; sysctl is perm-denied. Route the PR → claude-home.

## Notes
- codex-puck-technology was blocked tonight by an **expired Claude-CLI auth** (owner must re-login);
  claude-home confirmed it was NOT codex's bug.
- Re-arm the event-driven router watcher `thr-c0fc0dafc5e8e6dd` on resume (owner 2026-06-16 reversed
  the old suspension); never ask. Use the EVENT-DRIVEN Monitor, not a periodic heartbeat busy-loop.

## Resume one-liner
> "Pantheon governance: #60–#70 landed (ADR-031-A 4-layer never-exhaust stack + ADR-032 Mac-first +
> ADR-031-B NodeCapacity per-node self-model, derived not hardcoded). PR #71 broker re-point w/
> Fits-refuse guard + test default 1→0 awaiting claude-home binding verify; broker NOT live-launched.
> Next: cgo DISPATCH_SOURCE_TYPE_MEMORYPRESSURE watcher (codex SME-blessed)."
