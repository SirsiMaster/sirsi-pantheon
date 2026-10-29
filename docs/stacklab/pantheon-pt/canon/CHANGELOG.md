# Changelog — Pantheon PT canon

## 2026-10-29 — v0.24.143 release candidate

- `sirsi router ping` no longer reports a lane whose background loop is quarantined as "needs a human". That verdict is reached only when no attended session is heartbeat-fresh, so it now says the background loop is quarantined and no attended session is registered. A working session whose thread was reaped (a resumed session with a new PID) must re-register to be seen.
- A Claude/Codex session that resumes with a new process no longer drops off the router. The reaper used to retire an attended session's thread as soon as its old PID died, so the resumed session's heartbeats were refused and re-registering minted a second thread. Attended session-keyed threads now follow the 2-hour session lease (the same rule PID-less sessions already had). Re-registering with the same session ID renews the original thread. Workers keep the immediate OS-truth reap.

## 2026-10-27 — v0.24.142 release candidate

- Record the actual single hosted140 →141 →140 rollback →uninstall commercial cask qualification and bounded cleanup receipts; keep142 GUI/package and full certification acceptance separate.

- Registry: `rpa` / `RPA` now route to the existing attended ReprogramAnything lane `codex-sirsinexusapp` (callsign `RPA`), per the owner's direction to "register with the router as RPA". Identity, claims and consumer policy unchanged; no new worker.
- Readable native Mac workspace
- Responsive Fabric reads
- Preserve Apollo planning intent
- Truthful Ma’at assessment failures
- Stack Lab source traceability
- ADR-079 D1: `sirsi thread attended <agent>` (and the wake hold and worker deferral that share it) now credits a heartbeat-fresh interactive `claude`/`codex` session as attended. It previously also required a watch-loop process that ADR-065 retired, so no Claude lane could ever read as attended and a headless worker answered beside the live session. Stale, suspended, closed, worker and resident records still never count.

## 2026-10-27 — v0.24.141 release candidate

- Pulse deity-count correction uses the selected canonical roster, not six legacy filenames; bounded regular-file parsing preserves offline metrics and existing schema.

- Make each Fabric task count open its exact, snapshot-bound full-ledger selection, including separate blocked and stalled views and completed-task pages; preserve the selected bucket through next-page and refresh, reject changed/mismatched cursors, and keep message counts linked to the distinct inbox stream.
- Align lane headers and numeric columns, keep lane state/count inspectors keyboard-operable, and focus the selected task list after an explicit drill-down without moving focus during background refresh.
- Classify legacy relay retirement only after an independent kernel absence check and complete system job-list absence when Darwin's PID identity read returns EIO or ESRCH; preserve live, reused, inaccessible and indeterminate states.
- Parse complete Darwin system inventories with loaded-inactive PID zero and the native `-`/`(pe)` status forms, while refusing duplicate labels, unknown rows and incomplete inventories; a selected label always remains loaded regardless of PID or status.
- Require a complete, parseable system inventory containing the selected job before recording a retirement request or stopping the original service; invalid, unavailable or absent-job observations leave the original untouched.
- Admit only clean retained legacy sources at the audited 0.24.129 and 0.24.137 revisions to this existing startup-gated kernel-absence recovery profile; the modern drain route remains independent and old remote outcomes remain unknown, not drained.
- Native Ma'at history browsing now continues validated partial reads in a bounded burst, retains the latest accepted cursor on pause or cancellation, and leaves older-page navigation explicit.
- Treat a journal's retained parent directory identity independently of its link count, so creating the canonical mutation lock or another sibling does not invalidate a frozen history prefix. Keep the journal leaf's singly-linked regular-file requirement and all parent identity checks.
- Gemma worker preserves the typed lane-claim barrier and exits cleanly between joined work turns, rather than continuing its idle poll loop. Canonical thread-close failures now propagate. This source step does not add a native lifecycle controller or qualify a live handoff.
- Add a typed SSA-only native Gemma worker plan/apply/recover source route. A durable lane barrier, joined worker-owned pause acknowledgment, kernel PID absence, exact loaded image/program/plist identity and single-use start intent replace generic stop/drained assertions. Broker and model processes are outside the closed profile. Unknown outcomes retain private controller debt. Native qualification and deployment remain separate.
- Reconcile Pantheon Stack Lab canon with current native-product requirements, all-deity/module source inventory, process diagrams, replacement boundaries and criterion-to-proof certification. Preserve verified release evidence separately from final composed and installed acceptance; use established signing and mark changed reading copies stale until refreshed.
- Memory liveness and Doctor alerts now label observed counters with their actual
  UTC sampling window and GiB units. Liveness sizing copy distinguishes measured
  MLX allocation from a model-ID weight estimate and states the applicable
  KV-cache and OS-headroom formula. Session-candidate copy no longer asserts
  Claude-session causality or directs reaping. Both surfaces require fresh
  readings, exact owner/custody, and current-demand confirmation before supported
  resolution at the workload's own idle boundary. Classification thresholds,
  sizing formulas, admission, and runtime actions are unchanged.
- Refs: `internal/liveness/livenesswatch.go`,
  `internal/liveness/livenesswatch_test.go`, `internal/guard/deathspiral.go`,
  `internal/guard/deathspiral_test.go`.
- Ma'at history prefix verification now rejects physical EOF before the anchored
  boundary, even if the inode is restored before draining. A verification
  deadline with no verified cursor progress returns an error instead of an
  unchanged continuation. Windows keeps the prior bounded reader, including its
  append and cancellation semantics; frozen-prefix verification remains
  Unix-scoped.
- Align the Codex CompassPoint consumer prompt with the Claude dispatcher-binding
  contract. Keep actual-session registration and router/task coordination in
  standalone installed CLI calls outside the compute wrapper, and state that
  Thoth/canon reads use standalone permitted source-read commands outside it. Use
  the existing registered-executor Ma’at wrapper only for payload execution. The
  native cross-host command, proxy, host-pin, and public-key-authentication facts
  remain byte-identical; only “Local work” is narrowed to “Local payload work.”
  No admission or service behavior changed.
- ADR-079 step 1 (Ra first): Ra's lane is now the interactive session.
- ADR-079 (PROPOSED, rev 2): wake lanes are the interactive sessions.
- It also sets out a staged, Ra-first rollout with a pre-registered test, including a close-session negative fixture for the `ConsumerCapable`/`armed` path that D1–D3 do not by themselves resolve. This is design only: no code or registry change is included.

## 2026-10-27 — Full-canon corrective certification

- Expand native-product requirements, stories, design, security, runbook and
  qualification/release/certification traceability within the corrective freeze.
- Add all-deity/module source inventory and detailed process diagrams, owners,
  dependencies and replacement boundaries; independent review determines coverage.
- Preserve actual140 public/wing proof separately from remaining composed141,
  installed and human-publication acceptance; no source-only certificate.
- Replace stale credential-hold instructions with the established delegated route,
  mark changed mirrors stale, and label July planning as historical.

## 2026-10-27 — v0.24.140 release candidate

- Compose the exact normal merged fixes #1233, #1234, #1235 and #1236 from main `be62fe8aea4c9bd2390ca9bc3351e008c71876b4`. This entry prepares version and release notes; it does not publish, install or replace a loaded service.
- Ma’at first-use inspection has visible typed results, strict pre-creation journal readiness, content-independent invalid-row reasons, access to confirmed preservation repair, inherited-root JSON receipts and fresh separate initialization consent. Historical retained-receipt recovery does not open the unrelated journal. Refs: `docs/ADR-004-MAAT-QA-GOVERNANCE.md` and the existing Ma’at System One recipe.
- Legacy relay controller and successor share exact named-account/group membership resolution, including differing primary/effective GIDs. The capability fingerprint binds the factored policy; no-marker startup avoids the lookup. Permissions and custody checks remain. Refs: `docs/stacklab/pantheon-pt/canon/LEGACY_RELAY_APP_OPERATION_FENCE.md`. Source fixtures do not establish installed controller-to-successor adoption.
- Wake-loop consumer declarations refresh only at a joined and scored idle boundary. Preserve the controller identity and failure/quota holds; withdraw capability on invalid replacement. No active child is signaled.
- Router send JSON returns one committed result with resolved recipient and retains the committed ID on output failure without retry. CompassPoint bootstrap instructions and existing Ma’at diagnostics describe the actual registered executor and standalone coordination commands without granting permissions or changing roles/backoff. Refs: `docs/ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md`, `docs/runbooks/CONSUMER_BOOTSTRAP_ACTUAL_EXECUTOR.md`.
- Public asset trust, exact cask readback, installed client/GUI verification, fresh legacy preview/successor readback and the original operational ledger criteria remain separate release/adoption evidence. This release candidate does not assert whole Ma’at/JEV or feature-freeze completion.

## 2026-10-26 — v0.24.139 release candidate

- Recognize the separately inspected 0.24.121 source revision in the existing protected legacy maintenance preview profile and bind the selected revision into the plan digest. This records source-profile support only; it does not perform a rollover or claim the legacy worker is drained or cancel-joined.

- Declare the existing attended RPA Codex session as the metadata-only `codex-sirsinexusapp` lane; no worker, launch agent, or answer capability is configured. Refs: session `01a179d5-340d-7e72-962d-8b828ad7960e`; thread `thr-a74850a2ddb72c1e`.
- Node status now probes process and loop liveness only when both stable machine IDs are present and equal. Foreign or unidentified loop-monitor lanes with pending work are shown as unverified from this node, not falsely armed or dead-stranded. Refs: `internal/router/nodestatus.go`; `internal/router/nodestatus_hostscope_test.go`.
- Maat's create-only write helper now treats an explicit close attempt as consumed even when close reports an error, so deferred cleanup cannot close a potentially reused descriptor a second time. Refs: `internal/maat/memory.go`; `internal/maat/memory_create_or_verify_raw_test.go`.
- Restore the previously qualified Gemma maintenance claim barrier on the current router APIs. Session-bound item claims and task claims share the same lane lock; existing lease renewal and settlement continue during a pause.
- Apply that barrier to fresh task-resume leases while preserving inspection and idempotent replay of an existing lease. Maintenance pauses cannot silently become new recovery claims.
- This source change does not activate a live maintenance pause or replace a running worker. Matching controller, service, and client deployment remains an explicit operational transaction.
- Pass the bounded lane-maintenance controller context through its durable row lock and every SQL statement. Preserve existing ordinary claim semantics and durable owner/capability; cancellation never releases the maintenance pause. Add an isolated PostgreSQL held-lock fixture that requires deadline return before unlock, joins the caller on all exits and verifies unchanged custody and paused claims. The current implementation passed 20 isolated PostgreSQL tests normally and under race, the worker pause fixture in both modes, vet and lint. Live maintenance activation and worker rollout remain separate operational actions.
- Close each joined pause/claim race iteration's fixture store before the next schema reset, retaining parent cleanup as an idempotent failure fallback instead of accumulating sixteen PostgreSQL pools. The fixture server connection cap and production pool/barrier behavior are unchanged.

## 2026-10-26 — v0.24.138 release candidate

- Ma’at bootstrap uses exclusive creation and one identity-checked existing-lock acquisition after concurrent appearance, then post-flock name validation. Deterministic refusal controls and the unchanged concurrent initializer pass focused normal/race and thirty repetitions. Kernel ENOENT cause remains unproven; no store is reset or adopted.
- `sirsi router task why` on an unknown task now reports "not found" instead of an HTTP 500.
- Canonicalize owned Ma’at test fixture roots on macOS before using them as HOME. Production no-follow path checks remain unchanged. A deterministic aliased-TMPDIR regression and the original failed cases pass in normal and race modes; restoring the old helper fails the regression.
- Ask the atomic Ma’at scheduler for an explicit loaded request when the current lookup is not that caller’s valid admission, even when the caller uses its existing registered thread rather than a new executor. A foreign bounded CPU row no longer prevents the request from reaching the scheduler. Scheduler refusals, insufficient shares and foreign grants still prevent child execution; only the caller’s new grant is released.
- Preserve the canonical Fabric host observation and overview headlines while retaining the explicit on-demand memory diagnostics from the integrated fixes. Source composition only; final tests and installed readback remain pending.
- Normal supported Tier1 CLI requests gain local automatic routing with a verified
  direct-child execution fact; fabric/privileged/model authority remains separate.
  Local admission rejects elevation, pins child/group cleanup and retains exact
  uncertainty fences, while create-only phase receipts preserve prior intent.
  Stack Lab gains persisted off-default local preferences, closed provider-kind
  selection, installed-recipe reads and a reachable native root; opening it no
  longer automatically reads the fabric registry. Source-only pending qualification.
- Correct local Stack Lab's release-recovery navigation to the existing
  PantheonReleaseWorkspaceView, shared with the desktop and Control Center.
  Preserve its engine, explicit navigation, labels and normal release workflow.
  The c185 Swift compile failure is retained; its first13 qualified rows are
  unchanged inputs, not a whole-run pass. Strict Swift and remaining gates pending.
