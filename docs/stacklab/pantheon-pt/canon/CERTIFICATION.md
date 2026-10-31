# Pantheon wing and release certification

Owner: Codex Pantheon (source/canon), SSA (independent review, release and selected
runtime adoption), Ra (fabric and registry coordination).
The finite84-group source canon was independently accepted at `f6760af1` and
normally merged as `24b4e50b3df08505c7669091eb9aae9142c8ca42` on 2026-10-27.
The previous retained source inventory bound historical candidate
`958db55b58ddbd6560ce5d0da8f0a6f56488f225`, tree
`587f652d166b6156c3d9611771c9461696d8ae18`: 101 packages/targets, 858 source
files, 860 test files and 226 supporting files (1,944 total). Those counts
describe that snapshot only; they are not the final inventory or a completeness
claim for the subsequent corrective source.


The refreshed `MODULE_INVENTORY.json` pins corrective code commit
`856eae5077616b612bf89cb23e78a1f13c44682e`, tree
`24d3c63c56ca345d22993c44de1dccfe841bee59`. It enumerates 103 Go packages
and one Swift target: 1,026 source/data files, 904 test files and 232 supporting
files (2,162 listed blobs). Source/data includes direct `go:embed` assets that
were absent from the previous inventory; the five dashboard assets already in
support sets remain listed once. Literal imports and declared embed paths are
a source view, not compiler-resolved dependencies or proof of packaged payload.
`internal/apollo/INSTALLED_APP.md` and other prose/proof records stay outside this
declared code/asset inventory and remain covered by their existing canon.

The added canonical Cask golden file is classified as a test dependency of
`internal/caskrelease`; it is not an embedded runtime payload. The current
corrective source bounds the Ra handoff sheet, preserves typed inactive-disposition
history progress without authorizing generic action replay, compares separate
installed engine copies through bounded stable-byte reads, and aligns forward app
and Cask platform eligibility. Historical Cask bytes through 0.24.153 stay canonical.

The historical public baseline below is **v0.24.153**, source
`39383ba044c09591b2342ccb76359a4cc1e576cf`, tree
`0609ca8c43a6894553e1bfd1ec00f1018550d82d`. SSA publication/install receipt
`~/.sirsi/evidence/pantheon-outage-20261030/SSA-pantheon153-publication-m5-install-20261031.md`
has SHA-256 `b5a8120c3f1f582fc0cb86d6014a6a197e1041678c0d57a669b5c9b86c24207d`.
At historical source `4c17dc3d`, Main's strict
Release build and 40 selected Swift tests passed; the corrected handoff geometry
passed its separate four-combination fixture. The independent SwiftUI Text/Button
oracle is unavailable in that test host, so its AX-control subtest is explicitly
UNVERIFIED/skipped. Actual updated-app Cancel/Send frame and cancellation checks
remain required. Focused selfupdate normal/race each passed 43 top-level and 25
subtests; inactive-disposition normal/race each passed 6 top-level and 18 subtests.
These bounded results do not replace full CI, signing, package or installed tests.

The current corrective snapshot restores per-workspace Forward navigation,
uses the independently validated service-root policy only for relay health readers,
and reports native version identity from the app's own bundle before application
startup. Main-actor callback ownership and readable inactive-disposition failures
remain bound to the existing launch, history and journal fences.

For exact source `856eae50`, the strict native build and focused run recorded
142 tests: 134 passed, eight skipped as UNVERIFIED, zero failures and no compiler
warnings. The skipped in-process accessibility checks are not a product pass.
The strict Release build passed separately, and its temporary own-bundle version
fixture passed the stamped response, missing-stamp refusal and malformed-command
refusal without starting application UI. Focused relay normal and race runs also
passed. External receipts under `~/.sirsi/evidence/pantheon-freeze-zero-ledger-20261030/`
bind the actual commands and terminals; this is bounded qualification, not whole-app
certification.

