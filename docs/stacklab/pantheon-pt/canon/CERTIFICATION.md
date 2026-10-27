# Pantheon wing and release certification

Owner: Codex Pantheon (source/canon), SSA (independent review, release and selected
runtime adoption), Ra (fabric and registry coordination). Reviewed source base:
`e26e324be107ab541ec8d4d1400392e1ee55fc26`, 2026-10-27.
Target composition: **v0.24.141**, not yet released. Public140 remains the exact
historical baseline. This document defines full acceptance and records evidence;
it is not a certificate merely because the file exists.

## Binding criteria

Read the universal registry's `docs/LANE_CANON_REQUIREMENTS.md` and
`docs/PROCESS_DIAGRAM_RUBRIC.md` (SL-DIAGRAM-001), Pantheon ADR-066, the portfolio
manifesto's integrated definition of done, `.agents/completion.contract.json`,
the universal `canon/signing/SIRSI_SIGNING.md` and its registry, the universal
Ma'at load-filter self-interference guard, and component/recipe-specific contracts. The nine documents are PRD, PLAN,
USER_STORIES, SECURITY_THREAT_MODEL, DESIGN, RUNBOOK, CHANGELOG,
TRACEABILITY_MATRIX and COMPONENT_CATALOG. A shape check proves existence only.

No self-issued score, schema-valid wing, green component test or peer certificate
can certify the whole product. The whole process inventory is the denominator;
missing or stale coverage remains owned work. Canonical product authority is
versioned source and evidence, never a particular host or agent memory.

## Criterion-to-proof ledger

| ID / requirements | Acceptance | Actual evidence and scope | Owner / remaining action |
|---|---|---|---|
| PT-C01 / REQ11 | Origin/main wing, declared roster and byte-identical universal registry pin | Independent2026-10-27 GitHub readback: origin and pin SHA256 `27a7c95633b6124500d7e4e463b8c772c295c2c0b867f951ed2749a8e5d3e3e5`, same Git blob `d92b972ec832d659a0b490961e3c4cd07fa5754a`; roster includes PT; installed doctor returned no findings | Main/Ra: PASS for observed historical wing; any changed wing needs origin merge, repin and fresh readback |
| PT-C02 / REQ02,11 | Nine maintained records accurately cover current product | All nine files present; this corrective change expands previously skeletal product/stories/design/security/runbook/plan and marks source versus runtime evidence | Main: independent semantic review and normal origin integration required; existence alone is not PASS |
| PT-C03 / REQ08,11 | Complete component/dependency/upgrade and process-diagram coverage | COMPONENT_CATALOG, DIAGRAM_INDEX and detailed diagrams; recipes name sources, tests, inputs/outputs, writes and replacement constraints | Main: reconcile inventory denominator and every source branch/recovery with independent review; no runtime claim from diagram coverage |
| PT-C04 / REQ01,02,05 | Exact clean composed source, full Go/Swift correctness/security/resource qualification and Ma'at A17 assessment | PR1239 merged as f648; Ma'at PR1240 merged as e26. Exact component normal/race/causal receipts exist. Pending1241/1242/1243/1244 are not part of this base | Main/SSA: integrate remaining corrective source normally, then qualify final composition and bind exact commit/tree/logs; retain Ma'at assessment and honor A17 score85 stable-release threshold without treating a score as whole-product proof |
| PT-C05 / REQ03,10 | Same signed/notarized/stapled one-engine Python-free app in DMG and PKG | Public140 independent asset/signature/staple/cask readback exists; not proof for141 | SSA: final141 inventory, linkage/resources/modes, app/installer Team, notary/staple receipts and same-payload proof |
| PT-C06 / REQ02,03 | One exact tag/release and canonical cask with remote bytes | Public140 asset hashes below independently verified;141 tag/release/cask not created here | SSA: one supported publisher, paired remote asset SHA/size, tag/source join and exact canonical tap readback |
| PT-C07 / REQ04,09 | Supported install/upgrade/rollback/uninstall, loaded adoption and cleanup | Actual140 M5 root transaction completed with selected identity; old outcomes explicitly unknown_not_certified_drained. Old dashboard process remains independently identified | SSA/SHA: qualify141 supported isolated lifecycle, selected actual loaded UI/client adoption, original caller proofs and cleanup/debt; do not blindly restart fleet |
| PT-C08 / REQ01,05,07 | Complete, usable native/CLI/TUI/MCP/web paths with producer identity and truthful state | Component Swift/Go/UI proofs exist with explicit skips/limits; no final installed141 proof | Main/SSA: actual installed click/keyboard/accessibility, freshness/error/retry/cancel and same-engine checks; no dead-end/raw-shell handoff |
| PT-C09 / REQ06,09 | Ma'at default local assessment/search/classification and actionable safe resolution | G04 engine and native source qualified; recipe/catalog remain canonical. Component scan budgets do not prove JEV-relative superiority or all repairs | Main/SSA: final composition and installed baseline with optional LLM unavailable, original findings remediated/reconciled to terminal evidence |
| PT-C10 / REQ08 | Stack Lab recipe inspection and selection/run preserve exact authority | Existing local source catalog and native recipe source/tests; SNE construction remains upstream sovereign | Main: verify installed inspection, full component/resources/options, selection/preflight/qualified execution/result and unsupported-option recovery; no invented producer capability |
| PT-C11 / REQ11 | Repository, Owner Reading Room and native Workspace cross-linked and current | Repository is authority. This updated candidate has not yet been integrated or synchronized to human mirrors | SSA publication custodian: reuse stable existing IDs, record source hash/time and readback. Mark stale until actual synchronization, never assume current |
| PT-C12 / all | Independent integrated review, reproducible completion proof and truthful original task closure | Source reviews and individual receipts remain scoped; this file supplies no blanket acceptance | Main/SSA: review all applicable rows against final source and installed evidence; close satisfied originals only, keep owned next action for residuals |
| PT-C13 / REQ11 | Project wing and recorded GCP billing/cost ownership crosswalk | SSA is independently reconciling portfolio origin/pins and existing GCP project/billing mappings; no PT-specific final crosswalk receipt is asserted here | SSA: link exact canonical crosswalk and readback; distinguish billing association, storage cost sidecar, budget and paid-resource authority; create no new paid project/IAM/budget merely to fill a record |