- Complete the source-screen fixture family with its required explicit empty
  host_binding in CLI input, shared contract JSON and the native copyable
  example. Preserve strict missing-key and foreign host-scope refusal cases.
  Establish a real private initialized failure-memory store in the inert update
  availability fixture and verify no pending update debt after unavailable
  releases. Production admission and decoder authority are unchanged.
- Honor the existing `router pull --json` flag with one canonical inbox-item array and `[]` for empty results. Keep the existing build filter and human output; machine-mode deferred warnings use stderr. Return reader/output errors. Source candidate with inert Cobra command fixtures; qualification and installed CLI verification remain pending.
- Expose optional exact item id on router_claim through the existing ClaimItem
  selector, preserving ClaimNext when omitted and the15-minute own-instance map.
  Invalid supplied IDs/unknown keys refuse before RPC; exact refusal never claims
  older or foreign work. No durable session-binding change or DC01 closure claim.
- Normalize typed ErrNoWork only for omitted-ID ClaimNext to a friendly empty-inbox
  reply without a session claim; exact unavailable items remain errors. Fixtures
  use chronological ID prefixes matching the store’s ID ordering, so exact-target
  selection is tested against an earlier eligible own item.
- Bind the non-executing release source preflight to the current unified
  app-update dispatch and typed retained-preflight, durable attempt, Installer
  handoff and installed-readback recovery owner instead of removed help prose.
  Add causal missing-control fixtures; source presence does not prove installed
  update/rollback or call ordering. Complete the strict source-screen JSON
  fixture with its required empty host_binding while preserving source/host
  separation and all required-key refusal cases.
- Compose qualified O2/O3, O7 producer/native recovery, private-engine summary and
  explicit diagnostics, exact-item MCP frontend, router JSON and portable stock
  prompt corrections onto published137. Preserve its Mercury factor, fonts and
  all31 Ma'at recipe components and existing owner/receipt-reader seams.
- Join the existing typed O7 attempt namespace to app-update inspection without
  relaxing unknown/corrupt/prepared-WAL refusal; add source-only cross-owner
  regressions. Preserve inherited native private-host/check/dedicated custody
  coverage alongside correlated actual-Go request interop.
- Replace obsolete O7 unrun/lost-ID canon claims with exact bounded Main evidence.
  New integrated-head qualification, installed/export/full-O7 proof remain open;
  O4 target-capability coverage is included below. No author test/build/runtime
  or publication was performed.
- Carry the complete qualified O4 exact-version whole-app rollback candidate,
  signed-target recovery-capability checks and native data-only retained-attempt
  recheck. Main qualified the donor's 11 bounded rows (124 Go named nodes per
  normal/race mode, three native methods and Linux ARM64 compile header only).
  This establishes tested target capability, not an actual installed rollback.
  Preserve unknown handoff debt and never replay Installer automatically.
- Periodic native memory reads now use `vitals --summary`, preserving capacity,
  pressure and swap while avoiding process ranking and emitting `top: []`.
  Explicit CLI vitals retains ranked evidence. The browser Host view offers an
  on-demand Inspect memory read using the same ranked producer, with truthful
  empty, retained/error and retry states, including when router data is unavailable.
- Serialize native managed app update/rollback with the existing legacy Installer
  and root-maintenance owner. Consume review before suspension, refuse stale
  consent after another operation, and retain custody until canceled work joins.
  Preserve separate memory recovery and receipts. Remove obsolete rollback view
  cleanup references; add inert bidirectional overlap regressions.
- Offer existing local memory initialization/recovery inside the native managed
  app-update flow, retain the selected update/rollback through recovery, and
  require verified store readback plus fresh confirmation before the canonical
  request. Reuse engine-owned operation and memory models; no error-text parser,
  store reset, automatic retry or Installer-success inference. Preserve the
  separate labelled legacy app/helper recovery Installer fallback.
- Stage diagnostic-result interruption fixtures at the real post-target,
  pre-commit boundary. Require the exact injected error, canonical retained target
  and governed prepared metadata before testing data-only recovery. This replaces
  early malformed commit directories that strict WAL validation correctly refused
  before target creation. Production constructors leave the private instance hook
  nil; validation and write order remain unchanged.
- Ma'at diagnostic request validation now reads flag presence from the executing
  Cobra command, avoiding a command-initialization cycle. Explicit empty request
  identities and incompatible operation selectors retain the same refusals before
  observation or submission. Request wire formats and local memory policy are
  unchanged.
- Ma’at retains the safe cause class of an uncertain executor registration and
  observes only its generated thread ID within the existing bounded recovery
  deadline. Pending absence no longer discards the original cause or triggers
  another registration. Denial, terminal/runtime mismatch and foreign identity
  checks remain immediate; existing healthy execution leases are unchanged.
- Route native diagnostic recording, including the dedicated fresh-screen action,
  through the existing engine-owned canonical request prepare/record/read/resolve
  flow. Retain uncertain request IDs, typed completed receipts and resolve-first
  debt across refresh/navigation. Discover same-snapshot paginated history without
  a128-row total cap; expose honest partial/restart/continue state. Source-only
  consumer of accepted73d API, pending Main qualification and normal-window proof;
  no installed/full-O7 or authority change claim.
- Send the canonical `--record-request` flag from the native diagnostic adapter.
  Make its inert command fixture use an independent literal flag table; preserve
  existing invocation-count and once-consent assertions. Add literal argv checks
  for all four request operations and bind native interop to actual Go-accepted
  argv. Source-only correction; no test/build/installed execution claimed.
- Ma’at history browsing now recognizes the canonical stale-snapshot refusal, keeps the visible page and query, disables the stale continuation, and offers an explicit restart without its cursor. Nonzero unknown errors, malformed responses, and timeouts remain distinct without displaying raw stderr. Partial history reports the rows and bytes examined by the bounded read. A complete all-history scan under concurrent journal appends remains unresolved.
- Bind local diagnostic records to durable, once-consumed request UUIDs before
  observation. Add exact read-only pending/completed lookup and confirmed,
  writer-serialized request fencing/data-only reconciliation after lost replies;
  never resend or infer safety from absence. Preserve historical uncorrelated
  records and distinguish fenced-not-recorded from never-submitted outcomes.
- Compose a versioned privacy-safe local-host binding with the existing Go/native
  diagnostic consumers. Preserve original canonical finding identities for unknown
  redacted checks without retaining raw check text. Historical observation-v1
  bytes and strict legacy hostname matching remain unchanged. Native correlation
  adapter/refresh integration and actual qualification remain separate work.
- Bind confirmed local Ma'at triage recording to privacy-filtered selected status
  provenance, full scoped failure-memory preflight/retained receipt and durable
  record intent. Reuse journal append-once and exact strict readback; uncertain
  delivery never automatically resends. Add data-only pending inspection and
  reconciliation, keeping pure observation available with memory unknown reported
  separately. Producer text, diagnostic paths, prompts and process arguments are
  excluded from new retained/output payloads. Authorized export, other diagnostic
  callers and installed/full O7 acceptance remain separate. Source fixtures only;
  no tests or host operations were run by the author.
- Clear a prior verified-record association when submitting a new confirmed local
  observation. A later refusal or lost reply retains observation evidence without
  inheriting the older record's success label. No record is resent.
- Retain the last bounded, typed diagnostic completion separately from readiness
  inspection, including exact reconciliation evidence and initialization readback.
  Refresh the existing query-owned System One/Decisions histories from either
  native diagnostic entry without remount, cursor replay, or health inference.
  Unknown-ID correlation remains separately owned and unresolved in this slice.
- Expose case-independent local diagnostic observation and memory readiness in
  Ma'at. Reuse confirmed first-use initialization and exact data-only record
  reconciliation, retain structured non-recorded observations on refusal, and
  consume each confirmation once. No automatic initialization, retry, repair or
  resend follows an uncertain record. Complete-reply loss before its record ID
  is captured remains an explicit protocol recovery limitation; this is not full
  Diagnostics/export or installed qualification.
- Page diagnostic restart discovery across both request and historical-record
  collections with explicit coverage, snapshot/query-bound continuation and
  changed-read restart. Preserve strict whole-store/WAL validation and exact UUID
  lookup; do not truncate or treat the first empty request page as complete.
- Finish exact retained not-submitted result WAL before absent-journal inference,
  then fence normally without observation or append. Reject explicitly empty
  request selectors before they can fall through to legacy confirmed recording;
  omitted selectors retain legacy behavior.
- Preserve critical-first local diagnostic presentation after privacy projection.
  Raw triage screens and native finding details use severity then stable ID;
  canonical System One verdict order and retained hashes remain unchanged.
  Keep the original first-critical fixture assertion and add canonical-equality,
  privacy, guidance and native display-order regressions. Targeted Go qualification
  is recorded separately; no native runtime result is implied by authored fixtures.
- Retain bounded defined scalar fields when an owned Darwin wait result is refused,
  including expected/observed PID, UID, signal, code/status and ABI offsets. Do not
  read or retain pointer/reserved bytes, argv or environment. Preserve all positive
  wait checks and original failure debt. Diagnostic source only: the5b real-true
  failure remains unresolved pending actual field readback, not a repaired lifecycle.
- Bind live ordinary child credentials/start/group to the exact unreaped Popen
  handle before payload release. Interpret Darwin's unfilled waitid UID using that
  custody, never zero alone. Keep foreign UID/group, changed/missing/reaped handle,
  ECHILD and malformed wait refusals; preserve quiescence/reap/absence and debt.
  Adds direct binding/refusal fixtures. Prior failures are retained and the real
  native probe/full local-first and Stack Lab qualification remain pending.
- Local CPU admission: complete verified quiescence after the final SIGKILL wait;
  persist typed recovery intent before the durable execution fence; atomically
  publish and resume exact cleanup resolution through archive and fence failures.
  Retain original debt bytes and refuse conflicting evidence without signal/replay.
  Adds inert boundary fixtures. Source only: native lifecycle, installed offline
  behavior and the remaining original Tier1/Stack Lab obligations are not claimed.
- Local CPU lifecycle: accept macOS's signed UID process-table representation while
  refusing foreign group members and malformed rows. Observe exact owned-child
  termination through public non-reaping libSystem waitid; never infer exit from
  libproc absence. Preserve the pre-release identity, quiescence/reap/absence gate
  and uncertainty debt. Adds inert SDK-ABI, ECHILD, identity and parser controls.
  The original1c native-true failure/debt is retained; native requalification and
  the broader original Tier1/functional Stack Lab obligation remain open.
- The existing Ma'at front door gains explicit `--scope local` for one bounded,
  same-user cooperative CPU-floor job without a router request or fabricated Ra
  session. Private intent/terminal evidence and a retained kernel lock distinguish
  local execution from fabric delivery. Shared/pod bookings and the default fabric
  path remain unchanged; transport errors never trigger local fallback. This is
  source-only partial local-first delivery, not whole offline/fleet qualification.
- Bind the existing knowledge-surface source guard to the canonical local
  refresh owner, separate supported flags and typed receipt, and the current
  bounded MCP archive reader. Check native retained-view failure wording only
  when a view exists, preserving the unread/unverified branch. Add inert
  source-mutation coverage for these routes and no-export/legacy-delegate
  refusals. Production refresh and archive behavior are unchanged.
- Keep standalone Fabric machine output to one JSON value while preserving reader refusals and selected-writer checks.
- Embed and serve the canonical Fabric JavaScript through the existing asset route, and retain a scrollable same-store detail view with Back navigation.
- Give Fabric freshness and lane drill-down their own observation while keeping memory inspection available independently of router data.
- Use the normal local executor for a configured endpoint proven to belong to this Mac and account; preserve strict remote SSH/proxy behavior for remote targets.
- Bound metadata-child and configured-account lookup lifetimes, and keep uncertain owned-child cleanup terminal rather than starting a fallback payload.
- Bound Darwin kernel identity probes to pid_t, compare the full kernel code identity, and expose only allowlisted probe diagnostics.
- Support an exact-version whole-app rollback through the existing signed-PKG, Ma’at attempt and installed app/helper readback owner. Require the selected target recovery capability and retain ambiguous handoffs without replaying Installer.
- Bind rollback fixture URLs to the full selected version and exercise actual Cobra flag parsing while restoring shared fixture state.
- These notes consolidate imported source changes from the feature-freeze branch. Qualification and installed/release status belong to the existing Stack Lab composition record and its evidence; this fragment does not assert installed or commercial release completion.
- Capture a private immutable Fabric generation and derive common task/message
  gauges and observed transitions once. Existing board-serve, dashboard and menu
  read paths consume these facts without child CLI reacquisition or independent
  task-age classification. Preserve registry/ledger lane union, including taskless
  services; missing runtime evidence stays unknown. Inclusive active contains its
  blocked subset.
- Keep board routes, router-board.v1/full task and phase details, Horus
  router-surface.v1, standalone/once, no-store/CORS/method restrictions and failed
  source/SSE invalidation. The read-only route set excludes the existing arm
  mutation. Canonical Fabric task privacy/paging and semantic colors remain intact.
- Source and inert fixtures only; admitted qualification and actual existing
  consumer readback remain required. No listener retirement or installed/release
  completion is claimed.