The current public baseline is **v0.24.154**, source
`03319bb1ca9a8e5ce678e76b2e23d20b9f09931c`, tree
`03e6cc1310a907cb25a786c1ea86b5f3596572c1`. SSA's sole publisher
`41236480321` completed, and Main verified the supported M5 installation, exact
loaded app identity, cask bytes, all 14 root navigation entries, the Ra composer
frames/Cancel and the Horus memory confirmation/Cancel. These installed154 results
do not certify the new corrective snapshot. Protected CI, its normal signed release
and actual updated-app checks remain required.

The metadata commit follows this code snapshot. Its exact final commit/tree and
every listed blob must be bound by the external final-byte comparison receipt;
the inventory excludes itself and never attempts a self-hash. The process
census and integrated runtime/release criteria below retain their original scope.

Historical verified publication: **v0.24.151**, source
`b1ddd106440ca1856a47224375cf7996dc2987de`, tree
`a7c9b8cf90467966444b36b8e7a7cdb4ce7c5c84`. SSA's publication/install terminal
receipt is `~/.sirsi/evidence/pantheon-outage-20261030/SSA-pantheon151-publication-install-terminal-20261031.md`,
SHA-256 `11753e040ad3d0245f102950788f9e0db5437885cd9d2d924d7b7ec86e16887a`;
the original publisher readback receipt is SHA-256
`5a9ce11f2ddb0db14b821437c814f415ebd6fd039723af917a7fa391bb324bc8`.
Main's actual Ma'at CORE receipt is
`~/.sirsi/evidence/pantheon-freeze-zero-ledger-20261030/Main-installed151-maat-core-upgrade-and-native-projection.json`,
SHA-256 `f19545b657c657d01e73c3d9eb8d43a77ddab1087639b05151bb2b343f7cc923`.
It records bounded first-use/record evidence on 147 and 151 upgrade,
readiness, exact read, and native projection; it does not certify the whole
product. At the committed code snapshot above, the corrective source had not
yet been published. Later publication and supported installation are established
by their exact external release/adoption receipts; this source record does not
predeclare their result. Public140 and the detailed
140/141 evidence below remain immutable historical records. This document
defines acceptance and records scoped evidence; it is not a certificate merely
because the file exists.

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
| PT-C01 / REQ11 | Origin/main wing, declared roster and byte-identical universal registry pin | Owning Pantheon24b4e50b and central Stack Lab8097b5d46e481e7437904da186848aee98934eaf independently read back2026-10-27T18:23:47Z: active and pin each3,111 bytes, same Git blob0560f9ae782dbf837acf1e7f76a58b2aa2ad0d9e and SHA256743e7148cf2e68e58b742231dc3064d1a0be968cf1ccfd239d3d4be3b39e7d21; exact PINNED line agrees. Main readback receipt SHAd2ce62684a5a4867252be52adff5f76ef059ef10a13c97aa273b1f229aa9122d; earlier27a7 evidence retained in history | PASS for current source wing identity; not release or runtime certification. Mirror synchronization remains C11 |
| PT-C02 / REQ02,11 | Nine maintained records accurately cover current product | Nine records and84-group finite census independently accepted at f676 and normally merged as24b4e50b; the following corrective-source refresh is pending its final inventory and review | Main/SSA: preserve that acceptance scope and independently check the final refresh; product/runtime acceptance remains separate |
| PT-C03 / REQ08,11 | Complete component/dependency/upgrade and process-diagram coverage | 84 groups,286 parsed Mermaid blocks; original finite-census semantics reviewed at f676 and corrective slices carry bounded reviews. The historical inventory enumerated1,944 digests from958db55b; the earlier corrective d037 inventory enumerated2,151 blobs; the current 856eae50 inventory enumerates2,162 source/data/test/support blobs across104 packages/targets, including direct embedded assets. External final-head comparison remains separate from source classification and process/runtime qualification | Main: bind the final source inventory and review bounded corrective crosswalks; no runtime claim from diagrams or source inventory |
| PT-C04 / REQ01,02,05 | Exact clean composed source, full Go/Swift correctness/security/resource qualification and Ma’at A17 assessment | Exact c165 tree equals qualified clean PR head3d5663dd. Full no-short normal Go8504PASS/29SKIP/0FAIL and race8504PASS/29SKIP/0FAIL, full vet exit0; independent SSA readback accepts receipt SHA9cd8b34af9b0ffdcd015ce553c9cfa14f6b4deae22022b8ee41b76e0c85704c5. Full default Swift346PASS/13SKIP/0FAIL receipt SHA77ee4325243bfc00805a67f8db7161cc5c0d11a321f10075d0186263f2acbc1d. Exact PR protected CI40568582554 attempt1 green; post-merge CI cancellation is retained, not called a pass. Earlier owned test-harness failures remain preserved | Actual full installed141 A17 audit at clean c165 returned91/100,83PASS/11WARN/3FAIL; disposition SHA4fd6db55a5c66363ec7f5dbbf65f82b1028fd3ce50e957e43562dcd1c14b8012. Numeric85 threshold met; FAIL remains for historical canon linkage2/10, mailops19.8% and servicedrain34.7% vs50%. Historical corrective snapshot958db55b raises mailops to63.6% and servicedrain to51.0% against unchanged50% floors; four-package normal/race each1843PASS/2explicitSKIP, receipt SHA9e84c924c9bfe3a5d7d4a9601d1069e89f1a10d0f22e8dbbed8b93c7c6487569. This does not rewrite c165 or constitute a new full audit. Main/SSA: bind the independently accepted historical crosswalk without rewriting commits (SSA review SHA57eee0a04a4b9462b1959a2acb1fe9db5adc391c3a9d20be008cd5924cefcf7e; original20%/FAIL preserved), qualify correction delta. Counts/score are not resource/installed certification |
| PT-C05 / REQ03,10 | Same signed/notarized/stapled one-engine Python-free app in DMG and PKG | Public141 pair independently downloaded, exact size/SHA and staples matched; Application/Installer Team9D382WV988 and trusted notary verified. SSA exact signed-pair summary SHA6ab2c759b1b88ed1b22f374f01d56c2f604424e1ff9a215ac9bd38e3b87686f0 binds canonical inventory accepted=true,29entries,2expected engines,python_free=true (inventory SHAebfb0fcf99ab335d8bd27880196d5aa0b9b56457fbd68cfd0f4db9235f3a9a9b); package/app/built CLI+GUI hashes equal, expected resources/modes checked | PASS for exact observed canonical inventory, resource/mode and same-payload evidence. SSA linkage observation SHA f0186aca36fe3253d109307937b55b4685ef5be404e81f1f819e77ed2f695e75 lists only Apple/System/Swift dependencies for the two exact hash-pinned payload binaries. No Python dylib listed. Runtime resource behavior and transitive library closure remain distinct; no host-wide Python-free or all-runtime claim |
| PT-C06 / REQ02,03 | One exact tag/release and canonical cask with remote bytes | Public141 release425936406 at20:13:02Z, annotated tag6b23bb53977a1be34881ab0d19cabb39d3864461 peels c165. Independent public pair receipt SHAef7ae3963e90ecde07ec3bbf19e5d65adbcefe21e4468d648d06b0c9d24c9007. Cask at tap a4388a1fa081a4b913ea9fe054596f765beddc65, exact bytes SHA54125c3e428b7125dcc91a3d63ca6ea88030e32f89e250a71cfae876ffc22a19; independent readback SHAc26f0ee53802c366857d3d39b401cd18cf0df6867ea4b37507e0a557727abec1 | PASS for observed public141 tag/assets/cask only. SSA owns one subsequent corrective publisher; never rewrite141 or infer installed functionality from download hashes |
| PT-C07 / REQ04,09 | Supported install/upgrade/rollback/uninstall, loaded adoption and cleanup | ACTUAL hosted run40627882045 attempt1 at exacte613: unmodified commercial cask install140 → upgrade141 → exact reinstall rollback140 → uninstall all exited0 on macOS15.7.9 ARM64/uid501. Three PATH CLI version/source/digest joins and strict Team9/Gatekeeper checks passed; rollback digest equals initial140. Terminal4eccd47dc66b2b227082d84b09dba052342984ae7ebdf071429ef4c03154ee82 has original_failure=null and cleanup_debt=[]. SSA independently rehashed118 raw logs/59 calls and read final namespace/tap restoration | PASS for this exact140/141 commercial cask lifecycle and bounded cleanup profile only. Selected141 loaded-adoption observations remain historical evidence; this does not qualify142 packages, new GUI/fonts/menus, all-worker/fleet drain, M5-off behavior or physical VM disk erasure. SSA owns the separate142 release and installed acceptance |
| PT-C08 / REQ01,05,07 | Complete, usable native/CLI/TUI/MCP/web paths with producer identity and truthful state | Actual loaded M5 native141 vnode and SSA kernel CDHash match signed c165 payload. Native recipe/identity/Ma’at/history/knowledge/Mercury read-only subset observed, receipt SHAc0da33ab446823422b40f408dfcbbf8d60aad4a8f67403aba51f729f126b1058. Actual browser Fabric timed out: snapshot20.008s, Fabric18.075s; receipt SHA0629259e575f4e484dde6a50a6824a2c63a8f890dd5a1ab5c3de7426f7dc2ae6. Passive ReadFabric also wrongly reaches explicit auth probes by source inspection; actual latency attribution is not established | Main/SSA: same finite cohort removes passive auth/model commands, preserves canonical classification, makes reads cancellable/coalesced and verifies actual installed recovery. Browser closed to stop repeated slow polls. HTTP200 and native subset are not complete surface proof |
| PT-C09 / REQ06,09 | Ma'at default local assessment/search/classification and actionable safe resolution | G04 engine and native source qualified; recipe/catalog remain canonical. Component scan budgets do not prove JEV-relative superiority or all repairs | Main/SSA: final composition and installed baseline with optional LLM unavailable, original findings remediated/reconciled to terminal evidence |
| PT-C10 / REQ08 | Stack Lab recipe inspection and selection/run preserve exact authority | Installed141 inspects10 recipe contracts with0 unreadable; Ma’at components and exact Release source fingerprint visible. Actual planner transferred10cores/25GiB/1GiBswap into non-executing Apollo plan but normal Back reset9/24/0. This is a real selection-continuity defect; no inference was started | Main: independently accepted source patch preserves per-window/per-journey intent and fresh return validation. Actual focused Swift4/4PASS and private root-owner-reset mutant assertion FAIL establish the source guard, receipt SHA82b593f3151f7038b0dcca95d9a6aecf4470aa2c01c58ebc5467cd9ae7aa698d. Corrected installed observation remains required. Upstream qualified execution/result proof remains upstream-owned; no invented producer capability |
| PT-C11 / REQ11 | Repository, Owner Reading Room and native Workspace cross-linked and current | Exact c16511-document262886-byte source edition published in existing Owner Reading Room and native Doc1Q8l7mTpOCLsbrU6i0aW9CK3oe1yrOT-e-uz-JgIuKeo; producer receipt SHAcd95023fc937034bb82f8d143b6e11c7213578aca52fc08ca135f1d806cec809, independent source-byte/document readback SHA16c4e3bc0396c69b1e52ab869b14fa9e53f4e3b5a04b4ce865fac76d0119813c. Detailed diagrams remain canonical links, not claimed as copied bodies | PASS for observed c165 source edition only. SSA refreshes these same human homes after the finite correction merges; this draft is not silently mirrored or called current installed certification |
| PT-C12 / all | Independent integrated review, reproducible completion proof and truthful original task closure | Source reviews and individual receipts remain scoped; this file supplies no blanket acceptance | Main/SSA: review all applicable rows against final source and installed evidence; close satisfied originals only, keep owned next action for residuals |
| PT-C13 / REQ11 | Project wing and recorded GCP billing/cost ownership crosswalk | Central Stack Lab PR504 normally merged as360a1b533001e1d540626f867be0d001b8b867a2:20 repositories and5 named projects registered. SSA private publication/readback proof SHA2566bed753490120cdc23e9187db6a047205fcca80c5f223b814742257d4a48b9bd records existing billing associations, an immutable private cost-register object and human reading copies | PASS for bounded portfolio registration/accounting publication only. Current PT wing repin is verified separately in C01. This is neither native per-repository Cloud Billing enrollment, measured cost allocation, budget creation nor full product certification; no financial account details are published here |

