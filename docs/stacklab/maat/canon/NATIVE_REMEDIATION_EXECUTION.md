# Native remediation execution — corrective source contract

Scope: `menubar-remediation-20261005`, native operator review → confirmation →
one command → fresh diagnostic observation → retained bounded outcome. This is
platform-foundation correction, not a new remediation catalog, privilege,
installer, background service, or model workflow. Existing typed mappings and
Go operational authority remain unchanged. Installed UX is not yet qualified.

## Approval and invocation

When confirmation opens, freeze semantic diagnostic evidence (not its ephemeral
UUID), exact argv including `--json`, and working directory. Immediately before
launch, require the same current finding, freshness/error state, command and
repository. Cleared, changed, stale or failed observations invalidate consent.
The closed legacy repair registry follows the same rule; prose never becomes
executable authority. Each explicit next-action confirmation binds its source
receipt, exact command, argv and repository. No new command authority is added.
The producing directory stays with each report: changing selection from A to B
refuses offered actions before forming confirmation, not merely after consent.
Unknown-scope preloaded reports remain display-only; current selection never
supplies missing report provenance.

Set the invocation latch before any suspension. A ResultView identity captures
once, including non-JSON and error output. `.task`, project callbacks, Refresh,
Retry and post-action refresh must not replay the original invocation. Recheck
is diagnostic-only. Another mutation requires another explicit intent and its
own frozen confirmation. Retain the actual follow-up receipt, not an automatic
rerun of the original action.

## Capture and honest limits

Use nonblocking stdout, stderr and stdin; stdin is at most 64 KiB. Retained
stdout plus stderr share one aggregate 4 MiB limit. Read/write work is chunked
and checks a monotonic deadline. Success requires confirmed child exit AND EOF
on both streams. Overflow, cancellation, timeout and I/O failure withhold typed
decode, even if the captured prefix is valid JSON. Complete nonzero typed
reports remain available for their existing consumers to interpret.

On incomplete capture, send TERM only to the directly owned Foundation Process
and allow at most 250 ms exit observation. There is no process-group or
descendant cleanup claim, no foreign PID signal, and no unbounded wait for EOF
or exit. If exit is still unknown, retain that cleanup debt explicitly; stopping
capture does not mean the operation stopped. Kernel process launch and executor
scheduling are not hard real-time bounded. Output is a bounded in-memory receipt
and Activity projection, not a newly introduced durable forensic store.

## Verification and recovery

Consume the specific successful post-action diagnosis returned to the verifier,
not shared `engine.health` after awaiting. Failed/incomplete/malformed reads
remain unverified, preserve existing timestamp/error freshness, and never imply
resolution. A fresh clear observation reports current health only, not proof
that the preceding action caused recovery. For incomplete execution, inspect
current evidence and existing Ma'at/Stack Lab recovery routes before renewing
intent; never auto-retry a possibly committed mutation.
Each action start and newer verification read clears the prior verdict and
invalidates older read generations. A delayed old clear cannot overwrite a
new action's failed/unknown verification.

## Qualification still required

Source fixtures: `RemediationExecutionTests` (semantic/argv/repo drift, once-only
non-JSON/error, failed diagnosis, nonzero report compatibility, aggregate overflow,
blocked stdin, TERM ignoring, retained pipe writers, final launch denial).
Fixtures use only owned inert temporary scripts with finite self-expiring
children, not a model/service or live remediation payload.

Final published126-baseline source `b27419c5` passed Main's normal qualification
`8c211952d0e34144acab79308b66dae4`:82 Swift tests, two skipped and zero failures,
including the retained Fleet rendering test and15 remediation fixtures.
ThreadSanitizer qualification `21d1e55d28fd41a082979f3270e32b92` ran the15
remediation fixtures with zero failures and no race diagnostic observed.
Guard-omission control `f89daed5f1cf4a53b693dc4639d3328a` compiled, then produced20
expected semantic assertions. The negative ran before the normal rebase; helper
and fixture bytes are identical in the final source. These detailed outcomes are
Main-observed command-output evidence; the supervisor retains terminal/argv and
execution metadata, not full stdout logs.

