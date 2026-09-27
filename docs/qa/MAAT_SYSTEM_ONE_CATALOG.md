# Ma'at System One Catalog

**Wing:** `stacklab.wing.maat`

**Product role:** Pantheon's guaranteed local intelligence layer
**Runtime requirement:** none beyond the Pantheon binary and local state

Ma'at System One is the non-LLM foundation that ships with every Pantheon
installation. It has one authority boundary: recorded Ma'at facts are appended
to the decision journal, then Casebook derives a read-only local view. An LLM
agent may read this view or add an independently evidenced decision, but it is
never required for search, classification, prioritisation, or evidence links.

## Canonical local flow

```
observe -> assess -> append Ma'at decision -> Casebook -> CLI / Horus API
```

`internal/maat` owns assessment semantics. `internal/maat/casebook` owns only
deterministic projection. The Casebook never writes a determination, starts a
job, reserves a resource, or silently turns a fact into authorization.

## Component recipe manifest

The machine-readable recipe is
[`contracts/stacklab/maat-system-one-recipe-v1.json`](../../contracts/stacklab/maat-system-one-recipe-v1.json).
It lists every source module, test suite, input, output, write boundary, and
safe independent upgrade recipe in this wing:

1. Ma'at core report and assessor interface
2. Canon linkage assessor
3. Coverage assessor and bounded cache
4. Pipeline/CI assessor and failure classifier
5. Pulse, proof, and platform integrity collectors
6. Append-only decision journal and report recorder
7. Deterministic local Casebook
8. Shared-machine and rail scheduler/conflict detector
9. CLI surfaces for audit, scheduling, and search
10. Horus API and visible caseboard
11. The Stack Lab wing and recipe contracts themselves

An upgrade begins at one manifest component, follows that component's listed
tests and boundary, then updates the receipt/canonical contract. Components do
not get silently co-upgraded because they share a deity name.

## Accounted source surfaces

| Surface | Recorded unit | Current writer | Casebook treatment |
| --- | --- | --- | --- |
| Quality audit | one decision per assessment plus report summary | `sirsi maat audit` via `maat.RecordReport` | `assessment` cases, with pass/warning/fail priority |
| Canon linkage | audit assessment with ADR/rule standard | `CanonAssessor` through audit report | `assessment` cases, evidence is the report generation |
| Coverage | audit assessment with threshold and remediation | `CoverageAssessor` through audit report | `assessment` cases, failed cases are urgent |
| Pipeline/CI | audit assessment when the pipeline assessor is selected | `PipelineAssessor` through a report | `governance` or `assessment` cases, never synthetic success |
| Stack Lab guard/review | decision with a receipt reference | Ma'at/approved caller through the decision journal | `governance` cases linked to the receipt |
| Shared host, rail, and device capacity | reservation grant/refusal/queue/release | `sirsi maat reserve` | `allocation` cases tied to resource and holder |
| Cede, handback, or counter-request | explicit router-derived decision record | authorized Ma'at producer | `allocation` cases; pending is high priority, never inferred as granted |
| Contention and incidents | explicit decision with evidence | approved Ma'at producer | `contention` cases; blocked/refused/failed is urgent |

Every row has the same required journal shape: time, host, kind, requester,
assessed facts, determination, explanation, and optional affected/resource/
evidence links. The evidence reference is preserved verbatim; Casebook does not
claim a local record is a remote attestation.

## Product contract

- **Free and local:** no external JEV service, telemetry, model endpoint, or
  account is necessary.
- **Fast by construction:** JSONL append/read plus deterministic Go projection;
  no embedding, subprocess, or payload execution to inspect the casebook.
- **Portable:** each installation owns its local Casebook. A device contributes
  only records it can evidence; the router remains the cross-device handoff
  transport, not a copied policy store.
- **Inspectable:** `sirsi maat casebook [text] [--kind ...] [--status ...]`
  and `GET /api/maat/casebook` expose the same projection.
- **Fail honest:** unavailable journal input returns an error/503. Unknown case
  status is rejected. Missing evidence stays missing; it is never invented.

## Stack Lab status

This catalog and `contracts/stacklab/maat-wing-v1.json` are source candidates
until merged to the owning repository's `origin/main` and byte-pinned by
`SirsiMaster/sirsi-stacklab`, per ADR-066. A local checkout or unpushed branch
is useful for review but is not a canonical wing record.