`REQnn` refers to `PT-REQ-nn` in PRD. Detailed process/requirement mappings are
in DIAGRAM_INDEX and TRACEABILITY_MATRIX. Upstream inference/hardware qualification
is consumed by identity, not reissued by PT. Unknown is not clean; a named
remedy is not evidence that the remedy ran successfully.

Historical message-to-canon reconciliation is recorded in
[HISTORICAL_CANON_CROSSWALK.md](HISTORICAL_CANON_CROSSWALK.md). Its independent
acceptance repairs discoverability only; the original audit and message finding
remain unchanged. The original independently received review is retained as
`SSA-c165-historical-canon-crosswalk-review-20261027-original-received.md`,
SHA256 `57eee0a04a4b9462b1959a2acb1fe9db5adc391c3a9d20be008cd5924cefcf7e`.
SSA subsequently clarified the existing numeric85 criterion; that appended
record is SHA256 `a76c6ffe57716133de27f9361ae814834209000f2072ac40760083ecc13d7106`.
Neither record changes the original audit FAIL or creates a zero-findings gate.

## Published151 baseline and scoped receipts

- Annotated tag `c06550cba35164e7e2b36bf16d57ecfd87dbd02a` peels to the exact
  baseline commit and tree above. Release `v0.24.151` was published as release
  `429815923` through SSA's sole publisher.
