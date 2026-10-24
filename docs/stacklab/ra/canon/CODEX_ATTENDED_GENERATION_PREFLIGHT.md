# Codex attended generation preflight

This repairs the existing `codex-home-pid-churn` mechanism at the explicit
attended heartbeat boundary. It is not another watcher, dispatcher, database,
service, role, or source of work authority. Ordinary heartbeat is unchanged.

## Existing surface invocation

An already registered Codex session's existing automation can invoke:

```text
sirsi thread heartbeat --ensure --json
sirsi router wait <own-agent> --ensure --timeout 50 --json
```

Each explicit entry executes the real shared preflight before its operation.
The wait entry also heartbeats before opening the ordinary own-inbox facade.
On success it uses the **returned** thread binding, then invokes ensure again
after that bounded wait returns. It must
not wait or publish success after a failed ensure. No heartbeat is performed on
an ambiguous/missing runtime. This command does not schedule its own recurrence,
install automation, launch a model, or start a consumer. The actual automation's
invocation/adoption and installed recurrence still require separate evidence.

An explicit `--thread` must equal the current real session's recorded binding;
it is not permission to repair an arbitrary row. Omitting it follows the normal
persisted session marker, not a scan for another lane's freshest thread. A stale
explicit `SIRSI_THREAD_ID` is reported, not silently rewritten; an existing
controller that deliberately pins it must consume the returned successor and
update its own invocation normally. A wait whose inherited explicit ID just
became stale refuses before Inbox/Wait and names the persisted binding to consume.
No provider session identity is fabricated.

## Scope and lifecycle

The actual provider session aliases must agree. Existing session-to-agent and
session-to-thread markers, canonical exact-row identity, Codex surface and local
machine identity must agree before any registration. These are ordinary
cooperative local-host scope checks, not signed per-process authorship proof;
the canonical service still applies its existing authorization.

The strict durable-runtime resolver walks actual caller ancestry and, for the
desktop broker, its uniquely selected code-mode-host. The permission-error
immediate-parent fallback used by ordinary registration is **not** used by ensure.
PID/start evidence must be known and live, then match again after readback.

A changed generation normal-registers a fresh explicit ID with the predecessor's
exact agent/session/repo/workstream/watch/current-item scope, rather than letting
session-keyed reuse adopt a sibling. The predecessor is not rewritten or revived;
normal reaping remains responsible for genuinely dead active predecessors.
The explicit ID is a deterministic idempotency name for the exact predecessor,
session, machine and new native generation, not an authorization capability;
overlapping ticks for that same transition select the same normal registration.
The normal registration retry path carries a deny-only `EnsureCompatible`
intent (not persisted). An occupied ID is accepted only as an exact compatible
live binding, returned without registration mutation. Any session/workstream,
scope or native identity conflict, terminal or suspended target refuses before
normal registration/compaction. An absent target uses insert-only CAS, so a
raced insert cannot be overwritten; contention reloads and rechecks compatibility.
The normal `UpsertThreadCAS` endpoint carries this restriction in a versioned
array argument. Older servers cannot decode that argument as a record and refuse
before mutation; there is no fallback to an ordinary upsert. The endpoint's
existing host, lane and bootstrap rules are unchanged. This adds no Store method,
schema, role, grant or authority exemption. Explicit legacy registry mode is not
silently overridden and cannot supply this canonical insert-only CAS guarantee.
Terminal rows stay terminal. Suspended rows require explicit resume; same-native-
generation terminal rows refuse. No capability, answer-role or old lane outcome
is borrowed onto a successor. Service policy remains authoritative for grants.

Readback must match scope and native generation. The normal session marker is
persisted before heartbeat, which carries the existing expected-PID/start fence.
Failure after registration can leave an un-heartbeated normal registration and
is reported; it is not rolled back by deleting a row or hiding cleanup debt.

## Inert qualification (authored, unrun)

`cmd/sirsi/threadensure.go` is called by the real `thread heartbeat --ensure`
branch in `threadcmd.go` and `router wait --ensure` in `routercmd.go`.
Fresh command constructors capture flags locally and inject only kernel
observation; production supplies `strictCodexAnchor`. `threadensure_test.go` injects kernel observation and
normal store effects; it starts no real process and touches no real registry.

- `TestAttendedEnsureGenerationBeforeHeartbeatAndWait`: A then B, registration
  and readback/marker before heartbeat, unchanged exact scope, no repeated renew.
- `TestAttendedEnsureTerminalGetsNormalSuccessorNeverRevival`: both terminal
  states preserved and no capability/result borrowing.
- `TestAttendedEnsureForeignAndUnrelatedRefusedBeforeMutation`: exact explicit
  thread, agent, session, machine, surface, suspension and unknown-row refusals.
- `TestAttendedEnsureAmbiguousMissingAndChangingRuntimeFailClosed`: unknown or
  ambiguous host, absent start, generation race, readback drift and marker error.

Proposed normal/race selector: `./cmd/sirsi -run '^TestAttendedEnsure'` plus existing
`TestResolveDurableAnchor`, `TestRefineDesktopAnchor`, and router generation/fence
fixtures. Production-only negative: omit the changed-generation registration in
`ensureAttendedHeartbeat` with fixtures unchanged; require the A-to-B test to fail
semantically after compilation. No execution has been performed for this source.

The fixture's final wait marker asserts the caller's success-only sequence; it
does not impersonate actual router wait execution or prove an installed scheduler.
The additional `threadensure_entry_test.go` fixtures invoke the actual Cobra
heartbeat and wait entries with private HOME/markers, a private canonical store,
the real scoped authenticated HTTP handler, normal registration/readback and
fenced heartbeat. Only native observation is injected. Actual Inbox request
ordering and session binding are checked, not inferred from a synthetic marker:

- `TestAttendedEnsureCobraHTTPGenerationAndWait`: A live, changed B and stable B
  through both entries, fresh normal registration before heartbeat/actual wait.
- `TestAttendedEnsureCobraHTTPConflictsStayByteExact`: occupied and raced
  same-agent/session/workstream conflicts plus terminal/suspended targets.
- `TestAttendedEnsureCobraHTTPCompatibleTransitionReusesBinding`: compatible
  existing/raced same-transition readback, with no duplicate registration rewrite.
- `TestAttendedEnsureCobraHTTPTerminalAndPinnedBindingConsumption`: terminal
  predecessor unchanged, empty/pinned-old env, explicit successor consumption.
- `TestAttendedEnsureCobraHTTPMarkerAndScopeRefusals`: real marker-write failure,
  foreign thread/inbox, suspended/terminal, missing/ambiguous native proof.
- `TestAttendedEnsureStrictKernelDoesNotUseParentFallback`: strict source resolver
  refuses the ordinary resolver's permission-error parent fallback.

`TestRegisterCompatibleOccupiedAndRacedTargetsRefuse` covers the normal retry
derivation; `TestThreadInsertOnlyCASPreservesOccupiedAndScopedRows` and
`TestThreadInsertOnlyVersionFailsClosedForLegacyDecoder` cover restrictive SQL,
actual remote/scoped calls and old-decoder refusal. All are authored, unrun.
No universal hostile same-UID or atomic kernel-check-to-service-write guarantee.
Concurrent surface ensures and actual automation integration need independent
review before installed qualification; this source checkpoint is not full original
restart durability proof.