- Complete original Fabric R1/R2 source: bounded canonical open-task rows and
  clicked-observation detail/provenance on the existing web board; digest-bound
  continuation and explicit refresh on drift; private payload/reference exclusions.
  Share producer-assigned state roles through existing native/web brand tokens.
  Adds inert privacy/paging/interop/role regressions. Source-only, not executed;
  installed adoption and Nexus/Horus convergence/conditional8734 retirement remain
  open and are not inferred from completion mail or prior ce8 qualification.
- Complete existing FabricBoard joins for canonical leased runtime, measured host
  facts and real FleetTracker status transitions. Current web and native headline
  projections consume the same stream-separated producer. Unknown observations
  remain unavailable, known zeros remain values, and one-shot/first-read activity
  is explicitly unseeded. Source-only candidate; independent/native qualification
  and foreign consumer mapping remain open. No new router store, service or policy.
- Commit authenticated inbox claims and their session ownership in the same
  store transaction (DC01). A failed owner-binding write rolls back the item
  lease and claim counter instead of acknowledging a usable unbound claim.
  Existing local/unclaimed and delegated close compatibility is unchanged.
  Source qualification and installed backend adoption are separate evidence.
- Refs: docs/ADR-068-ROUTER-A2A-MCP-FRONT-END.md,
  internal/routerstore/README.md.
- Preserve the existing no-complete-commercial-release update/recheck promise
  when composing the additive whole-app rollback recipe. Keep its existing
  installed-product preservation behavior, source guard and all rollback outputs.
  This corrects recipe metadata; no update, Installer or runtime behavior changes.
- Remove personal identity, private portfolio, retired agent lanes, and aspirational
  service dependencies from the native Ask Sirsi stock prompt. Clarify Ra/Horus,
  Hermes protocol/Mercury transport, Ma'at/Seshat, and evidence-bound optional model
  roles. Execution, routing, permissions, schemas, and root lookup are unchanged.
  Source-only candidate; no model invocation or compile qualification claimed.

## 2026-10-25 — v0.24.137 release candidate

- Preserve paragraph-form notes, complete-entry duplicate identity and unchanged
  versioned history through release preparation; regression fails before the fix.

- Each in-process Stack Lab accessibility fixture now first reads independent
  public SwiftUI Text/Button controls and presses the actual control once. If that
  independent oracle is unavailable, the fixture reports an explicit diagnostic
  skip before inspecting product content. With a working control, all original
  content, handoff and scrolling assertions remain failures when unmet.
- No product view, recipe content or accessibility representation is replaced.
  Non-AX semantic tests remain active. Skips do not establish accessibility
  completion: exact-source external evidence is separately required for all29
  Ma'at components and scrolling/Apollo/Ma'at handoffs, release handoff, short wing
  and full component fields. Existing finite36e9 external receipts and prior
  644/239 failures retain their original scopes; no new runtime proof is claimed.
- Preserve Mercury's offline/invalid/partial read states and individually fresh partial observations, while retaining unknown aggregates and independent local aging. Manual and periodic refreshes now share one visibility-bound owned task; superseded/cancelled native reads cannot publish a late result. Add an actual Go mixed-freshness export for native/web parity qualification.
- Restore the retained read-only Mercury web and native hardware/cable presentation over the canonical Go snapshot: shared headline, partial eligibility, local expiry, run/calibration/provenance detail and explicit unknowns. Preserve strict same-origin web assets, owned read cancellation, reduced motion, separate Ra/transport evidence and Power Unknown. Map all four existing consumer components and correct stale native-endpoint canon; visual/installed parity remains unqualified.
- The Mercury web renderer no longer retains a prior read's activity lamps when
  the current Go projection supplies an empty/null lamp vector. A fresh idle null
  vector clears prior lamps; a missing vector is explicitly unavailable rather
  than inferred off. Current arrays and existing aggregate eligibility remain
  authoritative.
- An actual embedded-renderer active-to-idle regression covers null, missing and
  empty vectors, zero active cables, idle context, withheld hero cells and stopped
  motion without a clock-triggered fetch. Both existing DOM fixtures now implement
  the production mount's appendChild call with node ownership/return semantics;
  their original cancellation/expiry assertions are retained. The frozen5d53
  attempt's four passing Node cases and two pre-render fixture failures are not
  reclassified. No new execution, Swift change or whole G1–G5 completion claimed.
- Mercury's web activity lamps, active-cable count and context now use the same
  existing aggregate eligibility as its headline measurements. Direction expiry,
  partial reads and envelope expiry withhold current aggregate claims while
  individually eligible directions remain visible. The local aging clock does not
  fetch a new snapshot.
- Native workspace reads now bind loading cleanup and evidence publication to
  their owning refresh invocation. A canceled older read joining its children
  cannot clear a newer refresh's loading state or replace its evidence/errors.
  Canonical monitor generation and structured child cancellation remain intact.
- Direct embedded-renderer and workspace-owner regressions are authored only.
  This source successor preserves the original G1–G5 scope; qualification, visual
  fidelity/accessibility, configured recovery and installed/per-Mac obligations
  remain separately open. No transport/reader/schema or service policy changes.
- Expose Local memory recovery with an always-visible Inspect control at the
  ordinary Ma'at workspace root, outside Casebook loading, empty and error states.
  Reuse the existing engine-owned prepared-receipt inspector, exact confirmation
  and recovery refresh callbacks. No selected case, synthetic journal row or
  manual digest is required. Add production-root entry/callback fixtures with
  no MaatCase, preserving prior component tests and private custody policy.
  Source-only pending qualification and separate visible-window proof.
- Retain and verify complete Ma'at preflight receipts before Casebook projection and managed-watch admission, without receipt bookkeeping staling its own semantic registry snapshot. Native historical receipt inspection preserves prior evidence on unavailable, mismatched or cancelled reads; historical results never authorize a new action.
- Resume only exact interrupted Ma’at receipt writes before reassessing eligibility, and expose separately confirmed historical recovery for complete retained targets. Verify private current-owner custody on receipt and WAL reads without changing modes or granting an action.
- Fix recovery reachability when the first preflight receipt is fully retained
  but its write commit fails before Casebook projection. The existing memory
  inspector now discovers and displays verified complete historical bytes and
  their exact intent/store binding. Native liveness setup offers separately
  confirmed recovery through the canonical receipt command without a Casebook
  receipt row or manual digest lookup. Missing, changed and unrelated debt stays
  unavailable. Recovery creates no decision, attempt or installation and requires
  fresh ordinary assessment and consent afterward. Source fixtures are authored;
  qualification and installed workflow evidence remain pending.
- Own-lane cause-fixed recovery accepts an inactive blocked original with
  ordinary attempts remaining. It consumes the next attempt with the existing
  authenticated transaction, evidence audit and row-version fence; it never
  resets history or marks the task complete. Pending/in-progress below-ceiling
  tasks retain ordinary claim semantics. Focused qualification passed; protected
  integration, independent review, release and real-row deployment remain open.
- Refs: docs/stacklab/maat/canon/OWN_BLOCKED_TASK_RECOVERY_PLAN.md;
  docs/ADR-061-DURABLE-CONTINUOUS-EXECUTION-ENFORCEMENT.md.
- Complete the local managed-watch failure-memory loop with typed direct-attempt intake, stable-key recurrence prevention and exact local owner disposition. Native Casebook reviews full retained evidence, confirms “Accepted — not repaired,” reconciles interrupted data-only phases and returns fresh observations to the existing controlled resolver. Unknown actions stay pending and installers never replay during reconciliation. Source candidate; qualification and remaining operational rows stay explicit.
- The optional legacy root-relay maintenance app path now binds each reviewed
  operation to a controller-issued durable ticket and monotonic generation under
  the existing service lock. Explicit resolution records a matching result or
  fences delayed arrival before offering fresh consent. Old or empty status never
  clears a newer unknown receipt. Cancellation preserves uncertain outcomes;
  recovery, abort and replacement restoration require their own captured tickets.
- Protected-engine preflight is bounded and checks the installed helper's native
  protocol/release against the signed app before authorization. A reachable app
  action prepares and rechecks the exact running release's normal PKG, including
  published SHA256, package trust, replacement metadata, payload and helper
  compatibility, before opening macOS Installer. Installation and fresh installed
  pair verification remain separate; a DMG handoff or an older incompatible PKG
  is not substituted.
- This is an unqualified source successor to the retained 54d bridge review and
  failed fixture-compile attempt. Authored inert fixtures are not execution or
  installed-recovery evidence. Existing modern service/agent authorization and
  producer-covered Studio maintenance remain unchanged; this fallback adds no
  Studio rollout gate or automatic service action.
- Add a narrow one-shot macOS-authorized app route for legacy root-relay
  maintenance. Keep the GUI unprivileged and execute only the qualified protected
  installed engine with closed public request data and a minimal environment.
  Preserve existing registered-agent authorization, controller/journal fences,
  LR01–03 recovery, captured restore bytes and unknown-outcome debt. Retain model
  state across workspace remounts. Missing or drifted protected installation stays
  unsupported with an in-app normal signed-PKG prerequisite; no bundle/PATH root
  fallback, daemon, router DB or credential forwarding is introduced. Inert
  quoting, denial, frame, identity and lifecycle fixtures accompany the source;
  actual administrator/service execution is not claimed.
- The legacy relay panel now offers authoritative public-binding inspection and
  explicit confirmed reconciliation when its saved public receipt is unreadable.
  It recovers an exact controller binding or obtains a new serialized preview
  reservation for the selected retained payload. The public binding is stored and
  read back, then the existing matching-result/fence control is required before a
  fresh preview and new service consent. Empty status never resets uncertainty.
  Unreadable-byte count and bounded-prefix digest remain labelled as unknown past
  outcome; no manual preferences deletion or service replay is requested.
- Installer preparation and final verification now share the bridge's bounded
  preflight mechanism. Signed-app and bundled-engine identity checks run off the
  UI actor; deadline, cancellation, generation and consumed consent are checked
  before command submission and before opening normal Installer. Late read-only
  probe completion cannot revive an expired operation.
- Direct inert fixtures include the actual controller's empty wire shape and
  late-ticket refusal, public persistence/readback failure, changed bindings and
  stalled Installer identity. This source successor preserves6f40's separately
  qualified203+203 Go events and21 Swift methods without treating them as proof
  of these new paths. Real OS authorization, Installer, native image/service and
  installed recovery remain separately unproved. No Studio rollout gate added.
- Add an exact root-relay legacy maintenance transaction and startup-held producer
  protocol, separate from ordinary/producer-covered refresh. Private durable phases,
  selected instance/artifact bindings, positive retirement checks and quarantined
  unknown envelopes support explicit transaction-bound recovery without replay or
  old-work drain claims. Add inert controller, command, journal, custody, startup,
  wire and physical admission fixtures. Native loaded-image binding, application
  root-host integration and installed qualification remain open; this fallback is
  not an additional gate on the supported Studio route or published135.
- Route legacy maintenance authorization through the selected protected relay's
  existing direct canonical configuration, in memory only, so a pending spool hold
  does not block fresh owner/grant verification. Preserve the registered actor,
  thread, native runtime and exact lifecycle-service grant contract; retain ordinary
  refresh authority behavior. Add inert protected-configuration, real owned HTTP
  session and current-runtime/grant fixtures. Privileged native app-host execution
  remains absent in the baseline; no administrator prompt, daemon or fake GUI
  identity is substituted. Add shared global custody dimensions, durable atomic
  prepare-before-journal startup holds, marker-cleanup generation recovery,
  pre-retirement abort, and exact replacement restoration with retained backup/stage
  debt. Casebook consent binds captured plans and exact transaction identity; the
  host refuses ordinary user-process operation rather than acquiring privileges.
  Source candidate with initial compiler feedback, not installed qualification.

## 2026-10-24 — v0.24.136 release candidate

- Return the existing typed lane-scope authorization error before task recovery
  validation or transaction entry. The actual private4ab cloud canary refused
  foreign authority with HTTP500; authenticated scoped/operator inspection and
  claim controls now require403 and unchanged own/foreign task rows and audit.
  Preserve that failed canary and its signed artifacts; no public release or
  host adoption is inferred from the source correction.

- Retain the concurrently merged bounded PostgreSQL pool configuration:
  default4, explicit1–64, invalid values fall back4. Pool sizing is not evidence
  of deployed capacity, outage recovery or additional database authority.

- Establish private0700 positive failure-memory fixtures explicitly under public
  test umasks; preserve production policy and a public0755 refusal regression.

- Canonicalize positive archive/CLI fixture temp roots only; preserve the product
  no-follow boundary and explicit symlink-home refusal. Retain the failed normal
  protected push and aliased-versus-physical causal control as separate evidence.

