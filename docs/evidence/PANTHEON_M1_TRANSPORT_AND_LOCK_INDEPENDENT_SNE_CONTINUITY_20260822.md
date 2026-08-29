# Pantheon M1 Transport and Lock-Independent SNE Continuity

Date: 2026-08-22

## Result

The M1 is reachable and its supervised SNE service remains operational while
the graphical console is locked. Pantheon must not classify an idle Tailscale
peer as unavailable from `Active=false` alone.

## Observed evidence

- Tailscale peer: `m1.taild727f7.ts.net` / `100.88.242.95`
- Tailscale ping: direct via `192.168.1.180:41641`, 7 ms
- TCP 22: reachable
- TCP 5900: reachable
- Authenticated SSH command execution: accepted
- Architecture: arm64 M1
- macOS: 26.6.2
- Power: AC, battery 100% charged
- Graphical console: locked
- SNE supervisor and `sned`: resident
- SNE readiness: API v0, status `ready`
- Model: `gemma-4-e2b-it-nvfp4-sne-v0`
- Execution: plain, NVFP4-4-g16 weights, BF16 cache/accumulator/output
- Manifest SHA-256: `1886efb8ec163b0ddf7c8797bfab59204bfb03147409ffff6312922be6f84def`
- Swap: exactly 0 MiB
- System memory free: 68%

## Required classification

Transport reachability, graphical lock state, agent-session state, and model
service readiness are separate dimensions:

1. Transport is reachable when a bounded authoritative probe succeeds, such as
   Tailscale ping, authenticated SSH, or the required TCP service port.
2. `Online=true` is supporting metadata. Idle `Active=false` or an empty
   `CurAddr` is not proof of unreachability.
3. GUI lock is reported separately and gates graphical performance timing, not
   SSH/control-plane availability.
4. SNE readiness is established by its local readiness and exact model identity
   contracts, not inferred from Tailscale activity.

## Claim boundary

This is lock-independent operational continuity evidence. It is not an M1
performance benchmark, ANE result, model-quality claim, or graphical-session
admission. No package, model, or runtime bytes were changed.

## Human-access publication

- Canonical authority: this repository document.
- Owner Reading Room: Markdown and HTML mirrors created.
- Native Google Workspace mirror: pending because the current Sirsi Seshat OAuth
  grant is intentionally Drive-readonly; this blocker is explicit rather than
  silently omitted.
