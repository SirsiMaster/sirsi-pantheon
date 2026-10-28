# Traceability matrix — Pantheon PT

Process-to-diagram coverage is maintained separately in
[`DIAGRAM_INDEX.md`](DIAGRAM_INDEX.md), with source Mermaid in the linked
`DIAGRAMS_*.md` files. Process IDs (`PT-P01`–`PT-P84`) link each row in the
component catalog to its requirements, stories, owner, source state and known
evidence. Diagram presence is not verification evidence and does not change any
row's qualification, release or certification state.

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Existing legacy client configuration has a supported selected route to the single signed router engine | `routermcpconfig*`, `setuproutermcp_darwin*`; recipe `router-mcp-selected-config-migration` | Authored pure strict-schema/byte-span, owned filesystem backup/stage/drift/debt and actual Cobra flag-routing fixtures, plus existing Horus publication regression selector | SOURCE ONLY, no execution or real config read/write. Independent review/qualification and selected installed client consumption/apply/reconnect/caller proof pending; no generic MCP replacement or full delivery claim. |
| One canonical engine and one actual registered RPC audience per router MCP connection | `internal/routermcp`, `ResolveForCaller`, shared thread lifecycle helpers; Ra/Horus recipe `router-a2a-mcp-shared-core` | Authored real registration/HTTP/session-mint isolation, foreign-marker CLI, terminal-I/O, released manifest and transferred strict completion fixtures | Source only, not executed. Review and coordinated normal/race/static qualification pending; managed configuration migration/publication/installed selected-caller proof remain separate actual work. |
| Model-router v1 routes qualified buffered/streamed answers, honors explicit lane and generation requirements beneath privacy, and retains private final-decision evidence | `internal/modelrouter/router.go`, `internal/modelrouter/decision_record.go`, `cmd/sirsi/ask.go`; ADR-054 models plane; [`MODEL_ROUTER_V1_RECIPE.md`](../../MODEL_ROUTER_V1_RECIPE.md) | Existing execution/decision-record/ask controls plus `stream_test.go`, router/provider `requirements_test.go`, `json_profile_test.go` and `ask_requirements_test.go`: structured fallback, terminal cancellation/no replay, generation qualification/output contract, alarm consent, private bounded evidence and final route | Exact24e donor has finite composed qualification; this135 source composition still requires Main's combined qualification. Real routed-request/installed handoff and empirical backend determinism/JSON support remain separate; R5 readiness keeps its independently accepted evidence. |
| Token-fenced router completion | `internal/routerstore`, `internal/router`, `internal/mcp` | PR944 mainline CI: backend-aware tests and MCP route coverage | candidate-bound; release CI receipt required |
| Preserve checkout-evidence custody without rewriting task lifecycle, leases or history | `taskevidencerelocation*`, `routertaskevidence*`; Ra/Horus recipe finite-own-task-evidence-relocation | `TestTaskEvidence*`: raw column preservation, rollback, exact replay, own-lane HTTP authorization, offline nofollow/map/hash CLI fixtures; existing optional PG harness | Source only; no tests/build/live apply by producer. Independent review, admitted qualification, publication and external reviewed application required; parent operational obligations unchanged. |
| Truthful lane dispatch and recovery | `internal/router`, `internal/routerboard`, `cmd/sirsi` | `router ping`, worker acknowledgement, ledger dispatch, and `router reopen` tests | candidate-bound; runtime receipt required |
| Refresh wake consumer declarations only after the owned child joins and is scored | `internal/router/wake.go`, `internal/router/wakeconsumer_reload.go`, `internal/router/wake_refresh_publication_test.go` | `TestWakeConsumerReloadSameControllerAfterJoin`, `TestWakeConsumerReloadInvalidController`, `TestWakeConsumerReloadRejectsRemovedAndInvalid`, and the deterministic refresh-publication write-barrier regression | Source candidate; current-head normal/race qualification pending. Refresh preserves controller identity, independently granted answer role, and failure/quota holds; invalid replacement withdraws capability. Transient registry publication failures hold new dispatch and retry without exiting the observer. No active consumer is signaled. |
| A selected unreadable Ra outbox reaches actionable Ma'at guidance without losing target or acquiring new authority | `Views.swift` `OutboxBlockerCard.guidedResolution(for:)`; `MaatCasebookView.swift` typed context; `RelayOutboxResolution.swift`; existing `SirsiEngine` relay APIs; recipe `docs/runbooks/RELAY-OUTBOX-OBSERVATION.md` | Inert `RelayOutboxResolutionTests` actual destination/projection, refresh/refusal/scope/cancel/once-only/receipt recovery plus existing `RelayObservationTests` | Corrective source only; producer ran no tests/build/live action. Independent review, admitted Swift qualification and installed selected-row/return proof remain separate. |
| Detected legacy WEDGE cannot trigger download/probe; unattended HF download is default off | `scripts/gemma/sirsi-gemma-model-resolver.sh`; Ra/Horus recipe `legacy-gemma-resolver-wedge-prefetch` | Whole-program `model-resolver-lifecycle.test.py`: preserved/absent conf, opt-in cannot override WEDGE, cached chosen/fallback default no download, literal opt-in only serving model, missing downloader warning; existing cache/ranking guards retained | Source-only successor to original wedge/prefetch obligation; inert qualification and independent review pending. No model activity, broad Gemma/cache-task closure or installed-tool activation inferred. |
| Refresh the selected legacy Horus client without broad daemon installation or weakening existing profile pins | `internal/setup/launchdhorus*`, `cmd/sirsi/launchdservice.go`, exact SSA/SHA registry grants; `HORUS_SELECTED_REFRESH.md` | Tempfile/native-double conversion, preservation, child/intent, substitution, bounded recovery, cancellation and grant fixtures; existing lifecycle suites retained | Corrective source candidate only; admitted normal/race/lint, independent review and SSA/SHA loaded-client/registration/pass/rollback qualification required |
| Interrupt an exact recognized legacy root relay without pretending its old in-memory work drained | `launchdmaintenance*`, `servicedrain/maintenance*`, `spoolmaintenance*`, `LegacyRelayMaintenance*`/`LegacyRelayPrivilegedBridge`/`LegacyRelayPreflight`; Nash d802 namespace; existing Ma'at lifecycle component; [`LEGACY_RELAY_APP_OPERATION_FENCE.md`](LEGACY_RELAY_APP_OPERATION_FENCE.md) | Existing-lock tickets/running intent/fence; unchanged agent authority; exact-release verified PKG and installed-pair readback; explicit unreadable-public-state inspection/rebind/persistence before matching resolution; bounded off-actor Installer identity and final cancel/deadline/consent checks; authored shared Go wire and native-frame fixtures | Retained6f40 qualification:203 named Go events per normal/race and21 Swift methods, separately scoped. BR03/BR04 successor is source-only and has not been compiled/executed. No actual OS authorization, Installer or service action. Native image/installed recovery remain open; fallback is not a Studio gate. |
| Disk upgrades reach the existing protected root relay without disrupting active work | `internal/setup/launchdservice_darwin.go`, `cmd/sirsi/launchdservice.go`, exact SSA/SHA registry grants; recipe `ROOT_RELAY_REFRESH.md` | Injected privilege/argv/user/children/drift/recovery/new-instance controls and normal/race evidence | Corrective candidate after published124; independent source review, supported privileged invocation, signed successor and actual loaded-client/rollback proof remain separate |
| Released135 root refresh recognizes only the canonical publisher metadata and preserves actual queue refusal | `internal/setup/launchdrelayspool_darwin.go`, `internal/routerstore/spoolobservation{,_unix}.go`, existing descriptor ACL policy; `ROOT_RELAY_REFRESH.md` | Immutable9238 deployed-layout/actual-publisher red fixtures; added metadata continuity, stale/partial/nonzero, late-arrival, shared-policy and ACL fixtures | Main reports compiled causal RED `a17376ae7d404f71931758079f6c8f9a`; correction source-only until independent review and Main normal/race/negative qualification; no host refresh or future maintenance-protocol claim |
| Pressure sampling cannot indefinitely block dispatch or multiply cold-cache probes | `internal/router/backpressure.go`; `docs/ROUTER_HOST_PRESSURE_UNKNOWN_POLICY.md` | Separate three-second native command contexts; bounded shared flock; nine-process known/unknown/crash controls and lock-removal negative; independent aggregate native output/time observations | Composed retained source and focused race pass; M5/Studio samples pass; actual M1 sample and all loaded-consumer upgrade continuity remain open |
| Lease ownership across remint and threadless calls | `internal/routerstore` | `TestIdentityItemOwnershipSurvivesSessionRemintSameAgentThread`, `TestCachedSessionSurvivesThreadlessInvocation` (both directions) | candidate-bound; runtime receipt required |
| Bounded Apollo tool worker claims only its nominated item and may answer only with a declared, stamped lead grant | `internal/routerstore/lease.go`, `cmd/sirsi/gemma_worker.go`, `docs/stacklab/APOLLO_21_BOUNDED_TOOL_TURN_RECIPE.md` | `TestRemoteClaimItemSelectsOnlyItsNamedRecipientWork`, `TestGemmaWorkerExactItemNeverFallsBackToOlderInboxWork`, `TestGemmaWorkerApolloAnswerRoleIsDeclaredAndStamped` | source-bound; router-service rollout and fresh live qualification required |
| One engine across supported surfaces | `cmd/sirsi`, `internal/*`, native sources | identity and package-inventory evidence | source-bound; runtime proof required |
| Project-scoped native workflows remain repairable | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | Finder selection validates a Git root/worktree and preserves the current root on error | source-bound; native test verifies worktree admission |
| Apollo/Mercury/Ra/Ma'at native workspace | `contracts/stacklab/pantheon-native-workspace-v1.json` | Swift workspace and Mercury tests; fixture captures; existing-monitor read | candidate built/tested locally; signed release and installed proof required |
| No stale activity or duplicate case action after navigation | `ObservationFreshness.swift`, `WorkspaceSession.swift`, `MaatOperationState.swift` | focused Swift expiry/cache/operation-lifetime tests and isolated keyboard/scroll interaction | candidate evidence only; installed runtime and signed release remain separate |
| Fresh Ra board read fits native interaction budget and failed first read is not an empty-fabric claim | `SirsiEngine.loadRouterBoard`, Ra view; existing Go auth-probe bound | `RouterBoardReadTests`; actual installed129 default13.08s versus1000ms-probe8.27s observations | Corrective source only, no universal latency or installed successor claim. Six inert fixture methods, strict Swift release build and signed installed readback remain required. |
| Cancelled Ra navigation cannot publish a launch error or late board as a fresh observation | `SirsiEngine.loadRouterBoard`; existing capture-to-JSON adapter and view `.task` | authored actual-entry cancellation/late-success/pre-cancel/retry fixtures in `RouterBoardReadTests`, with existing genuine failure controls retained | Source-only until qualification; no claim that the retained installed131 symptom proves a unique runtime cause or that a successor is installed. No appearance, reader protocol, or service policy change. |
| Bounded native I/O | runner and native output-bound tests | focused normal/race evidence | source lineage accepted; execution evidence separate |
| Python-free product path | package inventory and plugin wiring | copied-package inventory | open until independently run |
| Deterministic cask bytes | `internal/caskrelease/cask.go`, `cmd/sirsi/cask_release.go`, release workflow | exact renderer and remote-tap readback receipt | source-bound; lifecycle proof open |
| Development/commercial artifact separation | `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, build scripts, Makefile | explicit mode and `-dev` filename contract | source-bound; package execution required |
| Signed/notarized release | release workflow | Developer ID/notary receipt | OPEN |
| No public partial release or published-asset overwrite | `scripts/publish-release-assets.py`, `RELEASE_PUBLICATION.md`, release-artifact recipe | authored fake-gh partial/idempotent/mismatch/unknown-outcome controls; paired size/SHA256 remote readback | Corrective source only; tests unrun, no successor publication claim. Existing signed132 custody is independent. |
| Release-source preflight follows the current delegated route without diagnosing owner credentials | `internal/maat/releasepreflight.go`, release-artifact recipe | released130 signing-input pins plus corrective workflow pin; mode/identity/delegation/notary/verification removal fixtures; actual Casebook NextAction projection | Corrective source only until qualified. Literal source indicators are not shell execution/semantic proof. Public local certificate readiness and actual signing/notary/publication receipts remain independent. |
| Install/upgrade/rollback/uninstall | cask lifecycle plan | installed-host evidence | OPEN |

## Diagram-linked process coverage

The earlier partial audit contained40 processes: the original28 plus
eight command-level Ma’at knowledge workflows and four local Stack Lab
workflows found in `cmd/sirsi/maatknowledge.go` and
`cmd/sirsi/stacklablocal.go`. The prior 28-row set was incomplete and is not a
whole-canon denominator. The full census below supersedes that denominator:84 groups now have logical/data views, with applicable lifecycle/sequence/recovery
views; sequence diagrams are included for cross-actor/store/API flows.
This is structural documentation coverage only and does not alter any
qualification, release or certification state. The newly added source diagrams
are in `DIAGRAMS_MAAT_STACKLAB_LOCAL.md`.

- MCP migration/shared core: PT-P03–PT-P04.
- Evidence custody/completion proof: PT-P05 and PT-P07.
- Horus/root relay/legacy maintenance/pressure: PT-P06, PT-P08–PT-P10.
- Native runtime and hygiene: PT-P11–PT-P20 and PT-P28.
- Package, cask, publication, source observation and installed lifecycle:
  PT-P21–PT-P26.
- Exact-item worker lifecycle: PT-P27; the index links existing Ra/Apollo
  diagrams where those owned subflows apply.
- Ma’at archive search, ingestion, rebuild, detail/history/status, owner
  resolution and projection reconciliation: PT-P29–PT-P36.
- Local Stack Lab inspect/enable/disable/catalog: PT-P37–PT-P40; PT-US-04 is
  explicitly mapped on each local preference/catalog process.

Main owns the certification matrix and completion record. This matrix does not
assert a whole-wing pass; preserve Main's separate13-criterion status,
including the actual public `v0.24.140` readback and still-pending `v0.24.141`
evidence.


## Full-canon requirement and process crosswalk (2026-10-27)

This table covers the current eleven requirements. Detailed source, tests,
effect bounds, owners and next actions are reached through the84-row
[process index](DIAGRAM_INDEX.md). Source coverage is not installed acceptance.
All source-only/runtime/qualification gaps retain their named owners; no row is
implicitly DONE or waived by a diagram.

| Requirement | Process groups | Certification / remaining proof |
|---|---|---|
| PT-REQ-01 One engine | P01,P04,P11,P57,P64,P71,P82 | C04,C08: exact composition and actual surface/engine correlation, P64 passive-auth/default-snapshot-reuse controls authored over c165 and unrun here, selected reads and original observation timestamps retained |
| PT-REQ-02 Source and custody | P05,P07,P21,P23,P45,P73,P84 | C02,C04,C06,C12: immutable source and original evidence/claims |
| PT-REQ-03 Release truth | P21–26,P51,P66,P83 | C05–07: same signed pair, exact remote bytes and lifecycle |
| PT-REQ-04 Reversible promotion | P06,P08,P09,P25,P26,P45–47,P50,P60,P61,P66,P77,P80 | C07: selected supported recovery, no unknown-effect replay |
| PT-REQ-05 Safe native operations | P11–20,P28,P43,P44,P55,P64,P68–70,P80–82 | C08: installed interaction, keyboard/AX, failure/cancel/retry/postcheck |
| PT-REQ-06 Local intelligence | P19,P24,P29–36,P41–48,P51 | C09: deterministic baseline and final installed resolution without LLM |
| PT-REQ-07 Qualified peers | P14,P15,P27,P52,P53–67 | C08,C10: producer/authority/schema/freshness and supported selection, P52 RPA owner/peer source proposal requires exact origin/main pin and preserves undeclared/mismatch refusal, no alias/service/runtime authority inferred |
| PT-REQ-08 Recipe system | P13,P21,P37–40,P50,P52,P53,P65,P83 | C03,C10: component replacement and actual inspect/select/run contracts |
| PT-REQ-09 Protected hygiene | P02,P10,P28,P43,P46,P47,P49,P68–70,P79 | C04,C07,C09: safety controls, protected work, measured effects and cleanup |
| PT-REQ-10 Unified payload | P21–26 | C05: final one-engine Python-free signed inventory and resources |
| PT-REQ-11 Canon certification | All84 groups, especially P52,P83,P84 | C01–03,C11–13: semantic review, exact origin/pin, reading homes, billing crosswalk |

Pnn means PT-Pnn. Cnn means PT-Cnn in [CERTIFICATION.md](CERTIFICATION.md).
The101-package/target inventory was regenerated from committed composed candidate
`7019c112ca73e0acbcbd8b811938675e0a6d451b`:858 source,860 test and226 supporting files.
Final-head digest equality and final-source qualification remain explicit; retained
component evidence above keeps its original source/environment scope.

Native readability/menu corrective acceptance (PT-P12/PT-P13): real standard
macOS menus; existing single-window operations; first-responder editing; body
15 pt/secondary 14 pt/supporting floor 12 pt; persisted presentation-only
100–150% scaling without losing project, navigation or Apollo intent. Source
coverage is in `ApplicationMenuTests` and the shared font/menu implementation.
Actual installed menus, shortcuts and narrow-window layout at maximum size
remain separate checks before this corrective cohort is called shipped.