`REQnn` refers to `PT-REQ-nn` in PRD. Detailed process/requirement mappings are
in DIAGRAM_INDEX and TRACEABILITY_MATRIX. Upstream inference/hardware qualification
is consumed by identity, not reissued by PT. Unknown is not clean; a named
remedy is not evidence that the remedy ran successfully.

## Immutable historical public140 proof

Public release: [v0.24.140](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.24.140),
source `b4d09aeb531a3e5a72fa56d205fa02644b708e5d`.

- DMG SHA256: `fe6350acf94bff2df6bb42de178c21c4d6fbad0664cb1a198bddc106830af0b3`.
- PKG SHA256: `3c99439e3bd9d4f77ee20dd1e02942403cee6eaa47aff812e15cfc1d4522ae24`.
- Independent readback receipt SHA256:
  `364631dabaed7f497e8180259775695ef50f5008b72a931097f8b92f02378764`.

Retained receipt location on the qualification machine:
`~/.sirsi/evidence/pantheon-freeze-corrective-20261023.1rud4wmz/public140-independent-download-readback-20261027/Main-independent-public140-terminal.json`.
Origin/pin/roster observations are in the sibling
`stacklab-full-canon-20261027/origin-pin-roster-readback.json`. These paths locate
evidence; they neither restrict product portability nor replace its content hash.

## Certification decision

**Wing identity: verified for the observed origin record. Full integrated141
certification: NOT YET ESTABLISHED.** Remaining rows are executable obligations,
not new features or owner approval requests. Update this record with exact final
receipts and independent verdict after the one corrective release/adoption;
never silently promote historical or component proof to whole-app completion.