- DMG: 22,335,527 bytes, SHA-256
  `4ec0b630dd2af73744ffb033077f3c912ccd0bce634486131ea033a296a08777`.
- PKG: 33,435,066 bytes, SHA-256
  `8280d9115fef50f32556ee7b173783b8cbefb1ab62dbf638b7751ae70733eb11`.
- SSA's terminal receipt records successful publication, trusted signing,
  notarization and staple validation. The bounded selected-host installs and
  Main's actual CORE receipt do not establish whole-product or fleet-wide
  certification.

## Historical published141 artifact identity

- DMG: 22,041,017 bytes, SHA256 `5b1db570ef2fadaba3c60c1ad4e9bd1e3b3931cc82f77773525c7a20cba70334`.
- PKG: 33,072,685 bytes, SHA256 `5f22ec60ded3d76933f344f00095e7599af49766c264adad920cefa7c696d674`.
- Main freshly downloaded both public assets and verified their hashes, staples and
  trusted Installer/notary identity. SSA signed/published through Team9D382WV988.
- Selected141 CLI/app installations on M5, M1 and Studio are recorded by SSA at
  `pantheon141-publication-and-selected-three-host-install-20261027.json`.
  A selected installation is not proof of every workflow or fleet-wide drain.
- Public141 proof directory under the retained evidence root below:
  `public141-independent-download-readback-20261027/`.