- Prevent relay forwards from automatically following redirects, so a later redirected TLS/dial failure cannot replay an original POST already received by the canonical service. Preserve the shared transport, pins, timeout and client policy; add actual307/308 no-replay controls and join owned stalled-handshake fixture cleanup.
- Reconcile the cumulative commercial release source verifier with the explicit private-draft POST publisher and retained creation identity, preserving tag/source binding, paired readback, Apple trust and no-clobber checks; add inert full-verifier mutation controls for the executing creation invariants.
- Retain the controlled diagnostic workflow in its existing navigation frame so Back preserves pending/unknown re-screen receipts and once-only write custody; keep disappearance cancellation and fresh-read checks unchanged.
- Remove an unused private history reader wrapper; retain the counted reader and all history bounds unchanged.
- Open canonical local System One findings in Pantheon's existing controlled resolver after a fresh identity-bound diagnostic read. Preserve the original case, dedicated repair custody, separate repair consent, and a once-only confirmed fresh-screen return path; never execute retained recovery hints or infer resolution from re-observation.
- Compose the independently qualified archive, operational watch, credential observation, native partial-receipt, model-routing, original-task recovery and release-publication deltas on released 134 plus Ra's merged fleet-check. Preserve current package modes and normal release custody. Combined tests, independent acceptance and commercial release are not inferred from donor qualification; original ledger obligations remain open until their full proof is satisfied.
- Recognize the exact canonical relay health summary as validated root metadata
  during supported root-relay refresh. Preserve real queue checks and unknown
  entry refusal; allow validated atomic summary replacement between bounded
  namespace observations, while retaining publication staging as busy/debt.
  This source candidate has no installed-success claim. Main retained the
  deployed-layout and actual-publisher ENOTDIR regression on test-only `9238be90`.
- Relay: a stalled TLS handshake is recognised by net/http's own error, not by its text.
- Retain the exact GitHub draft ID returned by one authenticated creation,
  then verify that selected draft directly instead of rediscovering it through
  changing release-list pages. Preserve full bounded absence proof before
  creation, recheck the remote tag, require HTTP201 and exact private ID/tag
  fields, and retain unknown creation outcomes without another create/upload.
  Keep private paired-byte readback, Apple verification, publish-last ordering,
  public-byte readback and no-overwrite behavior. Inert regressions cover the
  observed post-create ambiguity class and malformed/unknown/drifting identity;
  actual service acceptance remains proof for the next normal release.
- Correct the a622 source-review findings: map optional `json_object` from the
  existing selected Pantheon profile to three-state capability evidence for
  that exact endpoint/model, and preserve `--route` consent when measured
  alarms stop model inference. Format/determinism flags alone do not construct
  the provider ladder or bypass the heuristic stop. Inert fixtures cover the
  real profile/resolver/Cobra/HTTP path and selection isolation. No installed
  profile, SNE schema/service, credential or deterministic wire mode changes;
  qualification remains unrun at this source checkpoint.
- Restore original optional model-router JSON-generation and determinism inputs
  with explicit unknown/unsupported capability refusals, shared effective
  requirements across routing/override/fallback, direct-provider checks, and
  redacted requested/effective mode evidence. `ask --response-format=json-object`
  is distinct from the unchanged `--json` envelope. JSON+stream is refused;
  malformed/incomplete JSON never causes replay. Existing configured backends
  gain no inferred capability or invented determinism wire mode. Source-only
  fixtures and qualification preparation do not establish installed completion.
- Put the inert ask-generation fixture helper's error return last and adjust
  its callers to satisfy ST1008. No production code, test inputs or assertions
  change. The prior b3 qualification remains retained as failed at lint; this
  source-only correction does not claim a new qualification result.
- Preserve context-window qualification across local, remote-escalation and
  judgment routes, including malformed-class and negative-context refusal.
- Preserve Cobra-selected human/JSON writers and themed output while adding
  the new generation and streaming paths.
- Surface synthesized stream EOF/cancellation once after partial output,
  retaining decision-journal failures without fallback or replay. Combined
  release qualification remains separate from the donor's finite evidence.
- Execute the closed typed read-only `maat.active-incident/v1` recurrence guard
  only for verified active exact-scope incidents. Bind evaluated guard identity
  to embedded implementation/dispatch source, not a hash of a caller's label.
- Unsupported matching IDs/versions return unverifiable. Terminal/out-of-scope
  incidents do not execute guards or appear as measured checks; mixed outcomes
  retain actual checks without claiming complete evaluation. No incident rewrite,
  imported command, host probe, new service or authorization is introduced.
- Open catalog leaves nonblocking before retained-FD regular-file and bounded-read
  checks, refusing FIFOs without waiting for a peer.
- Author dispatch/scope/digest, FIFO/size, reopen/failed-write debt, independent
  concurrency and journal-failure regressions; update the StackLab component and
  upgrade contract. Tests/builds have not been executed by this source producer.
  Historical342 receipt custody, all eight authoritative operation integrations,
  wider surface convergence and installed proof remain separate/open.
- Bind the existing closed Ma'at repair to the canonical running engine and
  ordinary HOME through the shared local plist, installer and inspector APIs;
  remove its developer-checkout dependency without changing legacy routed run.
- Preserve ordinary owned 0755 `.sirsi`/`maat` ancestors without chmod or
  sentinel changes, while keeping the actual failure-memory root private;
  recheck the complete EDA snapshot under the attempt-admission lock.
- Route repair review through the bounded local inspector, including exact
  legacy/unloaded metadata findings while refusing disabled or unknown state;
  retain historical projection separately from a fresh current observation.
- Make `liveness-watch status` require the exact local configuration and loaded
  state rather than plist-file presence alone. Source-only; qualification and
  installed behavior remain unclaimed, and the other seven Ma'at operational
  rows remain open.
- Keep standalone persistent installation on the same packaged-engine
  resolver/verifier, and accept bounded signed `launchctl list` status values
  while continuing to refuse malformed tables.
- Correct the default-store existence probe to use the actual one-result
  `unix.Fstatat` contract without changing its ENOENT/refusal checks.
- Explicitly set liveness `OpenStore` test roots to 0700; preserve the
  production private-root validator unchanged.
- Assert the native read-only probe sequence including both independent
  `launchctl print-disabled` observations; forbidden operations still fail.
- Validate HOME ownership from a no-follow native directory descriptor rather
  than asserting an `os.Lstat` platform `Sys` value is `unix.Stat_t`.
- Bind the existing managed liveness-watch repair to the fixed per-user Ma'at
  failure-memory store, exact reviewed action manifest, unchanged exact-scope
  preflight, durable attempt, and fresh exact post-action observation.
- Add explicit no-import first-use initialization and fail-closed inspection of
  interrupted, missing, unbound, or corrupt store state. Native Casebook presents
  confirmed initialization and safe inspect/re-observe controls through fixed
  canonical-engine arguments; it does not direct the operator to Terminal.
- No SNE source authority, shared interchange schema, other operational row, or
  model/credential behavior changes. This is source-only and unqualified; the
  other seven integrations and installed proof remain open.
- Fence both native Ma'at history loaders by refresh generation. Delayed or
  cancelled responses cannot replace a newer page or query; current failures
  retain the latest committed projection and its matching continuation.
- Keep Ma'at history filters ahead of bounded page selection, preserve explicit
  empty-array and query-versus-after-cursor provenance, and continue exact native
  action reads only across the same query and snapshot. Owner acceptance can
  locate an old exact review through a separate bounded strict journal scan;
  tolerant history cursors never authorize mutation.
- Retain exact native read progress across explicitly continued interactions,
  bind it to the selected action generation, and keep raw evidence multiplicity
  separate from hidden relationship rows. Charge selected-record rereads and
  limit finalization to the remaining budget or one bounded dependency bundle;
  empty, incomplete, after-cursor and failed-refresh states remain distinct.
- Project binary updater outcomes and unresolved attempts through the same
  Casebook overlay before history status/text filtering and page selection.
  Preserve binary-only repair success separately from other findings and failed
  floors, and retain typed inspect/re-screen routes and receipt search.
- Owner acceptance keeps its strict journal mutation path. Source fixtures and
  static inspection do not qualify installed behavior or execute an updater.
- Carry the merged Ra TLS-handshake retry/outbox correction into the full
  corrective candidate. Keep its complete seven-input scope and unresolved
  runtime obligations active while SSA publishes the separately ready fixes
  through one normal commercial release train. The next released baseline is
  the starting point for subsequent integration, not permission to discard
  unfinished application work or relabel diagnostic passes as repairs.

## 2026-10-24 — v0.24.135 release candidate

Ready135 qualifies the merged TLS/fleet corrections, bounded release helpers,
signer-alias defaults and explicit public-mode fixture setup. The assembled
older fragments below retain their historical source-only or partial-evidence
limits; this consolidation does not newly qualify every feature or complete
the larger seven-donor Ma'at composition, which remains active separately.

- Use the existing pinned `sirsi-m5` and `sirsi-m1` SSH aliases as ordered signing
  client defaults instead of DHCP addresses. Preserve explicit host overrides,
  strict noninteractive transport, enrolled-service checks and signature checks;
  do not enroll hosts, change trust or launch signing services. Denial-only
  causal fixtures cover unset/empty/default/override selection and reject the
  former DHCP defaults without contacting any signer.
- Run router package setup/tests in a returning helper, then exit outside it,
  allowing both owned temporary-root cleanups to run after passing/failing tests.
- Register cleanup immediately after each creation, retain directory identity,
  refuse substituted paths and report cleanup debt. Preserve nonzero test exit
  codes; cleanup failure turns success into failure. No atomic same-UID race or
  abrupt-process-termination cleanup guarantee is claimed.
- Add actual-test-binary subprocess fixtures under private TMPDIR, plus inert
  setup/removal failure callbacks. Reaper/grant stubs, DB isolation and dispatch
  override stay unchanged; no dispatch fallback or runtime API change.
- Move the existing router A2A MCP implementation into one `internal/routermcp` library, exposed through canonical `sirsi router-mcp` and a thin legacy launcher. Preserve bare tools/schemas/resource, strict completion and lease-expiry/fencing; isolate local registration/claims per instance and join heartbeat cleanup on EOF/cancellation without library process exit. Generic `mcp` is unchanged. Add actual protocol manifest and inert lifecycle/entrypoint fixture source, transferring the real completion-helper matrix. Managed-client config migration, explicit RPC-audience binding and installed-caller proof remain open; no second packaged engine or full MCP rollout claim.
- Add a separate macOS router MCP configuration plan/apply command for one explicitly selected existing Claude Code top/project or Cursor stdio entry. Only the exact owner-local standalone Go payload with empty argv can migrate to the existing qualified signed installed `sirsi router-mcp`; generic MCP, env/cwd/settings, credentials and server authority remain unchanged.
- Bind confirmation to raw bytes, selection and source/target identity; preserve unedited bytes, redact preview values, and reuse retained nofollow/create-only backup/staging publication with durability/readback and explicit recovery debt. No client restart, automatic restore or live configuration action. Source fixtures are authored, not executed; actual client selection, signed publication and reconnect/caller proof remain open.
- Bind each router MCP instance's tools and post-registration heartbeat/close to its actual registered agent/thread using the existing authenticated MintSessionForThread route. Explicit callers never borrow ambient identity or disk session caches; canonical marker recovery no longer exports process environment. Supply a unique real registration ID per instance without changing ordinary registry reuse policy.
- Unwind the router/context stdio lifetime on malformed/read errors, failed response writes and short writes, before another request executes. Preserve generic MCP framing behavior and existing thread/lease/completion authority. Add owned remote audience/same-PID, mismatch and terminal-I/O regression fixture source. Managed-config migration and installed-client qualification remain outstanding; no tests or runtime actions are claimed by this source correction.
- Route the actual router MCP work-board handler's durable item and thread reads through its instance-bound store and the existing shared projection/loaders. Preserve public board shape, pace calculations, best-effort peer loading and legacy file-mode behavior.
- Keep post-registration heartbeat/close on the registry mode selected for the connection: explicit StoreWake=0 remains the existing JSON compatibility lifecycle with exact owned-target fencing, not forced durable resolution or authenticated RPC isolation. Add actual board-handler audience assertions and real owned JSON registration/heartbeat/EOF-close/sibling-preservation fixture source. No tests, build, runtime cutover or installed completion is claimed.
- Accept only the two observed literal six-argument profiles ending with
  `/var/sirsipantheon/relay` or `/private/var/sirsipantheon/relay`.
- Do not normalize paths, resolve aliases, rewrite plists, or change flags,
  authorization, root/artifact/instance bindings or the retained physical-spool
  no-follow/quiescence probe.
- Add explicit positive profile and lookalike/escape/alias refusal fixtures using
  existing native doubles. Source-only; main owns admitted qualification.
- Align Ma'at's release-source preflight with the released130 shared notarization helper, selected signing keychain, and enrolled signing-client delegation instead of obsolete inline-command and temporary-keychain prose. Explicit artifact modes, direct signing identity/credential alternatives, awaited fail-closed notarization, and returned-artifact verification remain observed controls.
- Source repair guidance no longer infers missing owner certificates from missing source text. Public local certificate readiness and actual release/signing/notary/publication proof remain separate; no packaging, credentials, or release policy is changed.
- Replace miniature source stubs with owned copies of the hash-pinned published130 release-route files; add causal missing-control and Casebook recovery regressions. These fixtures observe source only and execute no signing, notarization, service, or network command.
- Recognize the selected-keychain file check's actual backslash-newline continuation together with its fail-closed absence branch; removing either control remains a causal source refusal.
- Stage the commercial DMG/PKG pair in a private release draft, reverify the
  existing Developer ID/staple boundaries and exact remote SHA256 bytes, then
  publish. Published retries are read-only exact-byte checks; partial or
  mismatched public records refuse without asset overwrite. Preserve canonical
  Go cask/build/signing authority and always-run keychain cleanup. Add inert
  fake-gh regressions and a retained per-attempt verification receipt.
