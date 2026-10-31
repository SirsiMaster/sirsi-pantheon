# Explicit standalone CPU admission

Historical6c slice: the successor in LOCAL_TIER1_AND_STACKLAB.md adds automatic
qualified Tier1 routing, immutable phases, bounded owned-group cleanup/recovery
and local Stack Lab. The frozen6c implementation was not qualified or accepted.

This finite corrective slice belongs to SSA's existing
`maat-local-first-admission-20261022` task and the owner-directed local-first
service-tier contract. It is not a shadow task, new ADR, reservation backend,
router database, offline session, or pod-delegation substitute.

`sirsi-maat-run --scope local --work <name> --timeout <seconds> -- <command>`
selects one foreground **same-user, same-machine cooperative CPU-floor job**.
It needs neither an enrolled hostname nor a Ra thread/router response. It
records the bounded intent and terminal result in the front door's existing
private `~/.sirsi/evidence/maat-runs` receipt directory. It never constructs the
native fabric client, rewrites router configuration or claims canonical delivery.

The CPU budget is one cooperative core, as in ordinary build admission. It is
not a kernel sandbox or permission to load a model, reserve another machine,
perform measurement/quiet work, own the whole host, use a shared rail, spend a
fleet budget or mutate remote tasks. Quiet/loaded regimes, larger CPU requests,
fabric executor registration and recurring reservations are refused in this
scope. Existing local service/health/Thoth/Ma'at commands retain their own guards.

The existing default **fabric** scope is unchanged. Network failure never selects
local scope. Shared resources continue through the configured canonical or
finitely delegated pod authority, with the existing failure/unknown/outbox rules.
ADR-120's12-hour delegation/cache is not independent offline qualification.

## Custody and lifecycle

A retained private regular singly-linked file in the existing execution evidence
namespace provides one same-user kernel lock. This file is not a second task or
reservation store. A competing local job is refused. No stale timestamp, cached
lease, heartbeat or timeout manufactures a grant. The lock descriptor is passed
to the owned child, so supervisor exit alone does not release a still-held child
lock. The normal path joins/cleans its newly created child group before closing
its descriptor. Cleanup uncertainty is retained and cannot be labelled success.
The cooperative scope does not claim hostile same-UID isolation or authority
over unrelated processes/users.

Local receipts explicitly say `authority: local-user-process-lock`,
`scope: local`, `fabric_delivery: not_requested`, current UID and local resource.
They do not invent a fabric reservation or executor thread. An inherited native
execution receipt is not forwarded as this child's authority. Local records are
execution evidence, not Ra acknowledgments or a fabricated Casebook decision.
Configured remote client values remain unchanged; a payload requesting remote
work must still obtain its normal remote authority.

## Authored proof and remaining original obligations

The whole-entry fixture mocks every process/network/fabric call, uses owned
temporary paths and a new/unmapped hostname, and verifies actual argument routing
and private terminal readback. Other fixtures cover strict scope refusals, default
fabric outage without fallback, real owned-file kernel-lock exclusion, exact
lock/receipt metadata, symlink/hardlink/FIFO/public-mode refusals, durable-intent
failure before spawn, timeout/debt and preserved nonzero child exit.

No author execution, installed/offline-machine proof, service or network action
is claimed. This is not whole Tier1/offline or all-Mac completion. The existing
rootless local liveness watcher and Ma'at memory paths remain separate. Actual
network-down installed qualification and SSA delivery custody remain open.

Stack Lab contracts already ship, but the inspected local catalogue still needs
a checkout, and its doctor reads the canonical remote registry. No existing
off-by-default enablement/model-source preference was found to reuse. This
candidate does not add a cosmetic toggle or claim a functional local/frontier/
self-hosted ingredient/evidence runtime. That original clause remains explicit
work under the same owner contract, beyond this narrow admission correction.
