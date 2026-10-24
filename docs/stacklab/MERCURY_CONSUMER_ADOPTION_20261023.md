# Mercury consumer adoption and retained obligations

## Exact sources and scope

Source-increment baseline: `94cda6311803ccfe3e83c5dc16f7847dd9a156d2`,
including native workspace PR1190; this is not a claim about a later release
candidate's head. Recovered historical source:
`a6cae2add8b406784d07447077ce530842492dbe`, tree
`322cdb4394c796cf35e7f26bdea0e2cac210da58`; its later web-console repair is
`cd379651389ece5214536d251fb120c7cca0fd7d`, tree
`91aebd21485c74093832b20bc4d9fa81280b63ad`. Both bundles were hash-verified
and retained under `refs/pantheon-evidence/mercury-*`. The Go reader, projection,
scene and CLI are unchanged between those two historical heads. No whole
historical branch is cherry-picked.

| Historical requirement or surface | Current disposition | Evidence boundary |
| --- | --- | --- |
| Private versioned dashboard reader | Adopt core into `internal/mercury` | Freshly test actual current source; old fixture results are provenance only |
| Shared wire/compressed/gain, unknown/freshness semantics | Preserve Go projection | Source/unit proof does not qualify physical throughput |
| `sirsi mercury` | Adopt as thin current CLI, with shared configuration | Development and installed signed readbacks remain distinct |
| MCP access | Add zero-argument `mercury_status` over the same read model | No endpoint or control parameters; no second sampler/cache |
| Historical Go scene/card-edge geometry | Preserve as read-model data | Does not mean the current native UI renders that scene |
| Historical browser SVG, console and router-script repair | Retain, not imported in this increment | Dedicated current-web adoption/security/visual parity remains open |
| Historical `MercuryView.swift` | Not copied over current native1190 | Current `/api` hardware workspace remains intact; full versioned-dashboard scene parity remains open |
| Current native CPU/GPU/power/interface view | Native candidate consumes the canonical dashboard/v1 CPU-busy, calibrated GPU-work and projected link-rate fields; local monotonic aging withholds retained measurements and rates | Power is not exposed by dashboard/v1 and remains unknown; the broader native power/telemetry obligation in the original heterogeneous-compute PRD remains open. Full historical web/native scene parity is still open |
| October18 worker EPERM/socket limitation | Requalified, no longer a current blanket hold | Exact historical live read now succeeds; recovered socket/race fixtures pass |
| Full installed product and all enrolled machines | Not established here | Requires normal signed release, installed readback and explicit machine/freshness evidence |

## Data flow and ownership

Existing Mercury producer → bounded private dashboard/v1 GET → validated Go
snapshot and presentation → CLI or MCP. Errors return typed unknown/refused/stale
state. There is no write path to Mercury, Ra or an inference engine. Existing
native hardware UI continues to read its separately labelled producer endpoint.

Producer schema and calibration belong to Mercury. Configured endpoint choice
belongs to the operator, never to MCP arguments. Ra owns fabric authority;
Ma'at admission governs builds/tests, not the measurement's truth. Pantheon owns
read-model composition. SSA alone composes and publishes release0.24.124.

## Implementation order and decisions

1. Recover and verify exact historical objects; requalify old socket/read holds.
2. Extract only current-compatible Go reader/projection and focused fixtures.
3. Add CLI/MCP adapters and closed read-only contracts.
4. Review/test, record Stack Lab and exact package inventory, return immutable
   commits to SSA's sole release train.
5. Keep the separate web/native scene and installed-product obligations open;
   no source-only completion or silent feature retirement.

| Question | Chosen | Rejected |
| --- | --- | --- |
| Reuse strategy | Exact scoped source extraction | Blind historical multi-commit merge |
| Measurement authority | Existing Mercury producer | A new Pantheon sampler or inferred calibration |
| Surface composition | CLI/MCP share one Go read | Independently computed units or freshness |
| Native compatibility | Preserve reviewed1190 UI | Replace it with an older view to obtain an easier passing test |

## Known proof and remaining proof

Historical readback receipt `/private/tmp/pantheon-mercury-live-readback-20261023.json`
records a real bounded state-ok read with two nodes and three links. The later
historical socket/race receipt is Ma'at run `c22f91a1125a497292a94c601f65d93c`.
These resolve their exact old execution holds; neither is this new source
candidate's qualification. New source, installed CLI/MCP, web and native scene
proof must each identify their own exact object and scope.

Human mirrors: pending SSA integrated publication. This repository record does
not claim Reading Room or Workspace synchronization.