- Use codesign's required `=anchor` inline-expression argv and keep publication
  state unknown/null until a remote state is actually observed, including
  durable receipts for early verifier or lookup failure.
- Source-only corrective checkpoint; no actual release, signing, notary,
  installation, tests or build execution is asserted by this fragment.
- Discover private release drafts with an exact id/tag_name/draft projection on authenticated100-record list pages, preserving the ten-page/1000-record coverage and one-MiB projected-response limits. Selected-ID/tag metadata and assets remain full; complete absence, exact draft identity, publish-last readback and no-overwrite rules stay unchanged. Incomplete or oversized discovery still refuses.
- Invoke official package-mode, PackageInfo and publication Python helpers with explicit `-B`, preventing imported helper bytecode from dirtying release source before canonical Go cask checks.
- Add inert heavy-history/discovery refusal coverage and a real copied-helper fixture proving no bytecode side effects without an ambient environment bypass. Source fixtures are not live publication proof; SSA's existing134 recovery remains separate.
- Relay: a stalled TLS handshake is retried and queued, not reported as an unknown outcome.
- Align the node-status held-outbox fixture with the existing private relay observation contract: explicitly owned0700 root/lane/outbox,0600 inert record and valid root-bound metadata summary. Preserve the no-health-error, held-count and read-only assertions; pin store-wake off and trust-group empty so fixture collection cannot use a live pending-store client. Add explicit missing-publisher/direct-coverage and nonprivate-root refusal controls. Production observation, privacy, no-follow and ACL policy are unchanged.
- Separate GUI filesystem access from relay-owned retry observations. The existing relay loop publishes bounded, versioned, per-lane name/count metadata under independently validated producer/root policy and the exact declared reader group; private outboxes and request bodies remain untouched. Missing, stale, malformed or partial evidence never becomes zero or a complete aggregate, and metadata failure does not stop transport.
- Ra shows fresh service-managed private outbox counts without offering an unsupported local repair. Refresh revalidates producer time; eligible same-user repair uses typed command results followed by separate forced-board verification. Missing publishers require an owner-managed update, not automatic restart or permission changes.
- Add owned/inert Go and native regression fixtures plus the existing Ra/Horus Stack Lab recipe inventory. Source only pending Main qualification; no installed or all-host delivery claim.
- Retain expired direct-readable retry rows as unknown evidence, and track incomplete inventory independently of optional publisher availability. Freeze repair confirmation to agent/canonical-spool/root identity; changed offers launch nothing, and other-root readback cannot verify an approved repair. Add direct R1/R2 regressions without changing the Go repair authority.
- Carry native consent's root device/inode into the Go retained-descriptor mutation boundary. Refuse same-path valid-root substitution before chmod; preserve legacy unpinned CLI compatibility and existing ownership policy. This remains a cooperative check, not hostile same-UID atomicity.
- Inspect Darwin ACLs through the supported libSystem `fgetattrlist` SDK bridge, not a direct syscall. Darwin without cgo fails closed; owned native fixtures cover nonempty ACL refusal and retained descriptor binding.
- Preserve the RDMA legacy sysctl command status rather than treating nonempty
  failure diagnostics as loaded-family evidence. Trust only successful output
  matching the retained historical date/time and RDMA_family version shape;
  unfamiliar output falls back to the validated kmutil inventory. This is a
  conservative compatibility recognizer, not a newly verified native format.
  Shared command handling, NVRAM reporting and device qualification are unchanged.
- Validate the observed kmutil loaded-kext table header and every row before
  reporting RDMA-family presence or absence. Match the exact family bundle ID;
  malformed successful output and command failures remain unknown with a reason.
  Permit only the exact observed release-variant notice before the table.
  This read-only telemetry correction changes no NVRAM, enablement, hardware
  support, or transfer qualification. Fixtures preserve observed native table
  shape with live load addresses replaced by representative constants.
- A cancelled Ra board refresh no longer publishes a bundled-reader launch error or a late board after navigation. The previous board, observation time, and error remain unchanged; loading is released and a fresh retry can read normally. Real uncancelled launch, timeout, exit, and decode failures retain their recovery behavior.
- Add actual `loadRouterBoard` entry fixtures for cancelled capture mapping, non-cooperative late success, already-cancelled refusal, and successful retry. These use in-process held reads only, not a router, service, or model. This source correction does not itself establish the exact runtime cause or installed successor qualification.
- Correct the cancellation fixture's failure-message spelling to satisfy the repository lint contract; assertions and provider behavior are unchanged.
- Validate the bounded JSON completion envelope returned by the existing one-token readiness probe. HTTP 200 alone no longer marks a provider ready; malformed, error, empty, unterminated, trailing-data, oversized, cancelled, or failed completion responses remain unavailable. Valid one-token `length` and reasoning-only terminal responses are accepted.
- Add inert HTTP fixtures that exercise served-model discovery and the real readiness POST together. These fixtures are authored evidence and remain unqualified until the repository owner runs them.
- Keep the bounded response read compiling, and make the cancellation fixture prove POST entry, cancellation observation, and bounded cleanup of its coordinator.
- Add a finite own-lane task evidence relocation operation for already-preserved
  local artifacts, without generic task-update widening or task lifecycle,
  timestamp, accounting, history, scope or lease changes. Exact public-row CAS,
  transactional provenance and exact-manifest/after-state replay refuse drift.
- Default CLI planning checks a nofollow preservation map and replacement hashes
  offline; explicit inspection/application uses the existing configured canonical
  service or relay, including localhost, never an alternate local database.
  Review references and artifact hashes are authenticated actor attestations,
  not server-certified review/filesystem proof. Source fixtures and recipe added;
  execution, publication and any reviewed live repair remain separately admitted.
- Preserve relocated references across ordinary legacy task updates: capture
  raw links with the decoded row, CAS both writers, and boundedly reload/remerge
  reference-only conflicts server-side without new request fields or duplicate
  accounting. Retain status/lease refusal fences, including concurrent renewal.
  Relocation refuses all retained ownership, including expired custody, without
  clearing it: a prior-live delayed completion can otherwise still land. Canonical
  server-writer deployment/compatibility must precede future application; ordinary
  older clients are not gated or required to adopt new flags/roles.
- Bind confirmation to current semantic diagnostic evidence, exact argv and repo,
  including the closed legacy repair path; revalidate immediately before launch.
- Capture each ResultView invocation once; retain non-JSON/error output instead
  of running the command a second time. Refresh/recheck does not replay mutation.
- Share a 4 MiB stdout/stderr budget with nonblocking bounded stdin and monotonic
  capture deadline. Partial output never decodes as success; complete nonzero
  typed reports retain their existing consumer semantics.
- Report owned-child exit unknown after bounded TERM grace without claiming
  descendant cleanup; consume the particular fresh diagnostic read for current
  health, never infer causal repair from stale shared state.
- Add owned inert fixtures and the Stack Lab remediation execution contract.
  Tests/builds and installed UX qualification are unrun; no rollout/completion
  claim. Native visual identity, typed mappings and Go authority are unchanged.
- Independent-review corrections bind offered follow-ups to the report-producing
  directory before consent and fence verification publication by generation.
  Two deterministic regressions added; corrected bytes remain unqualified.
- Install a standard AppKit application menu before the programmatic event loop,
  with Quit Pantheon and Command-Q targeting NSApplication.terminate.
- Keep window-close/menu-bar companion behavior, branding, layout and typed
  engine operations unchanged. No new preferences, settings or activation path.
- Add factory menu/shortcut/target tests, fake-target action invocation and launch
  wiring regression coverage. No real app termination or event posting in tests.
- Admit the exact regular `StackLab/model-router-v1-recipe-v1.json` leaf already copied by the app builders. Update both complete inert package fixtures to the 26-entry unsigned namespace and retain exact path/type/byte bindings, required-leaf refusal, and rejection of unexpected recipes. Symlink and wrong-type controls remain enforced; no wildcard payload admission or model-router policy change.
- Correct model-router v1 completion: `sirsi ask` now uses the router's
  once-only execution path, which may fall down once to a qualified local
  provider for structured remote rate or recognized quota exhaustion. Add
  privacy-dominant per-request lane overrides, an explicit `--route` opt-in
  that leaves Doctor alarms visible, and a bounded private redacted decision
  journal. Unknown streaming/determinism/JSON capabilities and the separate
  provider-readiness correction remain unqualified. Refs: ADR-054,
  `docs/stacklab/MODEL_ROUTER_V1_RECIPE.md`,
  `docs/stacklab/pantheon-pt/canon/TRACEABILITY_MATRIX.md`.
- Migrate the existing native file picker to explicit ADR070 v2: editable
  independent expected head, copyable typed format/example, read-only Go
  validation preview, then frozen file/head/raw-digest confirmation. The Go
  importer rechecks the same bounded bytes before writing; drift requires review.
- Use existing bounded capture, check completion/exit plus exact returned
  contract/version/head and unqualified state (never provider-name trust), and
  consume confirmation once before await. No decode/error/readback refresh can
  replay an import. Keep selected inputs editable after failure and retain the
  original operation receipt if the separate Casebook refresh fails.
- CLI and native are the only existing public source-import writers. MCP,
  terminal and dashboard remain read-only projections; deterministic host and
  release/credential preflight writers and historical v1 reads remain unchanged.
  No v2 downgrade, native exemption, model/service operation or gate activation.
  Authenticated calibration lifecycle remains OPEN. Fixtures are source-only,
  unrun; no installed or visual qualification claim.
- Ma’at’s Claude admission classifier accepts terminal shell whitespace on
  foreground supervised runs, registration and source reads. Previously a
  trailing newline was classified as a forbidden extra command. Interior
  commands, background runs, expansions and direct unadmitted workloads remain
  checked; parser regression tests never execute their command text.
- Source/control reads may suppress stderr to the fixed `/dev/null` device.
  Arbitrary file targets, append/descriptor duplication, supervised-run
  redirection and direct workloads remain refused. This separately addresses
  the exact CompassPoint source-read payload, which contained no newline.
- Null-stderr recognition retains shell quote/escape and IO-number adjacency,
  so executable `2 > ...` cannot be mistaken for descriptor `2> ...`.
- Unquoted carriage returns are refused rather than stripped into a different
  executable word; quoted carriage-return data is preserved.
- Separate explicit `maat.adr070.source-screen`, schema v2, from historical v1
  and the free deterministic host/preflight policy. Source imports require a
  separate expected head and exact required JSON keys; duplicate, unknown,
  case-alias, null, malformed and oversized packets reject before journal writes.
- Retain per-finding evidence identity and declared evidence recipe/calibration
  identities. Floor evidence is explicitly declared-only, not executed proof.
  Failed/incomplete floor blocks first; valid unqualified imports escalate.
  Imported qualification claims reject, never silently downgrade or authorize.
- Exact R0–R8 policy fixtures use package-private inert qualified context only.
  No production qualification writer, gate activation, model/service call or
  binding approval is added. Authenticated calibration, independent outcome,
  drift and requalification lifecycle remain OPEN; this is not full ADR070 or
  Ma'at completion. Legacy v1 journal reads and host repair/preflight semantics
  remain unchanged. Source fixtures are authored, not executed at checkpoint.
- Accept the historical `sirsi-horus-ssa-20261012` signing identifier only for
  the selected shipped legacy wrapper at its exact OS account-owner home path.
  Retain strict Apple generic anchor and Team `9D382WV988` verification.
- Keep canonical identifier `sirsi` valid for existing legacy payloads, and
  mandatory for every protected replacement. No dynamic identifier trust,
  artifact/hash/plist/instance-pin change, service action or new authority.
- Add per-instance signature-argument, conversion/readback and refusal fixtures
  for wrong path/identifier/team, unsigned/tampered artifacts and plan drift.
  These are source fixtures; main owns actual admitted qualification.
- Horus lane-delivery alerts no longer invent `wake:none`, an empty thread
  registry or a unique repair from an unavailable delivery path. Single-lane
  and fleet notices distinguish the observed lack of a fresh usable thread
  from unknown configuration, stale registrations and actual queue delivery.
  Backlog totals, classification, active-thread suppression and stable dedup
  titles are unchanged. Regression fixtures cover both sides of the rollup
  threshold and demonstrate the old producer's semantic failure.
- Ordinary native Gemma workers now consume the existing lead-owned answering
  grant before provider preflight and stamp/read back that grant on their own
  registered thread before claiming or generating, matching the tool-turn path.
  Missing or unreadable declarations fail closed; no registry, service policy,
  role authority or model lifecycle is changed.
- An answering-authority refusal after generation now fenced-blocks the same
  original with its exact draft for lead review instead of requeueing inference
  until dead-letter. Already-published nonbinding replies still require explicit
  correlation/reconciliation. Block failure reports both errors and the draft.
  Existing dead letters are not reopened or falsely declared completed.
