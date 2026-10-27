# Fabric metric drill-downs

Classification: platform-foundation. Owner-requested correction to the existing
Ra web surface, not a new router, task store, identity or authorization system.

## Operator workflow and value

The operator must be able to select a headline count and see every task that
contributes to it, including the tail after the first64 records. This prevents
blocked or stalled work disappearing behind a passive count or a partial page.
Ra owns the query; Pantheon and Nexus consume the same authoritative projection.
No paid-tier or general release-readiness claim is added by this correction.

Every count in the four existing summary blocks is a separate native button:
completed, open, in-progress, assigned-not-started, stalled, blocked, and pending
inbox messages. Messages navigate to the separate inbox stream, never task rows.
Task counts select the complete captured ledger before paging. Each explicit
selection and next-page action focuses and scrolls the resulting heading;
background refresh does not move the operator's focus. Lane name, state and
count open their captured lane inspector. Numeric headers and cells share the
same alignment; narrow layouts retain explicit data labels.

## Read contract

First page: `GET /api/fabric?tasks_filter=blocked`.

Continuation: `GET /api/fabric?tasks_filter=blocked&tasks_offset=64&tasks_snapshot=<digest>`.

Closed selections are `open`, `done`, `in_progress`, `assigned_not_started`,
`stalled` and `blocked`. Their definitions exactly match `FabricWork` facts.
Assigned/stalled are overlapping classifications; do not add them together to
invent a new total. A blocker reference alone is not status=blocked. Completed
rows are available only through their explicit completed selection. The
unfiltered contract and its existing snapshot domain remain unchanged.

Filter identity and the complete ordered safe row projection bind each
continuation digest. A different selection, changed tail or wrong cursor
refuses; no page-local filter, truncation or silent restart is permitted.
Malformed/duplicate/unknown query fields refuse before authority read. Producer
implementations without complete filtered selection support return503 rather
than disguising the current64 rows as a complete answer. Cursor validation
occurs before committing transition history, preserving changes for a fresh
observation after a409.

The response discloses selected filter, selected total, offset, limit and next
offset. The browser checks the selected total against its headline and checks
each row against the selected bucket. Empty and unavailable remain distinct.
Errors retain labeled last-good data and the existing explicit refresh action;
late responses cannot replace a newer selection. Public row/provenance limits
are unchanged: no raw charter, phase, journal, lease, credential, local-file
reference or arbitrary remote-link fetching is introduced.

## Verification and operational boundary

Affected full Go packages `internal/ledger` and `internal/dashboard` passed
normal and race gates under native run d379f040889b4281beb74f11b2bbe3d6.
Production app/renderer tests consume the actual Go-produced wire JSON. The
private old-selection overlay fails at the intended full-authority assertion;
it is not a compile-error negative control. The first native attempt caught a
real state-color regression from adding an interactive wrapper; producer role
tokens are now on the visible state label. A later qualification recipe attempt
refused create-only wire output reuse between normal/race; both failed attempts
and logs remain retained, not recertified as passing.

Actual browser preview used the production embedded UI and handlers with350
explicit synthetic tasks, including70 blocked rows. The operator selected the
blocked count, reached rows65–70, and inspected desktop/narrow layouts. The
preview is opt-in, loopback-only, uses no canonical store or credentials,
provides no operational action adapter, and closes normally or after180seconds.
Its synthetic rows and development-build label are not production evidence.

The successor still needs exact source review, normal protected publication,
signed installation and selected loaded gateway readback. Do not hand-edit the
live application, rewrite released140 assets, or call the frontend complete
from these local controls. Existing guarded gateway rollout and rollback stay
in `RUNBOOK.md`; all previous originals and incident evidence are preserved.

## Human access

This versioned repository recipe is authoritative. Desktop and native Workspace
reading copies are not yet refreshed for this candidate. Their publication is
explicitly pending normal source publication; no silent three-home completion
is claimed.