Independent exact review
`c5eda27054f9204c34f1c748aa9b86d811b3966d1dae8cf0904920d3db8ea1f4`
accepts source/fixture scope and resolves the report-scope and verification-
generation findings. Installed review/confirm/execute/reverify/recovery UX,
normal integrated publication and generic Go contract commands remain separate.
SSA owns publishing and runtime rollout; the proof stays draft, not completed.

## System One binary-drift route

The closed `binary-drift` action is available only for the canonical typed host
diagnostic finding (including its exact legacy v1 ID/evidence form); producer
titles and `FixHint` text do not select or authorize it. Native Casebook first
previews the resolved Homebrew executable and every drifted Homebrew target by
path, method, version and SHA-256. Confirmation is bound to that immutable
preview and the original screen evidence. The action calls the same existing
Homebrew package-updater helper used by `sirsi fix`; unsupported, unmanaged,
mixed or changed targets are refused. It never uses the ad-hoc self-update
replacement/signing path. A separate factual receipt links to the original
screen, and only a successful updater plus a fresh clear diagnostic and
inventory marks that observation resolved. Failure or unavailable postcheck
leaves the original observation open. This source addition has not itself been
qualified or used to update an installed binary.

The confirmed command now retains the attempt before appending the existing
operation-wide claim, then records and reads back a typed invocation barrier
after final manifest/schema/target revalidation and immediately before the one
updater call. Store flock precedes the journal mutation lock; the claim callback
does not reenter Store. A new typed attempt with no invocation barrier can be
reconciled as a retained `not_invoked` fact; attempts with the barrier and no
terminal result remain unknown and cannot be replayed. Legacy attempts without
the protocol marker also remain unresolved.

`sirsi maat repair binary-drift --reconcile` is data-only. It can recover a
retained terminal outcome when projection intent was not yet written, or finish
the existing projection debt by writing the outcome projection, idempotently
appending it to Casebook, verifying the exact journal readback, and retaining
the readback receipt. It never invokes the updater or derives a terminal result
from current host state. History validation remains structural; the short
manifest freshness window applies only to new admission, so expired historical
attempts and outcomes remain inspectable.

Composition provenance: the complete 27-file updater feature delta was
reconciled from `ec6253d14980a6d7d4a8c3adea6679be11cd0dcf` through
`a5734b9b58f4f24895858bd53e388a1f4a4d1095` onto release-135 composition
baseline `2634cbeb054b91238a2c0691949d014cb35a4a76`. The release-134
liveness-watch APIs, journal idempotence, and credential-fixture schema
corrections remain intact. This provenance is not qualification or an
installed-update claim.

For this action the approved updater runs once in a newly owned process group,
with a two-minute deadline. SIGINT, SIGTERM, or deadline cancellation stops
only that group, escalates after a short grace if needed, and reaps the direct
child before Ma'at writes a failed/incomplete terminal receipt. The native
Casebook keeps the same case action in its visible in-progress state until the
CLI returns; retry remains behind a fresh casebook review. No resolution is
claimed after cancellation, timeout, or unconfirmed child cleanup.

## Snapshot history and updater receipt integration

The integration starts from the separately recorded merge `06a1b4ff0f54ffaffc07b14bc5dd7313bf2402a0`
of corrective composition `003b00ed5f513a5eef87d2f03084b42bae43e639`
and released135 `b0e17c427abaa6d2b8330cba222c3ca0c92ef064`, retaining
the enrolled signer aliases, released publication record, and VERSION135.
The cumulative history delta is `9da345045251abcc5a8245e629e9d053adf63383`
through `cec5dac790d170741738246bca9efdbd001ffc48`. Its independent
87bd cumulative review and cec HW10 source closure remain donor evidence;
the combined source still requires Main's qualification.

History follows only exact binary-drift `OriginEvidence` links from the same
journal snapshot. One shared Casebook overlay applies owner acceptance,
terminal updater outcome, then pending attempt before text/status filtering
and bounded page selection, and again during page projection. A successful
binary receipt closes only a binary-only screen with a passed floor; other
findings and failed floors retain the typed re-screen action. Failed receipts
remain unverified; pending attempts retain the typed read-only inspect route.
All three evidence fields are searchable in CLI and native Casebook.

