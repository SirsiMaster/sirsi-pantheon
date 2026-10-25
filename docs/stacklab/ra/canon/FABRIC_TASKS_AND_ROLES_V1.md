# Fabric open-task detail and shared semantic roles

This is the remaining R1/R2 source slice of the original
`docs/design/FABRIC_SURFACE_UNIFICATION.md`, based on qualified ce8d9f4f.
It does not create a task store, service, authorization path or global policy.
Canonical `ledger.Snapshot/Agent.Tasks` is already read by the existing producer.
No reads of referenced files or URLs are performed by this projection.

## R1: actual rows and detail

`FabricBoard.tasks` carries at most64 open task rows per page. A row has exact
agent/task ID, bounded subject, recorded status/liveness/responsible party,
stage/test state, a blocker-presence fact, source timestamps and provenance.
An exact same-agent blocker task ID is retained when present in the same open
task set; arbitrary blocker prose is withheld. This does not resolve/reclassify
the dependency. Unknown detail is not an empty ledger or an inbox observation.

Provenance names the canonical task registry and recorded commissioning actor/
time. Charter/outline/result presence is explicit; content is not inspected or
claimed verified. Only bounded HTTPS github.com links without credentials,
query/fragment or ports are included, at most8 per row. Other references are
counted as withheld, not followed. This URL restriction is a disclosure policy,
not evidence of publication, authority or accessibility. Raw charter/outline,
phase/timeline bodies, private result references, leases and journal payloads
are never serialized as generic Task objects. Bounded operator-authored public
metadata is not a universal secret-redaction guarantee.

Rows sort by agent and task ID. `snapshot` is SHA-256 of the complete ordered
safe projection; collection time is excluded. A fresh default GET `/api/fabric`
returns page0. When `next_offset` is non-null, the existing endpoint accepts:

`GET /api/fabric?tasks_offset=64&tasks_snapshot=<exact64lowercasehex>`

Both fields are required, singular, exact-case and bounded. Offset must be a
canonical positive multiple of64. Invalid selectors refuse before authority
read. The producer recomputes from its current canonical read: changed public
rows yield409, never skipped/mixed rows. A first-page refresh obtains a fresh
snapshot. Total and next offset disclose a partial page; no128-row cap or
hidden tail is presented as a complete task list. Empty observed rows/total0
are distinct from unavailable detail. A legacy injected producer without its
snapshot cannot serve continuation and refuses rather than substituting page0.

The full web board renders these actual rows after lanes. Selecting a task opens
the existing inspector with exact clicked-board provenance, source time/build
and same-board registration facts. Inbox/router inspection stays separate.
Refresh/next-page errors retain the last observation with its error/age; an
explicit first-page refresh remains reachable. Request generation fences late
page results from overwriting newer selections. Closing returns focus to the
same task control when still present, otherwise the main content.

## R2: existing semantic tokens

The Go producer supplies headline/lane/task role names and dark/light swatches
from `internal/brand` (ADR-038). Finished/working uses `ok`; in-progress/assigned
uses `gold`; stalled/blocked/idle uses `warn`; messages use `ink2`; unavailable
runtime uses `dim`. These are the current existing brand tokens requested for
this correction, not new raw hexadecimal exceptions or a replacement theme.
Lane roles come from the existing canonical runtime classification, never a
task status, heartbeat or locally summed count.

The web uses its existing generated CSS tokens. Native consumes the same
producer palette for the current color scheme. Unknown/missing roles or malformed
swatches remain neutral; old wire data does not invent WORKING/healthy color.
Original labels and task/message/host planes remain intact.

## Authored proof and remaining scope

Pure Go cases cover130 rows across three pages, a changed tail, private-field
canaries, malformed identity/offsets, known-empty/unavailable, canonical roles
and actual HTTP selectors including409. Failed continuation must not advance
the transition baseline. Node cases exercise real renderer/app handlers, task
detail, continuation/refusal/restart, escaping and late-result fencing. Native
cases decode the actual Go endpoint fixture and use producer role/palette data.
Existing ce8 runtime/health/activity/Back/scroll tests remain, not replaced.

All new source and fixtures are unexecuted by the author. Main owns normal/race,
Go→Node/native wire, strict Swift and served UI qualification after resource
coordination. ce8's prior finite proof is not proof of these new code paths.

R3 normal release/installed adoption remains open. R4 remains open: current
Nexus `routerboard/poll.go` independently computes ledger counts, a separate8734
surface remains, and its retirement is conditional. A completion reply alone
does not establish same-wire convergence or retirement. Main has requested the
existing owner correction; this branch changes no Nexus source or host/route.
