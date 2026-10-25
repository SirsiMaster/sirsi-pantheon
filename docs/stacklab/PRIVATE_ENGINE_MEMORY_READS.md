# Periodic summary and explicit memory diagnostics

This corrective slice addresses PE-CURRENT-01/02 from the current semantic
review of the original private-engine summary/resource review obligation. It
does not recreate the retired HTTP engine, a shared cache, or a new service.

`sirsi vitals --summary --json` uses the existing capacity/free probes,
EffectivePressure and swap provider. The summary sampler never enumerates
processes, reads footprints/broker memory, sorts, or ranks. The existing keys
remain intact and `top` is `[]`. Native periodic fetchVitals requests this
selector. `--top 0` remains the explicit ranked command's “all sampled entries”
setting; it is not a summary selector.

The default CLI continues to use the governor's ranked memory sample, including
physical footprint and existing broker correction. The browser Host view has an
explicit **Inspect memory** action through GET `/api/vitals`, wired to the same
CLI report builder and ranked producer. Periodic stats/refresh never invokes
that producer. No shell execution, permission change, repair or process signal
is involved. The dashboard stays on its existing loopback listener and uses its
existing theme, button, table and responsive layout conventions.

Host diagnostics remain reachable without router data. An empty sample reports
that no ranked data was returned, not that the machine has no processes. Failed
reads keep prior data visibly marked Previous read; retry requires another
explicit action. Missing/null ranked data is unavailable, not zero. These are
sampled top processes, not an exhaustive process inventory. Explicit reads expose
the existing process name/PID/memory fields to the local dashboard, as requested;
they are not uploaded or retained in a new store.

Authored verification covers raw-probe refusal seams for summary, command field
parity, actual default ranked/browser producer identity, native argv/empty decode,
real HTTP handler refusal/empty/positive output, and production JS read/render/
click handling with inert producers. Source-only at handoff: no latency/CPU,
installed UI, historical August execution, or whole-engine qualification claim.
The 137 fonts, closed recipe inventory and other workspaces are unchanged.