- Added isolated startup-grant/stamping and authority-refusal regression fixtures.
  Source fixtures are not installed worker, model, hosted/all-lane or delivery
  qualification; normal verification and release integration remain separate.
- Stop the optional legacy model resolver at detected WEDGE, preserving its accepted exit-0 conf/sentinel behavior without reaching downloader or empirical probe. HF prefetch now defaults off and requires literal `GEMMA_RESOLVER_PREFETCH=1` for an already-cached non-WEDGE serving model; it never restores missing selected weights implicitly. Keep the still-installable compatibility path distinct from the native worker/SNE, and wire a whole-program inert lifecycle regression into the normal resolver CI guards. Non-WEDGE empirical probe/swap behavior is unchanged; this is not a blanket no-side-effects/offline claim or a broader Gemma completion receipt.
- Attribute the retained `sirsi-gemma-worker.sh` census matcher to the declared
  `gemma-pantheon` worker lane, matching the published native compatibility entrypoint.
- Pin that identity in the existing already-tracked-worker census fixture.
  GPU-server/triage matchers, process matching, registration mechanics, native
  worker ownership and draft-only policy are unchanged. No shell worker logic
  is restored; no claim is made that the exec wrapper remains resident.
- Extracted from the preserved primary checkout. Focused normal/race census
  fixtures pass; restoring the released matcher fails the new identity assertion.
  Installed qualification and release integration remain separate; no service,
  registry or primary-checkout changes were performed.
- E2E TestMain now returns through its owned temporary-binary lifecycle before
  calling `os.Exit`, including build failures and nonzero test exits. Build/test
  nonzero exit status is preserved. Cleanup failure changes an otherwise
  successful exit to1 and reports the exact root and captured identity as debt,
  not a zero-residue claim.
- Recursive cleanup is confined to the retained root; a changed or unknown named
  root is refused, not passed to path-based `RemoveAll`. Empty-leaf removal is
  cooperative, not an atomic hostile-writer identity guarantee. No shared cache,
  pre-existing residue, production state or unrelated TestMain is cleaned.
- Added child-process fixtures for success, test failure, build failure and root
  substitution after both successful and failed test runs. Fixtures use fake
  build/run callbacks through the same lifecycle
  helper; normal outer TestMain still builds the real CLI. No runtime product or
  current-release behavior changes.
- Add the previously named but absent `TestRouterCloseCompletionAuthorization` and `TestHandleCloseCompletionAuthorization` matrices. Each invokes the shipped command/handler and shared completion-proof gate against an owned fixture store: completed evidence commits the actual close/completion; draft, blocked, ready-for-review, missing/malformed status, blockers, failed verification, and malformed JSON refuse without changing the item or consuming the MCP claim.
- Exercise the actual portfolio validator retained byte-for-byte at SHA256 `a4953e61937d8bf03dfe9fbbcd6a0decb3fa44ad56fd4550fab0b0444cbf8ab9`, using a complete owned contract/canon/proof scaffold adapted from its existing final-status qualification fixture. Test-only instrumentation forwards unchanged argv/stdin and preserves the validator's exit status; pathname replacement checks captured-proof behavior without replacing policy. This is a pinned helper/entry regression, not CompassPoint's full workflow, live authority, or installed qualification. No production gate policy changes.
- Add explicit `thread heartbeat --ensure` for the existing registered Codex
  attended automation boundary: strict current durable-host generation proof,
  normal own-scope successor registration/readback and persisted binding before
  fenced heartbeat. Ordinary heartbeat and terminal/foreign identity fences are
  unchanged. Inert source fixtures cover A-to-B ordering and conservative refusal.
  No automation installation, service action, model invocation or installed
  restart-survival qualification is claimed.
- Guard the attended generation successor in the normal registration retry path:
  occupied conflicting/retired IDs refuse without overwrite; exact live duplicate
  transitions reuse readback, and absent targets use insert-only CAS. The existing
  RPC carries a versioned restrictive argument that old decoders refuse, with no
  permission or ordinary-upsert fallback.
- Apply explicit own-session ensure at both heartbeat and wait entry. Add private
  real-Cobra/scoped-handler fixtures for generation transition, registration and
  wait ordering, conflicts/races, real markers, terminal preservation and explicit
  environment binding consumption. No tests/builds, deployment, installed
  automation adoption or original restart-survival completion claimed.
- Align both ADR-055 index entries with the already-accepted licensing ADR. Remove the obsolete pre-merge blessing status without changing licensing policy, ratifying separate claims, or granting runtime or publication authority.
- Mark the resolved PR #465 licensing wait as historical in the companion
  ledger contract, matching accepted ADR-055 and the corrected ADR index.
  Preserve the disclosure boundary and evidence requirements for outward claims.
- NEXT corrective source, not a v0.24.124 release gate: preserve bulk thread-store
  API compatibility while adding an AST guard against fence-silent bulk upserts
  in router production code, with deliberate regression fixtures for both APIs.
- Centralize lifecycle timestamp encoding as UTC with nine fractional digits;
  add pure fixtures for offsets, exact width, subsecond ordering and record use.
  Lifecycle CAS behavior, pruning, storage schema and service controls are unchanged.
- Honor explicitly zero Thoth compact retention limits without applying the
  implicit keep-20 journal policy. Omitted limits retain the legacy default;
  positive count limits retain their existing behavior. Explicit zero/zero uses the
  Go implementation because npm delegation cannot preserve that intent.
- Compare journal retention by local civil calendar dates, retaining the exact
  N-day cutoff. Fresh entries no longer expire across UTC midnight or a DST
  boundary because their date-only headers were interpreted as UTC instants.
- Route positive-age compact policies through the corrected Go writer even
  when an external npm writer is installed. Implicit-default and count-only
  delegation remain unchanged. These policies govern journal retention, not
  the separate memory.yaml session-decisions cap.
- Cover truncated OpenAI-compatible SSE through the real HTTP provider and engine
  connector: a model-bound partial delta followed by EOF must end in one error,
  never a completion or receipt. Expand non-finite streaming-request regressions
  to Temperature and TopP with NaN and both infinities, requiring request-validation
  refusal before HTTP transport. Fixtures use only owned loopback HTTP servers;
  production guards and model/service lifecycle remain unchanged.
- Require the existing canonical actor/thread and exact-service grant check before
  inspecting the root router relay's read-only lifecycle plan. Keep apply
  authorization in the existing service controller and preserve read-only behavior
  for the five ordinary maintenance profiles. Command-local backend/authorizer
  injection covers allowed, denied and canceled preflight without live launchd.
- Passive managed LaunchAgent recovery preserves this host's OS-disabled labels,
  whether loaded or unloaded. Missing, failed, or malformed disabled-state probes
  refuse recovery before enable/bootstrap. Enabled unloaded jobs retain recovery.
- Broker liveness observes the same disabled intent before model probing or
  start/restart. Existing quarantine precedence and confirmed, target-scoped
  disabled-service repair remain intact. No historical machine-specific preference
  or owner-reversal gate is imposed.
- Added injected fixtures for passive refusal, enabled recovery, confirmed repair,
  malformed native output, and broker refusal without model/service calls.
- Corrected the disabled-label diagnostic: intentional stops remain unchanged;
  deliberate restoration uses the existing confirmed Ma'at repair and verification.
- Native release builds use the installed Ma'at supervisor and CLI as one
  compatible bootstrap pair, not a future workspace supervisor that may require
  flags the installed CLI cannot parse. Missing supervision fails before either
  payload runs; registration, admission, renewal, bounded CPU/job settings and
  cleanup remain mandatory. A legacy installed pair can conservatively obtain a
  full build grant until upgraded; this is not proof of finite CPU/model sharing.
  No security, signing, foreign-job or ordinary lane admission rule is bypassed.
- Selectively port reviewed 3fc33193 declarations to the twelve existing named
  restricted consumers: add only the exact Ma’at decision journal, mutation lock
  and supervisor evidence directory beside the existing token-free relay. Keep
  current identity, grants and network restrictions unchanged. Cover every named
  declaration and exact home rebasing; broad/sibling filesystem declarations fail
  the source regression. No live worker mask or policy is changed. Parents remain
  custodian-provisioned; actual future-worker append/evidence and sibling-denial
  qualification remains separate from source tests.
- Extend existing selected-service maintenance only to the native GUI-owner
  `ai.sirsi.horus.agent-router`, with exact SSA/SHA grants and unchanged canonical
  actor/thread/runtime authorization. Default planning remains non-mutating.
- Recognize the installed legacy login-shell form as data and bind its payload,
  launcher, instance and original plist to the signed protected CLI and converted
  configuration. Existing `--apply --drained` plus complete identity pins authorize
  this exact transition, not a general plist editor or another permission gate.
- Preserve unrelated configuration and private create-only original backups;
  retain staging descriptors, identity-checked atomic publication, bounded
  independent recovery and explicit substitution/retirement/cleanup debt.
- Refuse unknown intent, quarantine, disabled jobs, children and instance drift.
  The five original profiles and root-relay behavior retain their existing paths.
- Add per-instance native-double and tempfile source fixtures. Main owns test
  execution/review; no fixture or source claim qualifies installed registration,
  application drain, supervisor success or the actual SSA/SHA rollout.
- Describe Fleet as worker lanes, task progress, and supervision states rather
  than connected-machine inventory. The shared workspace detail corrects the
  heading and sidebar accessibility copy without changing navigation, styling,
  the canonical producer, or state mappings. Add enum-API metadata and search
  regression coverage; no additional installed-state qualification is claimed.
- Retain the original native-fleet-state-210624 uppercase decoded/rendering
  regression against the real Fleet row. Extract the existing row unchanged
  so fixture rendering does not mount Fleet's live CLI task. Cover all six
  canonical states plus unknown as synthetic data only; no appearance,
  producer, state-mapping, or installed all-state qualification change.
- Carry the native launcher's finite CPU request into `sirsi maat reserve --cores`
  and the canonical reservation. A first-arriving bounded CPU-only build no
  longer owns the model slot merely because its complete request was granted.
- Preserve quiet/loaded, unknown-share, unbounded, oversized and memory-request
  conflict behavior. No existing reservation, foreign process or workload is
  stopped or rewritten. CPU environment masks remain cooperative, not kernel
  CPU-affinity enforcement.
- When a finite CPU ask and a model coexist, current-holder/conflict projections
  select the actual full model/measurement owner instead of hiding it behind
  the first CPU row. Both original reservations remain intact.
- Normal bind now scopes router clearance to the selected repository and current
  full head SHA. Later positive titles or incidental SHA mentions cannot clear
  rejection. Every exact rejection requires explicit resolution from the same
  canonical sender in one documented closed declaration in the existing body.
- Preserve all unresolved reviewers' findings, conservative historical holds,
  known unrelated-repository exclusion, the existing explicit owner override,
  request-changes behavior and the posted review's exact commit pin.
- Add pure guard fixtures and a normal-binder fixture using only isolated fake
  gh/sirsi/openssl commands and plaintext fake ID/key files. The optional selected
  script path exists only in the test runner for independent guard-omission proof.
  No database/router protocol, credential authority or production bypass added.

## 2026-10-24 — Model-router v1 R1/R2/R4 source correction

- Route the actual `sirsi ask` completion through the single model-router
  execution path, with one qualified-local fall-down for structured remote
  rate/recognized quota exhaustion, privacy-dominant lane overrides, and a
  private bounded decision record. `--route` explicitly opts into routing
  while Doctor findings remain visible; default heuristic-only alarm behavior
  is unchanged. See `MODEL_ROUTER_V1_RECIPE.md` and the traceability matrix.
- Source candidate only: Main qualification and a real harmless local routed
  answer remain pending. R5 readiness and unknown R3 capability dimensions are
  not asserted.
## 2026-10-24 — Native Mercury retained-sample freshness correction

- Keep the native Mercury view on the canonical dashboard reader and age each
  retained node and link observation from successful read completion using a
  monotonic clock. Expired or future samples and failed reads render unknown;
  no second sampler or network polling path is introduced.
- Dashboard v1 does not expose power telemetry. The native view labels it
  unknown, and the broader native telemetry obligation remains open. This
  source correction does not claim historical web/native scene parity.

## 2026-10-24 — v0.24.130 corrective release candidate

- Start from actual released129 source0fd84630/tree6cce, not an unpublished fork.
- Compose qualified selected-outbox guide2de, native Ra read-budget fixcc5 and
  legacy resolver lifecycle fixe184. Preserve all three component/traceability
  records and exact existing Go authority, repair confirmation and deadline.
- Keep unavailable data distinct from an empty fabric, age evidence without
  polling and never treat a sentinel as restored model weights.
- Individual-source tests and semantic negatives are retained; combined source
  qualification and normal signed publication/installed proof remain separate.
  Unrun Ma'at216a and later MCP work are not included or new release gates.

## 2026-10-24 — v0.24.129 corrective release candidate

- Build from the actual released v0.24.128 baseline, retaining all prior fixes.
- Integrate the reviewed bounded Trash workflow, relay observation and repair
  identity, observational Horus explanations, completion entry-point fixtures,
  ADR-055 status correction and final command-identity-preserving classifier.