The following correction remains source work until independently qualified,
merged, released and observed through the normal supported adoption path.

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

**Wing identity: verified for the observed origin record. Full integrated
certification: NOT YET ESTABLISHED.** Historical 140/141 criterion outcomes
remain tied to their stated source and receipts; publication and scoped 151
observations do not promote them or certify the subsequent correction. Remaining
rows are executable obligations,
not new features or owner approval requests. Update this record with exact final
receipts and independent verdict after the one corrective release/adoption;
never silently promote historical or component proof to whole-app completion.

### Corrective backend qualification continuation (2026-10-27 local)

The corrected concurrent dashboard fixture seeds the pending state before the
concurrent batch and requires every returned board to contain the same single
real pending-to-in-progress transition. A subsequent fresh acquisition must
retain that event without duplication. It does not require eight independent
acquisitions from eight overlapping callers.

Actual evidence: `corrective-fabric-backend-qualification-r2-20261028/terminal.json`
under the freeze evidence root, SHA-256
`5c6ddd0fa881c9885b73f09078a66e3d8785626184e8e0c8c0a7cfa6121bf00a`.
Five affected packages: normal **1753 PASS / 11 SKIP / 0 FAIL**, race
**1753 PASS / 11 SKIP / 0 FAIL**. Focused transition and actual passive dashboard
collector tests each passed normally and under race detection. Source manifest
was unchanged during the run. Native execution
`95f1a8d2ba4d45a3ae4a716a2247d342` completed successfully.
The prior fixture's race failure remains retained; this is source qualification,
not installed latency, remote cancellation, full certification or release proof.

