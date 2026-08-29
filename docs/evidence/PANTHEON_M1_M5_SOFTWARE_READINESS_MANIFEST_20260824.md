# Pantheon M1/M5 software-readiness manifest — 2026-08-24

Status: **not release-ready**. This is the authoritative current disposition
for Pantheon software readiness. It replaces the 2026-08-23 “current” boundary
as the current reference and leaves all service, security, and Tailscale state
unchanged.

## Method and authority

This manifest combines only:

- retained M1 receipts in this repository;
- read-only M5 observations captured on 2026-08-24;
- accepted Hardware Admin transport policy: positive bounded SSH/5900/Tailscale
  transport wins over stale local engine fields; and
- accepted SNE gate policy: host transport and GPU/Aqua/Metal eligibility are
  separate facts.

`active` means currently observed or source-backed, not release-qualified.
`required` is a release gate. `stale` is retained but cannot support a current
claim. `protected` is deliberately outside Pantheon mutation authority.
`deferred` is not release-critical in this cut or awaits its owner’s receipt.

## Current host facts

| Observation | Result | Disposition | Claim boundary |
| --- | --- | --- | --- |
| M5 OS / CPU | macOS 26.6.1 (25G76), arm64 | active | local read-only host identity |
| M5 power policy | `sleep=0`, `tcpkeepalive=1` on AC and battery | active | power policy supports unattended observation; it is not a performance qualification |
| M5 transport listeners | TCP 22 and 5900 listening | active | local listener proof only; Hardware Admin’s bounded positive transport probes are the availability authority |
| M5 Tailscale self projection | `Online=true`; `Active/InEngine/InMagicSock=false` | stale diagnostic | never overrides positive bounded transport; no second plane is authorized |
| M1 bounded service receipt | exact package/runtime identity, HTTP 200, `M1-READY`, rollback restore; readiness receipt is `ready` for fixed-capacity-64 interactive tuple | active bounded functional evidence | not a clean benchmark, performance claim, or current M1 deployment-validation |
| M5 five-lane work | v37/v21 Fresh20 remains terminally rejected; a distinct corrected v22/v39 full qualification is active under SNE ownership, using the source-reproducible parent runtime `15a3…` and repaired package `0e04…` | protected / active / required | The new run is **not r6** and is not the historical rejected stack. At the bounded delegated observation it had 245/700 sealed samples (7/20 repetitions), with all seven MTP rounds including near4k exactness passing so far and one serialized provider at a time. Its controller/packet receipt does not yet exist. Pantheon must not inspect, restart, deduplicate, or clean up its provider, and may not claim correctness completion, performance, release, or GA. |

## Receipt binding