Opaque cursor version3 requires refresh of version2 continuations. The reader
retains only the selected relationship per candidate (acceptance supersedes
outcome, outcome supersedes attempt), plus the existing hidden-review link.
The existing limit+1 references, cursor/record limits, per-request budgets,
bounded atomic record/dependency allowance, snapshot rechecks, and cancellation
rules remain. Every selected dependency reread is charged. Tolerant history is
read-only and never releases an updater claim or bypasses strict journal
mutation, replay refusal, fresh preview, confirmation, or verification.

## Casebook to the existing native resolver

The remaining generic host-finding gap is navigation custody: Casebook showed
the retained prescribed step but offered only copying or owner review even
when the current native diagnostic already had a controlled resolution. A
per-finding control now opens `MaatDiagnosticResolutionView`, retaining the
original case evidence on the return screen. This bridge reads the canonical
installed CLI; it never dispatches `claim`, `fix_hint`, producer titles, shell
text, imported source screens, or a command from the retained screen.

The retained v1 host/diagnostic screen must name this exact local hostname and
its canonical SHA-256 finding identity/evidence. A fresh complete successful
diagnostic with a recognized status must contain exactly one active warning
for that typed identity. Only fresh operands are shown. Opening the existing
`FindingView` rechecks the semantic snapshot and local freshness; its existing
confirmation, exact argv/directory launch guard, once-only ResultView capture,
and specific-read verification remain the action authority. No repair runs
when merely opening either review screen. The updater, liveness-watch and
managed-disabled-service checks retain their dedicated Ma'at preview/journal/
replay fences and cannot enter this generic bridge, including legacy screens.

Back returns to the original evidence. A separate confirmation can run exactly
`maat triage --confirm --json` once to record a new local observation. Complete
exit, exact local host/diagnostic schema, snapshot/head and decision evidence
are required before presenting that write as confirmed. Failure/unknown output
is retained and points to Casebook inspection, never automatic write replay.
A fresh screen does not accept or discharge the historical case and does not
attribute recovery to a repair. Existing Casebook owner review/acceptance stays
explicit; unchanged, unavailable and foreign observations never invent a fix.

The review read has an explicit cancel control that cancels its owned task and
invalidates delayed results. Re-screen cancellation retains the bounded capture
receipt, labels any unconfirmed write honestly, and offers Casebook inspection;
it never renews the once-only journal-write latch. Leaving the view cancels its
owned waiting tasks and never launches a follow-up action.

`MaatDiagnosticResolutionTests` authors thirteen inert test methods for canonical
wire identity, hostile hint nonexecution, foreign/imported/malformed/duplicate
custody, missing/clear/ambiguous current checks, failed/partial observations,
dedicated repair isolation, stale semantic snapshots, late reads, exact once-only
re-screen argv, unknown write custody and contradictory fresh-screen receipts.
The existing `RemediationExecutionTests` remain the confirmation/launch/result/
verification regressions. Recipe component `maat-native-resolution-surface`
names both new files; the component count remains29. This source has not been
built or executed by the author. Main qualification and installed native UX
acceptance remain required; other original Ma'at ledger gaps, including guided
archive corruption restoration and unresolved operational rows, remain open.

The controlled-resolution destination owns its model as a strong member of the
value retained by the existing `Nav.Frame`, with `ObservedObject` for the mounted
view, following `EmptyTrashView`. It does not rely on remount-scoped `StateObject`
storage: pushing the existing resolver or Casebook and returning with Back must
retain the original evidence, pending/unknown journal-write receipt and once-only
latch. Disappearance still cancels owned tasks; cancellation invalidates read
authorization without resetting write custody. Two actual `Nav.push`/`pop`
fixtures cover the resolver/Casebook round trip and weak-reference frame ownership
after the original destination variable is released, including pending and
completed-unknown writes. No global Nav change or registry
is introduced. Main's original5f9 qualification (native402146d9f0b34dc798f56ff349a65615,
63 focused Release tests/no skips and production Release build) stays separately
bound; this navigation-lifetime correction requires its own qualification.
