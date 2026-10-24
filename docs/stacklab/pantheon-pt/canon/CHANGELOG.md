# Changelog — Pantheon PT canon

## 2026-10-24 — v0.24.135 release candidate

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