| Host | Canonical receipt | SHA-256 | Accepted fact | Explicit non-claim |
| --- | --- | --- | --- | --- |
| M1 | `docs/evidence/sne-e2b-api-v2-compat-v6-pantheon-m1-20260821/readiness.json` | `141f71a56a0c80556d999083877e4dc9c18cb55ca3777c16999a3c989791b2dd` | `status=ready` for the exact interactive tuple: service/runtime SHA `3c27…`, native runtime SHA `e22d…`, fixed-capacity-64 cache, concurrency 1, queue 8 | clean benchmark, M5 equivalence, thermal/power qualification, performance, GA |
| M5 | canonical read-only `docs/evidence/artifacts/m5-benchmark-readiness-20260824-accepted/m5-readiness.json` | `210fcdcf8c224406daabcb471625dc1ab44dac94692bc02d9d811de3fc4f1af0` | at `2026-08-24T02:21:41Z`, `state=idle`, `admit=true`, unlocked console, observed power/memory/process inputs, and no mutation/restart/security/runtime change | persistent condition, serving proof, benchmark result, performance |
| M5 | canonical read-only `docs/evidence/artifacts/m5-aqua-unlocked-0519/m5-readiness.json` | `7d134bc5f9f555f3514d1c1e6635c4c0e6f6ee9c323a1334721bd445cadc9bf1` | at `2026-08-24T05:19:29Z`, `state=idle`, `admit=true`, `IOConsoleLocked=No`; unlocked Aqua-session timing prerequisite | current qualification after the later Fresh20 qualification, locked serving, or fresh Metal acquisition while locked |
| M5 | canonical read-only `docs/evidence/artifacts/m5-v37-v21-common-capacity-host-admission-20260824T062650Z/m5-readiness.json` | `18f63776269b8f84e4535722d8ce145b47ee468bb0e9a1dec67bfa9135088351` | copied v37/v21 common-capacity qualification admission: `state=idle`, `admit=true`, with no configured desktop/renderer contention candidate observed | five-lane completion, a package lifecycle result, locked serving, sustained Metal acquisition, performance, or GA |
| M5 | external protected `sirsi-native-rebuild/benchmarks/evidence/sne-five-lane-v25-common4k-safe-sne-plain-diagnostic-20260824-r1/aqua-receipt.json` and `provider.log` | aqua receipt `eb7f1cdbee9022379cba26a6afed6ac873f0cb7e554313b8f05182b647ed8ef7`; provider log `635f43be734ffc40f67678288686114e23bac7f39ef30fa6b28bd8141d783268` | Aqua receipt `accepted`, exit 0, same-session M5 Metal blit; seven SNE-plain workload records report `semantic_pass=true`, `instruction_pass=true`, and `stream_equivalence=passed`, including 3655 prompt tokens + 32 completion ceiling | multi-lane parity, fresh20, thermal/power qualification, sustained serving, performance, or GA |
| M5 | external protected `sirsi-native-rebuild/benchmarks/evidence/sne-five-lane-v37-v21-common4k-safe-fresh20-20260824-r1/controller-receipt.json` and `aqua-receipt.json` | controller `faffeb13890a2e79cf941e1527a982b21a850170b803345427035c1914948974`; Aqua `d4679ba2f49d11c0fd348bdb12212d39cd7ae485aa3e9103b91dfbd859a4ad9d`; bound admission/preflight/config remain `18f637…`, `6e6f11…`, `a5d48d…` | terminal **rejected**; its controller receipt was created at `2026-08-24T06:32:41Z`. Controller: 20 repetitions, 4 processes, 28 samples, `performance_claim=false`. SNE MTP runtime `82779b87b7813fda3beb07517c0e0eac7142c4e829edca8369ac88e523a7eb91` returned repeated `west warehouse` in streamed and buffered near4k retrieval where exact `east` was required. Raw MLX, patched MLX, public oMLX, and SNE plain passed; row-exact-disabled failed identically. Historical `98ee28bcd8691bd634de17e663c8c4adca3d399125c79b4b301da3eac2242fa7` and parent `15a3b4b975e8191df0e97f890060ebfe78b5a3dd7bc2dfbc2baaefb560c2e115` returned exact `east`, localizing the regression to the `15a3→82779` runtime merge. Aqua: rejected, exit 1, not timed out, Apple M5 Max Metal blit of 64 bytes passed. SNE lane’s later bounded observation reports no `:18580` listener and no `sned`/provider/controller process; only pre-existing protected Aqua brokers PIDs 69092 and 85269 remain. | correctness, throughput, bandwidth, durability, performance, release, GA, or any claim that the two protected Aqua brokers are benchmark residue |
| M5 | external protected `sirsi-native-rebuild/benchmarks/evidence/sne-five-lane-parent-runtime-modern-service-all7-correctness-20260824-r1/aqua-receipt.json` and `provider.log` | copied package SHA256SUMS `d2dc6c0bfb6ec9c18f0accbbb27f35c9b1496231d95eca9c4ae7adef1714061c`; Aqua `0342d4a25886e1e05aee68eef23adc77d7b4362dcf5fc01cd8143ce3c6f7e0c9`; provider log `2a963c88da65e65f0e56a0ed32d6670b07a0ee8a3c9bb72eb265d2e88c88c404` | **correctness accepted; performance qualification only.** Copied repair uses authenticated service `e34db906…`, accepted parent runtime `15a3b4b9…`, and model manifest `b90829d0…`; all seven strict public-API MTP workloads passed semantic/instruction checks and stream/buffer equivalence with zero process swap growth. Aqua accepted, exit 0, M5 Max 64-byte Metal blit passed; cleanup is recorded clear. | performance, release, GA, exact source rebuild, long-run durability, or full five-lane parity |
| M5 | external protected `sirsi-native-rebuild/benchmarks/evidence/sne-five-lane-correctness-repair-fresh20-v22-v39-20260824-r1` | host admission `87edacd1a964b784b996f5e0ad3e329929a04b1c13c0fdb8dfa8973f3a6e3ca9`; preflight `132a875b9668dc258c659a6584deb319547c9795bb7ed6297f834b692a77f1d8`; controller v22 `fb35f8cde6c0c69e301ce774564b011b8f0211d079c626971c79ac86ba511d94`; config v22 `99e22360529ecb4c3eb84680da6e0eaba34bf5c6775330607a201b77e4fc5df7` | **correctness qualification accepted; performance claim false.** The run sealed 700/700 samples under a single serialized provider; the final controller receipt is `status=accepted` and explicitly carries `performance_claim=false`. The exact package/runtime chain and M5 admission/preflight inputs remain bound; this receipt closes the run’s correctness gate only. | performance, sustained durability, broad package lifecycle, release, GA, or any claim beyond the receipt’s explicit correctness scope |

The M5 receipt files are authoritative Hardware Admin artifacts in the
canonical checkout and are read-only inputs here. Qualification is
**receipt-dependent**: a five-lane run may begin only against a fresh matching
idle/admit receipt and may be accepted only with its own complete provenance,
exit, cleanup, power, thermal, RAM/swap, isolation, and exact artifact receipts.

## Classified inventory