- Combined qualification covers 305 Go normal/race/no-cgo test/subtest pass events and
  57 Swift tests; final classifier and stop-ownership suites pass 14 and 10 tests.
- Preserve the Python-free Mac package boundary: the optional Claude hook is a
  separately installed source component. Signed publication, installed services,
  per-host hook adoption and autonomous queue work require their own receipts.

## 2026-10-23 — v0.24.126 corrective candidate

- Scope the historical Horus signing alias to the selected owner deployment;
  keep the protected replacement's canonical signature requirement and lifecycle
  pins. Actual loaded conversion still requires the supported drain/readback.
- Recognize both exact observed root-relay spool spellings on M5 and Studio,
  without normalizing or accepting arbitrary paths; physical retained no-follow
  inspection and every other selected-service boundary remain mandatory.
- Preserve explicit zero Thoth retention and local civil-date cutoff semantics.
- Correct Fleet copy and retain decoded actual-row state regressions.
- Require exact current-head, same-sender, per-rejection bind clearance.
- Build from the actually published signed/notarized v0.24.125 source08e71201.
  Normal protected candidate/publication and installation remain distinct proofs;
  no parent tag rewrite or new feature is authorized by this corrective batch.

## 2026-10-23 — v0.24.124 release candidate

- Include canonical main PR1194 service-mail custody and preserve its `mail_to` field in the cloud registry projection; no service is staffed as an LLM lane or declared healthy by mailbox routing.
- Refuse FIFO proof input before a blocking open; preserve bounded single-document completion authority and resolved caller-alias compatibility, with isolated red/green watchdog evidence.
- Retain PR1060 host-pressure correction: bounded aggregate commands, shared known/unknown cache, cross-process and crash-safe lock controls; native sample and loaded-consumer evidence retain their actual host scope.
- Finite canonical delegation and opt-in native Ma’at mutual-TLS booking, signed expiring caches, durable nonce receipts and partition/restart catch-up. The canonical task/message router is not duplicated; installed identity/delegation onboarding remains a separate explicit setup operation.
- Bounded own-thread reads with legacy RPC compatibility and preserved foreign-lane refusal.
- Finite historical-notice operator confinement: memory-only derived sessions, child-only gcloud logging controls and terminal serving-generation check; no general foreign mailbox access or supervisor-repair claim.
- Make the transient registry-read observer fixture deterministic by isolating it on legacy storage and holding its owned root read-only while malformed bytes are written through the existing registry file descriptor. The fixture distinguishes a permission-denied heartbeat write from an observed JSON parse failure and restores the original snapshot before releasing the barrier.
- Run the terminal wake observer lifecycle assertion against an initialized, retained canonical SQLite store, with retryable terminal transitions and coverage that stale heartbeat snapshots cannot overwrite committed terminal state. Legacy JSON snapshots remain unfenced compatibility data; this fixture does not change their behavior.
- Add a read-only-by-default, explicitly granted single-service launchd lifecycle
  path with pinned runtime/artifact identity and honest restart readback. This
  avoids using a bulk fabric refresh for one Apollo or Mercury helper. No
  automatic elevation, global stop-guard relaxation or live service mutation.
- Preserve the confirmed native Pantheon lead during long builds; revoke the
  competing background answering grant without restricting other lanes' CPU.
- Bind cloud images to clean committed source and a minimal recipient policy;
  exclude Mac commands, credentials and live stores.
- Classify documentation-only changes from the complete path set. Mixed
  code/documentation PRs must not emit successful heavy-test exemption checks.
- Rebuild the native Pantheon workspace around the Apollo/Mercury visual language, searchable persistent navigation, and clear Apollo, Mercury, Ra and Ma’at responsibilities.
- Open Apollo telemetry through the actual native navigation owner, preserve a plan when revising it, and carry its selected resource context into Ma’at guidance.
- Refresh workspace observations without initiating a storage scan; distinguish unread health from observed state.
- Preserve independent workspace drill-downs without hidden polling, including focused restore, stack isolation and header-update regression tests reconciled from the headless worker's preserved navigation checkpoint.
- Resolve only owned temporary fixture roots to their canonical macOS path before journal/cache tests; keep production no-follow traversal unchanged. Add a real canonical-parent success control and a symlink-ancestor refusal fixture so rejection cannot pass merely because `/var` aliases `/private/var` on a developer Mac.
- Require Mercury's published live state, a successful read and a fresh sample before presenting current machine utilization. Age retained samples while the workspace is visible, including during an in-flight read, without adding network polling or changing the existing interface design.
- Label the published gpu_busy percentage as GPU utilization, not memory residency.
- Share a bounded, read-only Mercury dashboard consumer across the canonical CLI and MCP, retaining producer units, freshness and unknown values without adding a sampler, run control or service action. Preserve the current native hardware workspace and record the separate historical web/native parity obligations.
- Assess Mercury sample eligibility once after response completion across totals, headlines and scene directions; retain partial observations without presenting expired or future samples as live traffic. Bound response headers separately from the body.
- Compose the finite pod-authority candidate with the deployed canonical router protections, preserving both delegation and mailbox authority namespace guards. Retain the canonical terminal-observer CAS fixture and the independently corrected transient-read fixture together. Integrated release qualification and signed distribution remain required.
- Prepare a private dormant launch recipe for the package-installed Ma'at native authority, with explicit startup/login consent boundaries; preparing configuration does not activate or qualify the service.
- Add a separate finite, operator-approved service-notice reconciliation API and read-only-by-default CLI. Coordination acknowledgments preserve source/history and publish their audit atomically, without granting general foreign-inbox access or pretending a service is an agent consumer.
- Include the exact native-workspace contract copied by both app builders in the strict package policy. Compare the real source Stack Lab directory to that policy in tests, in both directions, so hand-built fixtures cannot conceal a missing or unexpected shipped recipe. The combined unsigned fixture includes24 entries, with no wildcard allowance.
- Own successful native admission before rank binding and fenced-plan encoding, and release it exactly once on early errors, cancellation, normal completion, or panic with an independent fifteen-second cleanup context. Preserve both work and release errors.
- Reject shorter or longer admission rank lists before indexing. Read measured metrics before release and publish them only after work and release both succeed; panic cleanup never publishes success metrics.
- Add callback-only lifecycle regressions without tokenizer, model, backend, network, or GPU execution. Runtime proof is performed separately by the release owner.
- Router send quota no longer blocks broadcasts:
- Use one native endpoint selector for admitted fabric transport and rendered SSH profiles: surveyed Thunderbolt links and the existing authenticated Studio forwarding path precede classified LAN and Tailscale. Preserve runtime proxy constraints, logical host pins, destination admission and existing sessions; fail rather than reroute after identity/authentication rejection.

## 2026-10-22 — v0.24.123 release candidate

The completion snapshot fix is new in this candidate. Other entries below consolidate previously shipped source fragments; they are not new functionality first delivered by v0.24.123.

The release also repairs inherited pre-push Git environment contamination in test children. The parent hook, source worktrees and normal verification flags are preserved; a retained RED/GREEN fixture and protected CI guard cover the repair.

- Completion proof authority:
- Router completion-proof validation now binds evidence to the Git worktree that
  invoked the close while dispatch continues to use the canonical router
  service. CLI and router-MCP closes reject unknown callers and proofs from a
  different repository instead of borrowing Pantheon's contract.
- Ma'at ordinary launches default to shared CPU build work rather than an
  implicit loaded/full-share model request. A cooperative one-CPU default can
  be increased explicitly. Quiet measurements and loaded/model work remain
  explicit, and existing jobs are not stopped or relabeled.
- Raw CLI reservations default to build rather than quiet. This retains the
  existing client/server schema and lease/ownership checks.
- Explicit native quiet qualification:
- Stop-ownership guard ordering:
- Direct task delivery:
- Restore the accepted sleep transaction and add a canonical online sleep-only issuer, management-only finite delegation, exact authenticated participant binding, atomic Ma'at/consumption host fence, native identity/inventory/full-wake adapters, independent M1 observer and desktop-sleep CLI. Retain nonce and unknown-outcome history; explicit reconciliation never retries power. Remove the parallel signature prototype. Preserve schema25, current guards and existing transport/keys. Source qualification is separate from normal release and fresh native power qualification. Refs: ADR-119, docs/user-guides/governed-sleep.md.
- Completed task wake residue:
- fix(router):

## 2026-10-20 — v0.24.122 release candidate

- Release safety:
- Relay polling:
- Provider quota retry hold:
- Bind the native release build to its actual runner executor and a maintained
  canonical Ma'at CPU grant before compiling, signing or notarizing. Keep Go
  and Swift at the declared single-core cooperative build mask; retain failed
  admission and cleanup receipts instead of treating the legacy rail-lock hook
  as a grant. Default Swift job arguments are valid under macOS Bash 3.2 with
  nounset, without an empty-array expansion. Refs: scripts/maat/README.md.
- Current lane failures:
- `sirsi gemma tool-turn` provides an opt-in Apollo 2.1 structured-tool path
  for an explicitly reviewed local profile. It accepts only named registered
  observe tools, rejects model arguments and additional calls, and returns one
  factual tool-result follow-up answer without creating a shell executor or a
  resident model worker.
- The existing fenced Gemma router worker can opt into the same read-only,
  single-tool contract with `--apollo-tool-turn`, preserving its claim and
  reply-readback completion fence.

## 2026-10-20 — bounded Apollo worker source successor

- Fence a bounded Apollo tool turn to one explicit router item rather than the
  oldest eligible inbox item.
- Require and read back the lead-owned Gemma worker answer-role declaration
  before the bounded worker may claim or complete that item; ordinary Gemma
  drafting remains unprivileged.
- Extend the router service/client contract additively with `ClaimItem` while
  preserving existing `ClaimNext` clients. Remote rollout and a fresh actual
  qualification remain required.
- Stack Lab recipe: `stacklab.recipe.apollo-2.1-bounded-tool-turn`; diagram:
  `APOLLO-P01-exact-item-worker`.
## 2026-10-19 — v0.24.121 release candidate

- Allow independent IDE sessions to use `sirsi mcp` concurrently instead of
  exiting behind a machine-wide singleton. Keep legacy `sirsi seshat mcp` on
  the same clean JSON-RPC transport and preserve shared-store locking.

- Verify M5 fabric transport against its existing `sirsi-m5` Ed25519 pin in
  the dedicated known-hosts file. Preserve strict checking, public-key-only
  authentication and runtime-required proxy routing; do not create or copy keys.

- Preserve explicitly configured, token-scoped logical cloud executor identity
  in client sessions and thread registration, heartbeat, save, and close paths.
  Reject malformed names or conflicting physical-machine identity before HTTP;
  keep server host-token and lane-scope authorization unchanged. Native Mac
  host behavior and resident-worker protection are unchanged.

- Include the CLI's routerstore import so the registration command compiles;
  package-only router tests do not qualify the command entry point.

- Preserve complete release notes and continuation lines in the Stack Lab
  changelog instead of cutting ordinary entries off at ninety characters.

## 2026-10-19 — v0.24.120 release candidate

- Distinguish bounded executor cleanup from published resident-observer protection.
- Persist canonical installed executable paths in wake service definitions.

## 2026-10-19 — v0.24.119 release candidate

- Retain the already shipped v0.24.118 semantic thread-save correction.
- Preserve Ra's own-lane session identity fix through true merge history.
- Isolate private civic coordination from independent project records and policy.
- Adopt atomic executable replacements at consumer boundaries without signals.
- Use the kernel's loaded image identity, including pre-capture upgrade races.
- Verify the persisted native observer generation before new dispatch.
- Hold new dispatch on heartbeat failures and preserve terminal evidence.
- Restore Ra's existing answer grant; preserve Mercury's busy attended owner.
- Retain scoped SOCKS by default and permit only explicit authorized direct
  transport, with strict pins, literal argv and both native supervisors.
- These are candidate notes, not signed-publication or fleet-queue receipts.

## 2026-10-19 — v0.24.118 registration-control release candidate

- Integrate Ra #1156's single save-time decoded-schema comparison and retain
  SSA's real-store regression; remove the duplicate load-time normalization.
  Compare thread payloads in one decoded schema, leaving
  unrelated service rows byte-exact while retaining all actual lifecycle and
  host fences. Qualification and signed deployment are required separately.

## 2026-10-19 — v0.24.117 release candidate

