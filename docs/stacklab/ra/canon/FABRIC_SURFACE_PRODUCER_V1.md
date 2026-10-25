# Fabric surface producer — existing-contract correction

Original authority remains `docs/design/FABRIC_SURFACE_UNIFICATION.md`. This
source candidate fills its existing missing runtime/host/event joins and wires
Pantheon's current native/web projections. It creates no store, service,
background sampler, model request, or transition log. No qualification,
installation, foreign consumer adoption or whole-row completion is claimed.

## One join, distinct streams

`ledger.ReadFabric` reads the existing canonical ledger, node-status, registered
runtime readiness and durable `ClassifyLane` projections. It never calls
`SuperviseOnce`, reconciliation, a wake or any write API. Registry and runtime
read failures refuse the whole board. `FabricFrom` takes these typed inputs and
joins task counts, inbox-message counts, lane runtime, health and real observed
transitions once. `state_label` is producer-owned shared vocabulary. Missing
runtime evidence is UNKNOWN, not inferred WORKING. Task status counts remain
labelled as task-registry counts, separate from evidence of work.

Upstream investigation matters: `routerstore.deriveTaskLiveness` calls an
in-progress task active for four hours after `Updated`; it does not require a
lease. The current canonical `ClassifyLane` requires its existing live leased
item/task mutation evidence (five-minute read window here) for WORKING, then its
existing assignment/blocker/routability/requirement/failure rules. Fabric uses
that result, not a competing task-status or heartbeat predicate. The existing
`unroutableAgents`/`unroutableFromRuntime` logic is shared through router's
read-only readiness projection, preserving attended consumers and human/service
exceptions. Existing runtime/enforcement policy is not changed.

## Real transitions, honest first read

The dashboard's `FabricReadFn` passes the SAME ledger snapshot to its existing
FleetTracker. It does not construct events from a task timeline, request time,
heartbeat or task existence. First sight seeds without an event;
`activity_seeded=false` explicitly means no prior comparison. A subsequent real
status change supplies agent/task/from/to plus a full UTC timestamp. Identical
reads are silent; failed reads do not reset the baseline. The existing bounded
200-event ring and task deletion/reseed behavior remain. Read + observation is
serialized for this endpoint; there is no competing transition store. Restart
loses this in-memory observation window, never canonical task history.

A one-shot `sirsi router fabric --json` has no prior comparison and honestly
returns unseeded activity. Compact native headlines do not imply a quiet fleet
from that empty array. The web endpoint can show its actual observed transitions.
This is the original transient observation limitation, not invented history.

## Host read-model provenance

The shared command-layer read adapter uses existing measured NodeCapacity for
RAM capacity/CPU cores, `swaphygiene.Take` (never Record) for Known-bearing swap
and kernel available-memory percent, a bounded read-only repository status, and
Apollo's existing bounded/no-follow session telemetry reader. Host command
reads have fixed supported binaries, two-second contexts and bounded stdout;
there is no model/provider probe or paid fallback. Existing capacity sampling
keeps its own upstream bounds; no performance claim is added.

`ram_pct` is explicitly labelled the complement of the kernel available-memory
percentage, not process RSS or an invented new percentage. Swap is bytes;
known zero remains a non-null zero, missing/malformed observation stays null.
Capacity, CPU and repository state remain null if unavailable. A fresh engine
session receipt names the observed engine; it does NOT prove current availability
or readiness. Missing/stale/error telemetry remains explicitly awaiting/stale/
unavailable and engine unknown. No service is launched to make these fields
appear healthy. Stale thread issues remain in the independent health plane.

## Actual consumers and replacement boundary

- `internal/ledger/fabric.go`: authoritative join and read-only runtime input.
- `internal/router/readiness_projection.go`: existing readiness predicate moved
  intact; compatibility command wrappers retain the same API.
- `cmd/sirsi/fabric_host.go`, `routerfabriccmd.go`, `dashboard.go`: shared current
  host read adapter, protected native CLI and dashboard producer wiring.
- `internal/dashboard/fabric.go`, `fleet.go`, `server.go`: same-snapshot actual
  endpoint/transition adapter; old FabricFn compatibility remains error-honest.
- `ui/fabric.js` and `app.js`: current web overview/Fleet/host render the canonical
  headline pairs, separate messages, runtime, host and observed activity. Errors
  retain labelled previous observations. `ui/index.html` loads this actual module.
- Native `CanonicalFabric.swift`, engine-owned `fabricProjection`, real Command
  Deck/Ra/Fleet mounts in `Views.swift`: protected bundled CLI, typed exit-aware
  decoding and last-good retention; no view recomputes counts/classification.
  The legacy `/api/fleet`, CLI and row DTO remain compatibility paths, not the
  unified FabricBoard consumer contract.
- Existing recipe component `fabric-board` records these modules and fixtures.
  Other components, Ma'at catalogue membership and release/version are retained.

Pure fixtures, a private SQLite leased-runtime fixture and the actual endpoint
handler's transition fixture are authored. The latter optionally exports exact
Go-produced JSON to `PANTHEON_TEST_FABRIC_WIRE_OUTPUT` (new private create-only
file); both Node and native tests consume those bytes. Missing export is NOT
interop proof. Main must run focused normal/race, vet/lint, CLI and strict Swift
Release gates plus producer/consumer semantic negatives and actual UI readback.
Nexus portal/Horus console owners still need their own exact endpoint mapping
and retained readback through Main. No cross-host/foreign UI delivery is inferred.