Native font/menu final source qualification: full default Swift suite selected
369 tests: **356 PASS / 13 SKIP / 0 FAIL**; compile succeeded and source hashes
remained stable. Evidence `native-readable-menus-qualification-r4-20261028/terminal.json`,
SHA-256 `8a3475ebe6a53a90a7a45899b42305dc217725c7196baa536a999a53270868e0`.
Earlier compile failure and earlier pre-chooser-correction successful runs remain
retained, not substituted as the final source proof. Full Go vet exited zero:
`corrective-go-vet-20261028/terminal.json`, SHA-256
`5212fb80455a9d703f8cca35130489486b812d9d2ad23fba191d2ebfb219c504`.
Current 286 Mermaid blocks parse with zero syntax errors:
`corrective-diagram-parser-20261028/terminal-r3.json`, SHA-256
`688a75cf70be12c0f23be22ac58ed14a4122cc4cd81752e71c7939daab0208be`.
This is syntax proof, not visual/semantic certification. Native installed menu,
keyboard, accessibility and maximum-size layout acceptance remain separately
observed checks; no source test promotes those rows to PASS.

The native font/menu bounded source review is accepted for its exact file hashes:
private receipt `native-readable-menus-source-review-20261028.md`, SHA-256
`07a568eb0fcdaa16e13da7f37abb64bc901382093338f13e6d07d3c45d2560d9`.
Its dirty-byte identity is reconciled to committed source `958db55b` by the
final external source receipt; its historical HEAD field is not the candidate.
The metadata inventory refresh does not change product source or promote
source tests to installed certification.

Post-qualification native delta: the existing scaling-primitive comment marker
was moved onto the `.font` line so the existing whole-target CI guard can
recognize it. This is a comment-only source change; the prior 96-file test input
manifest is retained, with this one exact delta rather than falsely claiming
all final file bytes are identical. The font structural guard passes with
964 scaling sites across 50 Swift files and zero bypasses.

Actual C07 evidence binding: `~/.sirsi/evidence/SSA-C07-commercial-cask-140-141-ACTUAL-20261027.md`,
SHA-256 `2dd6bb1153c33603ba60c1bdddd65243fd533b8b6af3075422ecbc73f8a5a6c1`. The one authorized hosted run used the unchanged
reviewed v4 controller. Artifact12789788992 contains the actual phase, trust
and cleanup receipts; its API-reported compressed ZIP digest is not represented
as an independently hashed ZIP. Earlier source-only/inert fixtures and failed
qualification attempts stay retained as history. This actual scoped result
replaces only the formerly unrun140/141 lifecycle profile, without promoting
the142 candidate or the entire application to certified.