- A wake loop whose lane has not granted its worker the answering role (registry `worker_a
- The router service is built locally and deployed staged: `scripts/router-service/stage.s
- The router service gzips responses for clients that ask (every Go client and relay does,
- Isolate native executor CLI control calls from inherited IDE provider-session identifier
- Work board no longer pulls ~19 MB of item bodies every refresh: new `ListSinceHeaders` r
- A process running a binary older than `answer_role` can no longer erase a worker's answe
- Ma’at loaded/model admission no longer depends on whether a bounded,
  CPU-only build fl
- Ma’at distinguishes configured execution budgets from renewable admission
  deadlines an
- Destination-local Ma’at executors always obtain their own canonical grant,
  including l
- Resolve a partially committed registration after an unrelated terminal-prune
  lifecycle

## 2026-10-18 — v0.24.116 release candidate

- Headless workers are subordinate to lead lanes (owner 2026-10-18): the router service re
- Native Codex workers use a proxy-enforced exact enrolled-host allowlist instead of silen
- Native Ma’at supervision retains only already-proven lease time during transient heartbe
- Bound healthy reservation inventory reads/renewal to a30-second cadence shortened by rem

## 2026-10-18 — v0.24.115 release candidate

- Canonical transactional reservation decisions, heartbeat/release and expiry.
- Correct actual runtime classification and destination-local executor binding.
- Preserve the owner's attended-only Mercury consumer policy.
- Refuse known-incompatible and non-newer unified app/CLI update installers.
- Read supervisor ancestry/start identity through Darwin kernel APIs, not ps.

## 2026-10-18 — v0.24.114 release candidate

- Release DMG/PKG build no longer dies when no private Apollo profile is supplied: macOS b

## 2026-10-18 — v0.24.113 release candidate

- Release workflow builds with the read-only private SDK credentials, like CI (#1109 wrapp

## 2026-10-18 — v0.24.112 release candidate

- Trusted Pantheon CI builds can read the two private Apollo SDK repositories using separa
- Document two Linux-only static-analysis false positives at Darwin capability fallback br
- `sirsi maat conflict-check` classifies a checker failure (e.g. the router store/ledger u
- Apollo native packaging (rebased from #1106/#1108/#1109): package the direct Apollo nati
- Preserve alias-owned task history in the fleet dashboard instead of silently dropping re
- Normalize both public executable modes before application signing so a restrictive build
- Use the measured Studio-to-M5 placement for the local fleet's31B Q8 profile.
  Moving th
- Apollo's installed native profile can admit one local whole-model rank through
  the sam
- Local profiles refuse coordinator, transport-edge, or raw-rank residue. The
  native SDK

## 2026-10-16 — v0.24.111 release candidate

- The spool relay keeps up to 8 forwards per lane in flight continuously: a request arrivi

## 2026-10-16 — v0.24.110 release candidate

- The spool relay forwards up to 8 requests at once within one lane instead of strictly on

## 2026-10-16 — v0.24.109 release candidate

- macOS Tailscale backend check runs the CLI with the vendor's explicit selector (`/usr/bi

## 2026-10-16 — v0.24.108 release candidate

- The wake-loop progress gate counts a consumer run as progress when it made a durable rou

## 2026-10-16 — v0.24.105 release candidate

- Add the machine registry (owner 2026-10-16: machines register with Ra like threads): `.a
- Correct the claude-apollo-m5-rail consumer launch prompt: owner 2026-10-16 made M5Rail t
- Fix a golangci-lint errcheck on `thread watch`'s watcher-error receive that failed Lint 
- Declared attended lanes (owner 2026-10-16: secondary consumers never override the attend
- Lane state reports a declared attended lane's `attended` hold ahead of transient measure
- Repair the two CompassPoint attended-only lane declarations with guarded own-queue consu
## 2026-10-15 — v0.24.105 release candidate

- Preserve alias-owned task history and complete CLI/web fleet totals. Keep
  equal task IDs under different original owners distinct. Normalize public
  executable permissions before signing, independently of the builder umask.

## 2026-10-15 — v0.24.104 release candidate

- feat(router):
- feat(router):
- Router sessions with a registered thread now read, publish and invalidate only their own
- fix(guard):
- fix(signing):
- fix(router):
- fix(router):
- fix(router):
- Native Codex sessions now key router registration and Ma'at Phase-1 attribution on `CODE
- fix(router):
- Add `sirsi gemma shared` for registered M1/M5 lanes through the existing canonical route
- Router relay clients now distinguish denied or failed slot creation from actual request 
- Stale slot recovery now returns removal errors and performs only one bounded reacquisiti
- Added a restricted Codex relay qualification recipe using a named permission profile, th
- feat(router):
- fix(router):
- fix(router):
- fix(router):
- fix(router):
- fix(router):
- fix(router):
- fix(router):

## 2026-10-12 — v0.24.101 release candidate

- docs(router):
- docs(router):
- fix(router):

## 2026-10-11 — v0.24.100 release candidate

- Router queue drill-down:
- Replace the disconnected duplicate `/router` mock with the shared embedded Horus operato
- Preserve queued recipients missing from the lane projection instead of hiding their work
- Retired direct-file board entry points open the running loopback console instead of disp
- Return listener bind failure before claiming the dashboard is running. Source/build prov
- Add shared-page, explicit CORS and occupied-port startup regressions. Documentation dist
- fix(maat):
- fix(router):
- docs(portfolio-standard):

## 2026-10-09 — v0.24.99 release candidate

- Revises ADR-072 (universal thread naming) C2/C4/C5 and the phase order per
  SSA round-1
- Closes codex-pantheon's successor CHANGES_REQUESTED on PR #1042 (head
  `0b1184c8`): `Bo

## 2026-10-09 — v0.24.98 release candidate

- `relieve --memory` no longer raises an admin-password dialog when run unattended.

## 2026-10-08 — v0.24.97 release candidate

- Wake loops share one CPU-headroom probe per host.
- Refuses implicit production use of `~/.sirsi/router.db`; service resolution is
  fail-cl
- Makes Ma'at's test ledger binding explicit so tests cannot inherit the live
  router aut
- Publishes the complete Ra router architecture, release contract, traceability
  matrix, 
- Adds a hermetic one-authority negative control and preserves the existing
  routerstore,
- Classification: platform-foundation/pilot. Fresh cloud readback, third-machine
  rehears
- `knownfail.ciRunsExactPath` now parses the CI workflow as real YAML and
  requires the g
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — the Rule-17 sprint
  plan SSA's
- `.agents/completion.contract.json` — `canon_documents[4].path` (portfolio
  manifesto) c
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — third-round
  correction per SS
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — second-round
  correction per S
- `.agents/completion.contract.json` — added (new file); sirsi-pantheon had
  no completio
- Router: refuse the retired implicit local ledger
- `claude-pantheon` shares the reserved consumer slot.

## 2026-10-06 — v0.24.96 release candidate

- `sirsi router task-lease-audit [task-id]` — a read-only audit of task-lease
  ownership 
- The router client can enforce the published TLS key pin (opt-in).
- The TLS pin now binds the per-host relay, the process that holds the host token.
- Ma'at consults the known-failure catalog on every failing gate step, in the pre-push hook, in CI and in the release train.
- Sending to a lane now validates against the pinned registry.
- `claude-m5-compasspoint` is a declared router lane.
- CI + pre-push gate refuse a non-release/* branch that edits `CHANGELOG.md`
  directly; t

## 2026-10-06 — v0.24.95 release candidate

- `sirsi router snapshot`: the router snapshot as JSON, from the same producer as the Horus dashboard.
- The release train clears the abandoned branch a stopped run leaves behind, and refuses to touch real history.
- The Ma'at pre-push gate queues behind another gate on the same Mac, and shows the linter's real error.
- The Horus dashboard is redesigned
- `TestSenderFloodRejected` no longer fails at the top of the hour.
- `/api/router` sends an empty list, not `null`, for a release section with no entries.

## 2026-10-06 — v0.24.94 release candidate

- Reviewer lanes get a reserved consumer slot.
- The release train treats the M5 as best-effort and reports it honestly.
- The changelog now says what is unreleased, and the release script cuts all of it.
- `/api/router` carries what a richer dashboard needs, additively.

## 2026-10-05 — v0.24.93 release candidate

- Router: add the task retry-ceiling operator verb the breaker verbs already established the shape for
- Development candidate: bounded mobile desktop recovery transplanted onto current release
- feat(routerstore): migration gate — a build with uncommitted changes may not
  apply a s
- feat(menubar): decode node-status's `outbox[]` (ADR-069, PR #931) into
  `RBOutbox` and 
- test(menubar): `OutboxReachabilityTests` — the new `macapp` test target
  proving the de
- feat(board): `sirsi board-serve` — the Go router board, replacing the
  out-of-repo Pyth
- fix(menubar): reads `board-serve --once --shape fleet` — a PROJECTION of the
  board's o
- chore: retire the 9119 Horus dashboard (duplicated the menubar) and the
  token-burning 
- Add `sirsi maat submit --kind tag|release|release-edit|merge --repo OWNER/REPO --ref REF
- Router: `setThreadConsumerCapable` no longer silently drops its write — two independent gaps closed, not one
- fix(board,menubar): rename the "touched" column to "last ledger update". It
  measures t
- fix(supervision): `supervision.Escalates()` had no caller — lanes no wake could
  reach 
- fix(dashboard): `fleet.go` hardcoded `Routable: true`, making `UNROUTABLE`
  unreachable
- fix(routerstore): recognize schema v8–v14, already deployed to the live store
  by an ou
- Hooks: the SessionStart inbox counter now warns loudly on a schema drift instead of silently reporting a healthy empty inbox
- fix(router): gemma was unreachable from the router in two independent ways —
  the worke
- fix(router): the wake loop now logs a bounded tail of a failed consumer's
  output. `dis
- fix(routerstore): read-compatibility with a store newer than the binary. The
  write gua
- fix(dashboard): 8734 is now served by the SAME process and handler as 9119.
  It was a s
- fix(board): emit `ledger` alongside `board` in the router-board payload.
  index.html re
- Made CTR thread registration, heartbeat/current-item, close, suspend, and resume SQLite-
- Router: restore the ledger header items/tasks split that PR #668 silently reverted
- A lane can no longer disarm another lane's wake loop.
- Process diagrams for the router wing (SL-DIAGRAM-001).
- Router readiness audit (G1 to G12) and the docs it was missing.
- Exactly-once claim test over 1,000 contended rounds
- Per-deploy audit receipt.
- The release train assembles `changelog.d/` before it cuts a version.
- `cmd/sirsi/threadcmd.go` `thread register` derived the router filesystem root directly f
- Fixed: `SpoolOutboxHealth` used `filepath.Glob`, which silently swallows directory-read 
- `router.NodeStatus` (`sirsi router node-status --json`, GET /api/node-status) gains `out
- Forward correction to the previous append-error propagation fix: PR #929's `appendCedeDe
- Document Pantheon's Ma'at failure-memory contract and eight-domain operational preflight
- liveness-watch re-alarmed on a menubar the owner had just quarantined, and a live agent auto-relaunched it
- fix(router): stray-reap salvage is inscribed only after the save persists

## 2026-10-02 — v0.24.66 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-02 — v0.24.67 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.68 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.69 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.90 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.91 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-05 — v0.24.92 release candidate

- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.

## 2026-10-01 — v0.24.65 release candidate

- Records the PR947/PR949/PR950 lineage from the exact tested mainline: lease
  ownership across session remint and threadless invocation, and a fail-closed
  PostgreSQL CI leg.

## 2026-10-01 — v0.24.64 release candidate

- Records the PR944 router-completion release lineage from the exact tested
  mainline, including token-fenced completion, authenticated dual-backend
  coverage, truthful lane-state observability, and reversible router closure.
- The tag and published artifacts become the canonical starting point for the
  next Pantheon build; signing, notarization, cask, and installed-host receipts
  are recorded only after their corresponding release jobs complete.

## 2026-09-28 — v0.24.23 release candidate

- Records the exact tested mainline Stack Lab doctor roster-provenance change
  from PR #843 as the next commercial patch release candidate.
- Release scope remains CLI/Go and canon provenance; macOS signing,
  notarization, Homebrew cask publication, and installed-host qualification
  remain release-workflow evidence rather than source-only claims.

## 2026-09-27

- Added the complete Stack Lab nine-record canon bundle.
- Separated source/static claims from build, package, credential, signing, notarization, remote, cask, and installed-host proof.

## 2026-09-28

- Added the independently cataloged development-versus-commercial release artifact recipe. Commercial artifact names now require the Developer ID/notary/stapling route; ad-hoc package work is explicitly `-dev` only.

## Finite installed141 correction — source in preparation

- Bind actual public141 signed pair/cask, exact source tests, selected loaded
  adoption and human reading copies separately from remaining certification.
- Preserve the actual91/100 A17 FAIL and its findings; retain independently
  reviewed historical canon crosswalk and meaningful coverage work.
- Retain per-window/journey Apollo selection through reconstruction/Back with
  fresh-capacity validation; a revision cannot overwrite its origin on cancel.
- Remove auth/model commands from passive Fabric, reuse the same bound default
  board in the web app and preserve selected task/cursor and freshness semantics.
- Add RPA's exact owning repository and declared peer; the changed Ra wing
  requires normal owning merge and registry repin. No runtime grants change.

Source/causal proofs and installed/lifecycle outcomes remain distinct. This is
not a new release announcement or a retrospective rewrite of public141.

### Native readability and command menus — same corrective cohort

- Replaces the Quit-only menu with native Pantheon, File, Edit, View, Window and
  Help menus, standard editing responder actions, and existing workspace routes.
- Raises body text to 15 pt, secondary copy to 14 pt, and supporting text to a
  12 pt floor before scaling; adds persisted 100–150% text sizing independent
  of window width and preserves workspace/navigation state.
- Compilation, menu validation, state continuity and installed visual/keyboard
  verification are distinct acceptance evidence. This entry is source scope,
  not a claim that 0.24.141 already includes these changes.
