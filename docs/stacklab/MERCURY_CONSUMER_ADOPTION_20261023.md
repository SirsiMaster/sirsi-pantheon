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
| Historical browser SVG, console and router-script repair | Restored as scoped source from cd379 into the existing console, GET-only shared reader and same-origin external assets | Current-source fixtures/security/visual/installed parity remain unqualified until Main runs them; no wholesale historical merge |
| Historical `MercuryView.swift` | Reuse only the retained vector geometry and presentation under the current canonical CLI/owned runner; do not copy the old process helper or replace adjacent native scopes | Native now reads dashboard/v1 and retains separately labelled Ra/transport sections; independent scene fidelity and installed parity remain open |
| Current native CPU/GPU/power/interface view | Native candidate consumes the canonical dashboard/v1 CPU-busy, calibrated GPU-work and projected link-rate fields; local monotonic aging withholds retained measurements and rates | Power is not exposed by dashboard/v1 and remains unknown; the broader native power/telemetry obligation in the original heterogeneous-compute PRD remains open. Full historical web/native scene parity is still open |
| October18 worker EPERM/socket limitation | Requalified, no longer a current blanket hold | Exact historical live read now succeeds; recovered socket/race fixtures pass |
| Full installed product and all enrolled machines | Not established here | Requires normal signed release, installed readback and explicit machine/freshness evidence |

## Data flow and ownership

Existing Mercury producer → bounded private dashboard/v1 GET → validated Go
snapshot and presentation → CLI or MCP. Errors return typed unknown/refused/stale
state. There is no write path to Mercury, Ra or an inference engine. Existing
native Mercury UI now reads that same CLI projection. Its adjacent Ra/admission
and hardware-transport receipts remain separate facts, never another Mercury
measurement generation. Web/native clocks only expire retained claims; they do
not sample a device, calculate gain, or fetch from a clock-render callback.

Producer schema and calibration belong to Mercury. Configured endpoint choice
belongs to the operator, never to MCP arguments. Ra owns fabric authority;
Ma'at admission governs builds/tests, not the measurement's truth. Pantheon owns
read-model composition. SSA alone composes and publishes through its normal
release train; this source restoration does not choose a version or a new gate.

## Implementation order and decisions

1. Recover and verify exact historical objects; requalify old socket/read holds.
2. Extract only current-compatible Go reader/projection and focused fixtures.
3. Add CLI/MCP adapters and closed read-only contracts.
4. Review/test, record Stack Lab and exact package inventory, return immutable
   commits to SSA's sole release train.
5. Restore the retained web/native scene without replacing the producer or
   adjacent native scopes; keep exact-source runtime, fidelity/accessibility
   and installed-product obligations open until their own proof exists.

| Question | Chosen | Rejected |
| --- | --- | --- |
| Reuse strategy | Exact scoped source extraction | Blind historical multi-commit merge |
| Measurement authority | Existing Mercury producer | A new Pantheon sampler or inferred calibration |
| Surface composition | CLI/MCP share one Go read | Independently computed units or freshness |
| Native compatibility | Preserve reviewed1190 Ra/transport scope and current bounded runner; reuse only retained vector/presentation code | Replace the workspace or runner with the older whole view to obtain an easier passing test |

## Known proof and remaining proof

Historical readback receipt `/private/tmp/pantheon-mercury-live-readback-20261023.json`
records a real bounded state-ok read with two nodes and three links. The later
historical socket/race receipt is Ma'at run `c22f91a1125a497292a94c601f65d93c`.
These resolve their exact old execution holds; neither is this new source
candidate's qualification. New source, installed CLI/MCP, web and native scene
proof must each identify their own exact object and scope.

Human mirrors: pending SSA integrated publication. This repository record does
not claim Reading Room or Workspace synchronization.