| Domain | M1 | M5 | Disposition | Evidence and precise next proof |
| --- | --- | --- | --- | --- |
| Go shared operational engine | source and broad tests exist | source and broad tests exist | active / required | Go remains the CPU/I/O/control-plane owner. Prove a packaged, sustained idle/active resource budget on each host. |
| Go CLI, TUI, MCP | build/test evidence; current isolated package candidate | build/test evidence | active / required | The current bundled CLI reports `v0.23.9-beta`; `tui --help` exposes the five-screen console and `mcp --help` exposes the packaged IDE entrypoint. On 2026-08-24, an isolated temporary CLI built with the same `-X .../internal/version.Version=v0.23.9-beta` release stamp completed a stdio MCP `initialize` handshake with exact `serverInfo.name=sirsi-pantheon`, `serverInfo.version=v0.23.9-beta`, and Pantheon product instructions; `version --json` reported the same version (both exit 0). An unstamped source build reports `dev` by design and is not package-identity evidence. Focused `internal/tui`, `internal/mcp`, and `internal/dashboard` tests pass. Missing: installed-device invocation and sustained-resource evidence. |
| Go control engine | source and focused tests exist; legacy Go tray sources and `fyne.io/systray` were removed | isolated app packages `pantheon-engine` helper | active / required | `cmd/sirsi-menubar` is now a quiet local API/lifecycle helper only. The release contract rejects a Go tray dependency or source linkage; Swift is the sole visible surface. The helper is no longer started during native-shell launch: an owner opening the panel starts it, while CLI, TUI, and MCP retain direct Go entrypoints. Its DMG assembly enforces a 128 MiB RSS / 5% CPU short-window gate. Fresh isolated build `2026082406` proved the engine was serving—not merely idling—by completing six loopback `/api/stats` requests (one before and one per sample), then sampling 25,472–26,768 KiB RSS and 0.7–1.4% CPU across five samples; it verified no descendant process, loopback-only listener, and child teardown. The earlier 21.70 MiB / 0.0% idle and 22,352 KiB / 0.0% 41-second source observations remain bounded supporting evidence. None proves installed M1/M5 sustained resource behavior. |
| Native Swift menu-bar shell | builds/tests in the current source | isolated app packages `SirsiMenubar` as `CFBundleExecutable` | active / required | The shell is bound to the exact bundled CLI and Go helper. The bridge now projects `running`, deliberate `stopped`, or `unavailable` helper state into the native command-center header. A real temporary helper exit with status 7 was surfaced exactly once as owner-visible unavailable state; no automatic restart occurred. The header offers **Retry local control** only after an unavailable state; the owner click invokes one direct bridge start and is suppressed while already running. Launch, timers, and exit handling cannot invoke this retry. Closed-menu projections now update from coalesced directory events (findings, health, and router board) and never spawn `sirsi thread list` or use a stale-board CLI fallback. A 15-minute fallback with 5-minute macOS tolerance heals unavailable watches or dropped events; it replaces the prior 90-second poll. Opening the panel and explicit operator actions retain interactive live reads. The root is now a five-destination Pantheon command center (System hygiene, Applications, Developer workspace, AI & SNE, Automation), with one owner-visible next action and receipts continuously reachable; route tests preserve existing deity screens behind those hubs and explicitly separate transport from GPU-session admission. Current deterministic 400 pt and 720 pt fixture renders pass visual QA and embed `VISUAL FIXTURE · NOT LIVE`; they prove hierarchy only. A manual interaction pass on the real packaged app remains required, as do clean install, sustained CPU/RSS, sleep/wake, crash, or Developer ID/notarization receipts. |
| Native menu installer source | native-shell-only, explicit owner action | generic setup no longer invokes the installer | active / required | The installer accepts only `Pantheon.app/Contents/MacOS/SirsiMenubar`, refuses a bare Go tray, does not issue `launchctl enable`, and is guarded by release-contract checks. This source correction does not migrate the already-observed legacy installed label; that needs an explicit receipt-backed lifecycle validation. |
| Package composition and relocation | isolated ad-hoc bundle contains Swift `SirsiMenubar`, Go `Contents/MacOS/sirsi`, Go `Contents/Library/Helpers/pantheon-engine`, and the app-local LaunchAgent resource | same isolated arm64 bundle | active / required | DMG and `make bundle` now use that identical native shape; the latter no longer creates a raw Go app. The curl installer now refuses macOS to prevent a second raw CLI surface; macOS distribution is the native app/cask only. Both the DMG and local native-bundle paths run the same verifier, which proves the bundle executable, embedded Go versions, local/ad-hoc/Developer-ID signing mode, and a one-request EOF-terminated MCP `initialize` server identity; all three executables are inside the bundle. The verifier accepts a linked Git worktree only after structural validation of the actual release-root inputs (`VERSION`, `go.mod`, Swift package, DMG script, workflow, `cmd`, and `internal`); it no longer mistakes a `.git` pointer file for an invalid release root. It does **not** prove a drag-install, upgrade, uninstall, rollback, or a clean user account. |
| Build toolchains and dependencies | release workflow pins Go `1.25`; macOS package compiles SwiftPM release source in an isolated scratch path | no signed CI artifact/receipt | active / required | The 2026-08-24 audit host reports Go `1.26.2`, Swift `6.3.3`, and Xcode `26.6` (build `17F113`); that host-toolchain observation is not a signed CI receipt and the Go `1.25` workflow pin remains the release build contract. No Python launcher is called by the app assembly, embedded CLI, or embedded control engine. Swift/Xcode, Developer ID, notarization credentials, and a reproducible signed CI artifact still require exact receipt capture. |
| M1 Pantheon installation | 2026-08-22 ad-hoc dev package upgraded/recovered | not applicable | stale / required | The receipt is controlled development evidence only. Missing fresh M1 public-path install, upgrade, rollback, uninstall, reboot, and resource qualification. |
| M5 Pantheon installation | not applicable | no accepted installed Pantheon release receipt | required | Do not infer installation from source/build tests. Missing signed/ad-hoc-labelled package identity, install/rollback/uninstall, and launch stability. |
| Installed menu LaunchAgent | current audit-host setup projection reports `ai.sirsi.pantheon` installed but not loaded, targeting `/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar` | host identity is not asserted as M1 or M5 | stale / protected | The retired Go tray path still exists, so the old label is not an installation proof for the native Swift shell. Preserve its disabled state; a future receipt-backed package migration must replace the path only during explicit owner-approved install/rollback validation. |
| SNE runtime, model, checkpoint identity | retained exact M1 tuple evidence | v37/v21 Fresh20 is terminally rejected; copied parent-runtime repair is checksum-valid and all-seven correctness accepted; corrected v22/v39 qualification is active | protected / active / required | The regression is isolated from service/API/corpus/assistant/model/MLX and the row-exact toggle. The active run binds source-reproducible parent runtime `15a3…` and repaired package `0e04…`; its final receipt is pending. SNE artifacts remain immutable external identities and none of these facts authorizes a Pantheon runtime mutation. |
| GPU/Aqua/Metal eligibility | historical M1 locked-serving evidence is separate | M5 Metal preflight passed for the rejected fresh20 command | protected / required | A successful Aqua/Metal blit proves only command admission/execution; it does not prove correctness or performance. A reachable host is not fresh Metal acquisition. |
| Unattended transport | historical M1 SSH/5900 evidence | current 22/5900 listeners plus accepted positive bounded transport policy | active / protected | Lock, display sleep, closed lid, and stale Tailscale fields must not mark the host unavailable when bounded transport succeeds. Missing a canonical copied M5 probe receipt in this worktree. |
| Tailscale plane | retained M1 evidence | stale M5 engine fields with online identity | protected | Single-plane only. No bootstrap, re-authentication, daemon coexistence, or settings change is authorized. |
| Aqua-session ownership | historic multi-session observation | two `sne-aqua-session` processes observed | protected / required | Separate SNE Aqua launcher owns them. Missing SNE-owned queue/session reconciliation; Pantheon must display/fail-closed, never kill or deduplicate them. |
| External SNE qualification stack | no Pantheon authority | v37/v21 Fresh20 stack exited rejected; distinct corrected v22/v39 qualification is currently serialized under SNE ownership | protected / active qualification | The historical controller/Aqua receipts identify the rejected attempt, its cleanup, and the localized `sne-mtp` near4k regression. The current active run is **not r6**, has one provider at a time, and has no final controller/packet receipt. Pre-existing Aqua brokers PIDs 69092 and 85269 remain protected and are not benchmark residue. Pantheon does not infer final state, inspect activity beyond the supplied bounded receipt facts, or perform cleanup. |
| Permissions | source/test contract is receipt-backed and fail-closed | same contract | active / required | Sirsi Admin accepts the posture for internal evidence: no ambient TCC, keychain, OAuth, Firebase, Seshat, or sudo prompts. Missing installed-artifact owner-visible receipt-flow test. |
| Signing and supply chain | M1 dev receipt: ad hoc only; prior Developer ID key unavailable | no current release artifact | required | Missing Developer ID signing, notarization, reproducible asset SHA, and installed verification. |
| Python | optional scripts/training/orchestration remain in tree | an unrelated Python HTTP server was observed, not attributed to Pantheon | deferred / required governance decision | The assembled macOS app, embedded control engine, release DMG script, Homebrew cask path, generic first-run setup, and `sirsi router close --proof` do not invoke Python. The package-identity gate rejects source/bytecode (`*.py`, `*.pyc`, `*.pyo`, `*.pyw`, `*.pyi`), wheel/egg distributions, every `python*` interpreter name including versioned variants, a copied `Python.framework`, and a Python framework/runtime linkage from each shipped executable. A fresh isolated ad-hoc build `2026082405` passed the clean payload gate at 22,224 KiB / 0.0% CPU; adding only `Contents/Resources/fixture.py` made the verifier fail closed with `reason=python_payload_present`. `sirsi completion validate --proof` exposes the same local in-process Go validator used by router close; it is read-only and creates no prompts or ambient authorization. `sirsi ra` remains explicitly opt-in. This removes Python from the normal completion-proof product path, but does not support a repo-wide zero-Python claim while optional scripts/training/orchestration remain. |
| Version and publication | at local `HEAD=ede68456f40065e7331a74dd51ef337e10bfc7aa`, `VERSION=0.23.9-beta` (file SHA-256 `20a044f185faf886dbe8105889ddf5cc97018b15a9a7524aa7bd763a5676c85d`); `git describe --tags --always --dirty` resolves `v0.23.8-beta-526-gede68456-dirty`, and the newest listed local tag is `v0.23.8-beta`; tracked Homebrew cask is `0.23.8-beta` (file SHA-256 `948f2cac8afe348e0a708621c0ec0d11146015f9e842cb9baa3c7ee3306a7b27) | stale / required | `scripts/verify-release-source-identity.sh` is now a direct CI gate in both release jobs: source/tag alignment is required before build; a local tag object is required in CI. Current static proof accepts `--tag v0.23.9-beta` but reports `cask_state=stale`; it rejects `v0.23.8-beta` with `tag_version_mismatch` and rejects the untagged `v0.23.9-beta` under `--require-local-tag` with `tag_not_present`. Source, reachable tag, and cask therefore cannot identify one releasable asset. Reconcile VERSION, tag, cask/formula, GitHub asset SHA, signing, and publication proof before any release claim. |

## Security and permission posture

Pantheon remains the sole human permission broker. A resident/background path
may observe, report, and present an owner action; it must not create ambient
TCC, keychain, OAuth, Firebase, Seshat, or sudo prompts. Receipt-bound
rollback, removal, retry, and uninstall retain exact operation, host, artifact,
expiry, and owner identity and fail closed when a receipt is absent or invalid.

This audit did not call a privileged System Settings command, did not request a
credential, and did not change a LaunchAgent. Current disabled SNE labels and
the Pantheon label remain protected; no disabled/quarantined service was
reactivated.

## Reviewer acceptance receipt

| Reviewer / gate | Status | Durable decision |
| --- | --- | --- |
| Codex SNE | conditional acceptance | Pantheon’s fail-closed, evidence-oriented boundary is acceptable only while SNE package/model/checkpoint/runtime identities remain external and immutable. Host transport remains separate from Aqua/Metal acquisition. The terminally rejected Aqua-owned copied v37/v21 fresh20 attempt on `:18580` remains SNE evidence, not a Pantheon cleanup candidate. |
| Sirsi Admin package review | rejection for public release | Exact embedded bundle identity is verified ad hoc, but clean M1/M5 install, upgrade, uninstall, rollback, signing/notarization, stale-label/disabled-state, and sustained resource/crash receipts are absent. |
| Pantheon release decision | rejection | `v0.23.9-beta` is an internal evidence boundary, not a published release. No GitHub, Homebrew, performance, live-host, or GA claim is authorized until every required receipt below is completed. |

## Exact drift and missing-proof ledger

1. **Release-surface artifact is now isolated-package proven, not installed-
   lifecycle proven:** the DMG packages the Swift native shell, embedded Go CLI,
   and private Go control-engine helper; its ad-hoc identity verifier passes.
   The retired Go tray is no longer a shipped source path; the observed old
   installed label remains protected until a signed clean install proves the
   receipt-backed replacement end to end.
2. **Publication drift:** source `0.23.9-beta`, local `git describe`
   `v0.23.8-beta-526-gede68456-dirty` (newest listed tag `v0.23.8-beta`), and
   Homebrew cask `0.23.8-beta`. GitHub/Homebrew publication remains unverified;
   no one of these identities may be presented as current.
   `scripts/verify-release-source-identity.sh` (SHA-256
   `0fff9fd3aef5313f08878ff0e668b4e236f14367e6d94855a66add89dee3804d`)
   now gates both release workflow jobs. It requires `VERSION` to equal the
   pushed tag and requires that tag to resolve before a release job builds.
   The existing tag is fail-closed rather than publishable: `v0.23.8-beta`
   produces `reason=tag_version_mismatch`; the intended `v0.23.9-beta` produces
   `reason=tag_not_present` while no such local tag exists. A tag/version match
   alone still reports `cask_state=stale` until a signed DMG’s exact SHA updates
   the cask, so this gate cannot manufacture a publication claim.
   The duplicate post-DMG cask update job was removed from the workflow; the
   `menubar` job owns the single remaining cask update because it has the DMG
   and its SHA in the same job.
3. **M1 live-proof gap:** retained M1 development/package receipts are not a
   fresh deployment-validation profile.
4. **M5 lifecycle gap:** fresh M5 idle/admit receipts exist, but they are
   bounded preconditions rather than a package lifecycle or sustained-serving
   result. This worktree references the canonical receipt paths and still has
   no completed package lifecycle receipt.
5. **Five-lane rejection and repair boundary:** copied v37/v21 common-capacity Fresh20 is
   terminally rejected after 20 repetitions, 4 processes, and 28 samples. The
   exact SNE-MTP near4k semantic expectation was `east`; runtime `82779…`
   returned repeated `west warehouse…` in both streamed and buffered forms.
   Raw MLX, patched MLX, public oMLX, and SNE plain passed; row-exact-disabled
   failed identically, while historical `98ee28…` and parent `15a3b4…` returned
   exact `east`. The failure is therefore localized to the `15a3→82779` runtime
   merge, not a Pantheon-owned artifact. Aqua completed without timeout and
   passed the M5 Max 64-byte Metal blit, but explicitly does not claim model
   correctness or performance. A copied runtime repair must pass isolated
   near4k, all seven MTP correctness gates, then a newly admitted full
   five-lane qualification with exact parity, cleanup, provenance packet v3,
   power/thermal/RAM/swap/isolation evidence, and gate pass before any
   performance claim. A separate copied repair with parent runtime `15a3b4…`,
   current authenticated service `e34db906…`, and matching manifest `b90829d0…`
   has since passed all seven strict public-API MTP correctness workloads with
   zero process swap growth. That admits only a fresh performance qualification;
   it does not repair the exact source lineage or make a performance/release
   claim. The new provenance-bound full five-lane timing receipt is not yet
   available and must remain unclaimed.
6. **Resource/crash gap:** the current packaged helper held 21.70 MiB RSS and
   0.0% CPU over five isolated samples through 10 seconds and stopped cleanly.
   A separate temporary current-source observation held 22,352 KiB / 0.0% for
   20 samples through 41 seconds and now verifies post-`SIGTERM` child teardown.
   The normal DMG path enforces a 128 MiB RSS / 5% CPU short-window gate, but
   none of this replaces long-run evidence. It does not show that Pantheon
   lowers, rather than raises, system load over a sustained installed M1/M5
   lifecycle.
   The native shell's prior closed-menu CLI refresh path is now removed and unit
   tested. Its resident projection changed from a 90-second timer to coalesced
   filesystem watches plus a 15-minute fallback with 5-minute tolerance; this
   reduces periodic closed-menu wake opportunities tenfold while retaining a
   recovery path. No long-run native-shell sample yet proves that the repaired
   resident surface remains within budget across M1/M5, lock, display sleep, or
   wake.
7. **Lifecycle and duplicate-label gap:** the package verifier confirms the
   bundled LaunchAgent has no `KeepAlive`, but no clean M1/M5 receipt proves
   install, upgrade, uninstall, rollback, reboot, disabled-state preservation,
   or absence of stale/duplicate live labels. The current audit-host projection
   has one installed-but-unloaded `ai.sirsi.pantheon` label targeting the
   retired Go `sirsi-menubar` path; it is stale and protected, not a candidate
   for automatic reactivation. External SNE Aqua brokers and the former Fresh20
   `:18580` qualification stack are likewise excluded from Pantheon cleanup
   authority.
8. **SNE conditional-acceptance gate:** Codex SNE conditionally accepts the
   v`0.23.9-beta` boundary only. Pantheon must keep SNE runtime mathematics and
   immutable package/model/checkpoint identities external; transport
   availability must remain distinct from Aqua/Metal acquisition. Final release
   acceptance still requires exact runtime/model/artifact identity, clean
   M1/M5 package lifecycle and sustained resource/crash receipts, plus the
   completed matched five-lane receipt.
9. **Python governance edge:** the release package and normal native-control
   path are Python-free, including the agent-facing completion-proof path:
   `sirsi completion validate --proof` and `router close --proof` use the
   in-process Go validator. Optional scripts/training/orchestration remain and
   prevent a repo-wide zero-Python claim.
   This must not be casually replaced: it is a security/governance gate. The
   canonical outcome is a reviewer-approved, parity-tested Go validator or an
   explicit product-contract split; until then, no repo-wide zero-Python claim
   is authorized.
10. **Public-copy hold (implemented):** README, getting-started guide, and FAQ
    explicitly distinguish the unpublished `0.23.9-beta` candidate from old
    records and offer no unverified current-candidate install command. The
    static `scripts/verify-public-release-copy.sh` gate rejects an unqualified
    FAQ Homebrew install instruction and requires the hold language. This
    removes copy as a source of a false release claim; it does not create an
    asset, signing, cask, or publication receipt.

## Non-mutating verification record

Read-only evidence inputs verified in this audit:

- `shasum -a 256 docs/evidence/sne-e2b-api-v2-compat-v6-pantheon-m1-20260821/readiness.json`
  → `141f71a56a0c80556d999083877e4dc9c18cb55ca3777c16999a3c989791b2dd`.
- `shasum -a 256 /Users/thekryptodragon/Development/sirsi-pantheon/docs/evidence/artifacts/m5-v37-v21-common-capacity-host-admission-20260824T062650Z/m5-readiness.json`
  → `18f63776269b8f84e4535722d8ce145b47ee468bb0e9a1dec67bfa9135088351`.
- Read-only v37/v21 fresh20 inventory at
  `/Users/thekryptodragon/Development/sirsi-native-rebuild/benchmarks/evidence/sne-five-lane-v37-v21-common4k-safe-fresh20-20260824-r1`
  → terminal controller/Aqua receipts reject the attempt and their SHA-256
  values, plus host admission/preflight/controller-config hashes, match the
  receipt-binding table.
- SNE lane correction supplied after the v37/v21 Fresh20 closure: controller
  timestamp `2026-08-24T06:32:41Z`; that **historical post-cleanup** bounded
  observation found no `:18580` listener or `sned`/provider/controller process.
  The only named remaining Aqua processes were pre-existing brokers PIDs 69092
  and 85269, retained as protected SNE-owned processes. It does not describe or
  constrain the distinct active v22/v39 qualification. This is a delegated
  bounded observation, not a Pantheon cleanup result or a future-state
  guarantee.
- Isolated-copy `bash scripts/verify-menubar-release-contract.sh` →
  `accepted=true`, canonical entrypoint Swift native shell, Go control engine,
  channels `dmg,cask`, source-`VERSION` DMG default, and fail-closed lifecycle
  policy. The verifier copy was `/private/tmp/pantheon-release-contract.ZqGCoF`;
  it was source-only and performed no build, install, or service mutation.
- Fresh isolated-copy DMG assembly (`VERSION=0.23.9-beta`, build
  `20260824`) → exit 0 and `pantheon_package_identity accepted=true` with
  ad-hoc signing only. The normal build executed
  `verify-engine-resource-budget.sh` and accepted all five samples below 128
  MiB RSS and 5% CPU (22,224 KiB / 0.0% at each 2-second sample). The
  uninstalled 15 MB candidate is
  `SirsiPantheon-0.23.9-beta-arm64.dmg` SHA-256
  `6194ad4765b54b92ac232d349101dbaced8c59eca142f0b4ced5b9dd78fa977f`.
  Its bundled `pantheon-engine` SHA-256 is
  `df10d7614321c4ef9bb016ce61bdaa1564a0914de225973d2ec2a27822041eb0`.
  `pantheon-engine version` returned `Pantheon Engine v0.23.9-beta`, and deep
  signature verification passed. This remains an uninstalled ad-hoc artifact,
  not Developer ID/notarized or a lifecycle acceptance receipt.
- `go test ./internal/sne ./internal/dashboard ./cmd/sirsi-menubar ./internal/tui ./internal/setup ./cmd/sirsi`
  → exit 0 (only the existing duplicate `-lobjc` linker warning).
- `go test ./cmd/sirsi-menubar` after removal of the Go tray → exit 0;
  `go list -deps ./cmd/sirsi-menubar` contains no `systray` dependency, and a
  local `pantheon-engine version` prints `Pantheon Engine dev`. A `CGO_ENABLED=0`
  comparison correctly fails in the existing Guard/vitals physical-memory path,
  so the packaging build retains CGO for measured macOS memory reporting; this
  is not a UI dependency and must be profiled before any language rewrite.
- The current ad-hoc packaged helper (SHA-256
  `df10d7614321c4ef9bb016ce61bdaa1564a0914de225973d2ec2a27822041eb0`)
  ran in a unique temporary home for five samples: 22,224 KiB RSS / 0.0% CPU
  at 2, 4, 6, 8, and 10 seconds; it accepted `SIGTERM` and its exact PID exited.
  See `PANTHEON_CARETAKER_PROCESS_BUDGET_GATE_20260823.md` for the full
  containment and claim boundary.
- A separate current-source engine/CLI pair built only into a unique temporary
  directory passed `verify-engine-resource-budget.sh` for 20 two-second samples
  through 41 seconds: each was 22,352 KiB RSS / 0.0% CPU, under the 128 MiB / 5%
  limits. No installed state was read or changed. The current verifier requires
  the exact test child and every captured descendant to be absent after
  `SIGTERM`; it rejects any idle descendant before sampling the parent. Its
  new child-tree behavior passed a single-process mock and rejected a deliberate
  child leak; the captured mock PIDs were then absent. Verifier SHA-256:
  `7d380b5445f4886fa1eefcc52b2e824006667ffb856048b6e333a0dd063c3118`.
  Earlier five-sample results prove only the parent-PID gate then in force. This
  is isolated short-window source evidence only, not a package, M1/M5,
  sleep/wake, active-operation, crash, or installed-lifecycle receipt.
- A fresh isolated current package (`0.23.9-beta`, build `2026082403`) then
  repeated that **20-sample / 41-second** observation against its exact bundled
  helper: all samples were 22,288 KiB RSS / 0.0% CPU and the verifier confirmed
  both child and descendant teardown. The same uninstalled ad-hoc bundle passed
  embedded CLI/MCP/TUI/native-surface identity, strict signature, and zero
  `*.py` payload checks. Exact component hashes are recorded in
  `PANTHEON_ISOLATED_PACKAGE_BUILD_GATE_20260823.md`. This strengthens the
  current package idle boundary only; it does not prove installed lifecycle,
  sleep/wake, active workload, crash resilience, signing/notarization, M1/M5,
  or public-release acceptance.
- `make -n build-menubar bundle install-launchagent uninstall-launchagent` →
  the compatibility build emits only `bin/pantheon-engine`, `make bundle`
  calls `macapp/build-app.sh`, and the two raw LaunchAgent targets refuse to
  mutate state. An isolated `macapp/build-app.sh` candidate completed with the
  same resource gate (22,080 KiB RSS / 0.0% CPU over five samples), codesign
  verification, and exact native Swift shell / Go CLI / Go engine payload.
- The current bundled CLI reports `Sirsi Pantheon v0.23.9-beta`; its
  `tui --help` exposes Pulse, Waste, Ghosts, Health, and Activity, while
  `mcp --help` exposes the IDE server entrypoint. `go test -count=1
  ./internal/tui ./internal/mcp ./internal/dashboard` → exit 0. The MCP test
  link emitted the existing duplicate `-lobjc` warning but no test failure.
- MCP client identity no longer leaks the retired Anubis product label. Focused
  `go test -count=1 ./internal/mcp` → exit 0; a stamped temporary CLI's live
  stdio handshake returned `[pantheon-mcp]`, exact `sirsi-pantheon`
  `v0.23.9-beta`, and `Sirsi Pantheon` instructions. Published
  `anubis://` resource URIs remain unchanged for compatibility.
- `scripts/verify-pantheon-package-identity.sh` now executes the embedded CLI's
  one-request, EOF-terminated MCP `initialize` handshake in a unique temporary
  HOME/router root and fails closed unless the server name and
  `v${VERSION}` are exact. A fresh isolated ad-hoc bundle (build `20260824`)
  passed that strengthened verifier: `pantheon_package_identity accepted=true`.
  Its package build's five helper samples were 22,208 KiB / 0.0% CPU and
  `child_terminated=true`; verifier SHA-256
  `6eb4ae64efcb5b292c3602614f268e970ead7c0548cc5e6253f315a1e3f4faa5`.
  This is still an uninstalled temporary bundle, not signing, publication, or
  M1/M5 lifecycle acceptance.
- `macapp/build-app.sh` now runs that same full package verifier after its
  resource gate. Fresh isolated development bundles passed both supported local
  modes: local `Sirsi Local Code Signing` at 22,112 KiB / 0.0% CPU and explicit
  ad-hoc at 22,640 KiB / 0.0% CPU, each for five samples with
  `child_terminated=true` and exact `0.23.9-beta` identity. Neither is a
  Developer ID, notarized, installed, or public-release artifact.
- `swift test --package-path macapp` → exit 0; 4 XCTest assertions, 0 failures.
- Current integrated verification after package/MCP surface reconciliation:
  `go test ./...` → exit 0 across the complete repository, including
  `cmd/sirsi` (56.304s), `internal/mcp` (40.969s), `cmd/sirsi-menubar`,
  `internal/sne`, and `tests/e2e`; only duplicate `-lobjc` linker warnings
  appeared. `swift test --package-path macapp` → exit 0, 4 XCTest assertions,
  0 failures. These source-level suites do not replace signed-package or live
  M1/M5 lifecycle evidence.
- Current focused CLI verification: an in-sandbox `go test -count=1
  ./cmd/sirsi` attempt exited 1 solely because `httptest` could not bind
  sandboxed IPv6 loopback (`[::1]:0`, operation not permitted). The identical
  unrestricted command exited 0 in 43.609s; the only emitted linker message was
  the existing duplicate `-lobjc` warning. This is command-level source evidence
  only, not an installed-artifact or lifecycle result.
- Current static/native verification: `go vet ./...` → exit 0, and `swift test
  --package-path macapp` → exit 0 with 4 XCTest assertions and 0 failures. The
  Swift checks cover exact bundled CLI/helper binding plus lifecycle-state and
  receipt decoding. Neither check launches Pantheon, nor does either establish
  package installation, resource stability, or M1/M5 acceptance.
- Static public-copy contract across landing, download, getting-started, FAQ,
  and release-notes sources → `public_release_copy accepted=true`; no current
  release download, cask, or source-install command remains on those pages.
- `go run ./cmd/sirsi setup --json` → no `python3` dependency in the normal
  setup projection. It also observed the installed-but-unloaded legacy menu
  label described above; the command was read-only and did not load, unload,
  alter, or restart it.

Earlier observation-only commands were `sw_vers`, `uname -m`, `pmset -g custom`,
`tailscale status --json` (self projection only), `netstat` listener inspection,
`launchctl print-disabled`, and process enumeration. No deployment, restart,
Tailscale mutation, security change, package installation, or SNE runtime
mutation was performed.

## Release decision

**Reject public release today.** The native-shell/Go-engine integration and an
isolated ad-hoc package are verified; next prove a signed clean installation,
then qualify it under sustained M1/M5 load. Do not rewrite Go in C++ or Rust
without profile evidence of a specific engine hotspot and an ADR.
