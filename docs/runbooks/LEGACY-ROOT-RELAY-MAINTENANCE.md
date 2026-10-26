# Legacy root relay maintenance adoption

Source candidate only. This fallback does not gate Studio's supported
producer-covered route, published135, or SHA's installed pilot. It does not
change `launchd restart`, its strict selected-instance checks, the five original
profiles, or Horus/surface/model ownership.

## Current-source handoff boundary

The current spool client takes the existing shared directory lock before it
creates a lane or publishes a request and retains it through response readback.
When the durable legacy-maintenance marker or preparation is present, a current
client uses one existing 30-second call budget across admission and response
readback. While the marker or preparation is present it waits without creating
a lane, slot or request. If that budget expires before publication, the command
reports that the request was not published and may be retried after the exact
retained transaction is resolved; after publication, the existing consumed or
outcome-unknown classification applies. The maintenance UI keeps a typed
`prepared` transaction actionable through its existing reviewed Resume/Abort
controls; it does not report a drain.
The owner waits for current shared calls under the same physical directory's
exclusive lock before retirement. If that bounded wait expires, the prepared
transaction remains held for explicit recovery. The current relay tracks and
joins its lane/request workers on shutdown, and a failed response publication
leaves the original inflight/outbox object as recovery evidence. After joining,
`Serve` returns an unresolved-settlement error if such evidence remains; worker
exit alone is not reported as a clean settlement.

This is cooperative behavior of the current source only. The exact loaded/source
matches for 121, 122 and 129 do not establish their shared-lock admission or a
joined shutdown, so none is added to the maintenance profile. An installed old
app can receive the normal signed app replacement, but its already-running
process keeps its old code until the canonical app is relaunched. That makes the
current maintenance UI reachable; it does not make an old relay/client safe to
drain. For those old sources, in-flight remote outcomes remain explicitly
unknown and are never replayed automatically. Their lossless session-preserving
retirement remains open pending an actual source-supported join boundary.

## Closed operation

The canonical app engine exposes `launchd legacy-maintenance` with `preview`,
`confirm`, `status`, `recover`, `abort`, `restore-preview`, and `restore-confirm`.
There is no arbitrary label, PID, executable
command, force, `--drained`, timer release or credential mint. These existing
registered-agent verbs do not prompt for privilege. Their backend requires
real/effective root **and** the existing registered
actor/thread/exact lifecycle-service grant. Authorization happens before the
physical spool's exclusive publication barrier; no RPC may run under that lock.
These verbs are the application integration API, not a manual recovery recipe.

Preview needs an explicitly selected retained, root-owned signed legacy payload.
The first closed source profile is released134 commit
`ba2a07bb464bbfd68808254a7d489769f122149b`, inspected for consume-before-forward.
Other old releases are not inferred from their names. Preview independently binds
the old kernel PID/start and signed CDHash to that payload, plus the currently
installed replacement, plist and physical spool. The protected program is
`/usr/local/libexec/sirsi-pantheon/sirsi`; target is exactly
`system/ai.sirsi.router.relay`, with the existing six argv, root0600 plist and
`_sirsipantheon` service identity. OS-disabled or unknown intent refuses adoption.

The Security.framework bridge requires a kernel PID-plus-CDHash selector to
succeed and a deliberately wrong CDHash to return no-such-code. Unsupported or
ignored selectors refuse preview; on-disk signing information is never relabeled
as loaded-image identity. No-cgo builds explicitly lack this native operation.
Actual native selector behavior, signatures and read-only system enumeration
still need owner-admitted qualification. These source checks are not such proof.
The framework call runs in a fixed read-only signed canonical engine probe,
bounded and joined by the controller's existing native command runner. No C-call
goroutine is left running after a synthetic timeout. Probe verbs suppress the
general CLI pre-run and do not resolve router authority or start the relay.

The replacement must implement the fixed read-only capability verb and match the
embedded startup/publication/gate implementation digest. A modern/occupied maintenance
endpoint is refused by this legacy profile; use its supported producer route.

## Durable transition and custody

Explicit confirmation binds the complete preview digest and acknowledges
interruption with **unknown old remote outcomes**, not a completed old-work drain.
The private root0600 append-only transaction journal lives beside the selected
plist at `/Library/LaunchDaemons/.ai.sirsi.router.relay.maintenance.jsonl`.
It binds transaction/revision, exact plan, phase, verified successor generation
and retained count. It contains no token, plist environment or request body.
Phase CAS rejects overlap, reuse, skipped phases and damaged/torn history.

Cancellation before retirement makes no service lifecycle change. If preparation
has already published its durable marker, that scoped hold remains explicit
recoverable debt; it is never silently deleted on error. A cancelled caller does
not authorize a bootout, and recovery must revalidate the same selected plan.

The phases are prepared, retirement_requested, retired, custody_preserved,
bootstrap_requested, activation_requested, activated, completed. A durable
root/service-group startup marker and exclusive physical directory flock keep
updated clients closed even after controller death. Existing shared admission
covers publication through settlement; aliases bind the same physical directory.
Uncovered legacy writers remain explicitly uncertain. A lock or an empty scan
cannot certify lossless old in-memory work or remote completion.

Only the exact authorized launchctl bootout may retire the pinned job. A command
error is not absence. Recovery requires kernel disappearance of the exact old
instance plus a successful complete **system-domain** job inventory. Old-alive,
PID reuse, another selected job or an unknown probe refuses bootstrap and never
repeats bootout. After positive retirement, recovery has an independent15s bound.

Before bootstrap, req, inflight and outbox envelopes are moved create-only into
each lane's existing private `failed/.maintenance-TRANSACTION` namespace. Private
write-ahead audit leaves preserve original lane/name bytes, device/inode/size and
unknown classification. Envelope contents are never opened, printed or replayed.
Existing response/audit files are untouched. Unknown queue objects, names, links,
policy or continuity changes remain retained debt and refuse activation. Newly
created staging objects may be prepared; existing objects are never chmod/chowned.
The startup marker is this operation's own exact metadata contract. Existing
health-summary classification consumes Nash's qualified d802 shared namespace
correction in this source composition. No competing health schema or metadata
allowlist is introduced; unknown files remain retained debt.

The next relay reads the durable marker before observation publication,
recoverInflight, discovery, retry or housekeeping. It starts in
`maintenance_ready_not_drained` with actual kernel identity, fresh generation,
transaction/root and retained count. Root peer authentication and exact state CAS
bind activation. Cancellation, elapsed time, reboot and client disconnect cannot
release the gate. A lost activation reply is re-queried; a rebooted held successor
gets a fresh persisted generation before activation. The marker is removed only
after verified activation; final durable completion follows marker cleanup.
A complete prepare intent is atomically published before the private journal.
Both startup-control records are staged outside the queue namespace. A private
uncommitted stage has no lifecycle authority; a committed prepare holds startup
even across a crash before journal append or marker publication. The prepare
remains until completion is durable. A restart in the marker-cleanup window is
therefore held with a new generation, which recovery must rebind and verify.
Preservation and readiness share global envelope, audit, entry and lane bounds.

Completed activation with retained debt still authorizes **zero** replays and
proves nothing about old canonical effects. Existing authenticated read APIs may
later reconcile exact original IDs. This operation does not invent results,
sessions, nonces or a second router store.

## Application and remaining limits

The GUI remains unprivileged. Its frame-owned maintenance model now sends one
bounded typed public request through the existing macOS administrator-dialog
mechanism to `/usr/local/libexec/sirsi-pantheon/sirsi launchd legacy-maintenance-app`.
The AppleScript source and executable are constant; base64 request data is a
separate argument and shell-quoted by AppleScript. The root command receives a
fixed minimal environment and `/` working directory, never GUI actor/session,
checkout, credential or arbitrary command state. No helper daemon or router DB
is installed. This source bridge still needs independent qualification; no
actual privilege dialog or service transition has been run by this work.

Both sides require the protected root-owned, non-writable, no-follow installed
engine, no ACLs or hardlinks, exact Sirsi signature and the captured engine hash.
The app also validates its own signed Pantheon identity. A missing, drifted or
incompatible protected engine refuses authorization. The panel's Check compatible
Installer action selects the exact running release's normal PKG through the
existing updater client. It verifies the published digest, Installer signature,
Gatekeeper/notary assessment, closed one-app replacement metadata and matching
native helper capability. Open verified Installer consumes that captured
selection only after a second byte/identity/trust check. Normal macOS Installer
remains responsible for installation; Recheck installed pair is a separate
explicit readback. An older incompatible package or DMG drag-only handoff is not
offered as helper repair. The app never elevates a user-writable bundle CLI or
PATH substitute.

Only this entry uses local OS-authorized root authority for the closed root
profile. Its controller authorizer rechecks installed
engine identity. The existing registered-agent CLI and remote authorization are
unchanged. Root revalidates closed fields, complete plan/transaction digests and
restore bytes before invoking the same controller and journal implementation.
Operation errors retain a bounded frame and original controller bytes. Cancel
or lost transport after possible launch is outcome unknown, never denial,
rollback or proof the root child stopped. Consent is consumed once; a frame-owned
receipt survives view remount. Status and explicit same-transaction recovery are
available; uncertain no-history observations do not authorize reconfirmation.
App preview also persists a bounded admission ticket, not a service transition.
Under the existing selected-service lock each effect checks the complete ticket
and records running intent before work. Recovery, abort and restore receive
separate operation tickets. Check result and prevent delayed submission uses
that same lock to return a matching terminal result or durably fence late
arrival. Old/empty status cannot clear another outstanding operation. Only a
verified fence/result with zero lifecycle debt permits fresh preview and consent.
The app retains the exact public binding and unknown receipt across navigation
and restart; it never relabels a running-intent result as never-run.
Unreadable public preferences expose Inspect public binding recovery, not a
manual-delete prerequisite. Read-only inspection is followed by explicit
confirmation to recover the exact controller ticket or obtain a fresh serialized
preview reservation for the selected retained payload. After local persistence
and readback, the existing result/fence control must resolve that ticket before
new service preview/consent. Empty status cannot unlock the latch. Known B is
never overwritten by older status A. Busy/corrupt/changed authority and failed
public readback remain actionable retry/debt, with the verified ticket retained
when available. The unreadable byte count and bounded-prefix SHA256 remain
explicit unknown-past-outcome metadata; no raw invalid preferences are displayed.
Installer identity checks now use the same bounded asynchronous preflight as
the root bridge, with off-actor signed-app/file qualification before either
preparation or final verification. Late identity completion, cancellation,
changed generation and expired/consumed consent cannot submit or open Installer.
Abort re-verifies the original alive and cannot run after retirement may begin.
Restore accepts only the originally approved replacement bytes and signed identity,
binds the current protected target and parent, retains create-only backup/stage
paths, and performs no service action or envelope replay. Application restore
confirmation forwards the exact captured bounded plan bytes to the engine.

Fresh authorization for all legacy maintenance verbs uses the exact selected
root0600 relay plist's existing direct canonical route, revalidated before/after
the lookup. It requires native root and the existing exact service profile. URL
and host token stay in memory; no environment export, new token, saved session,
spool bypass or database fallback is introduced. The ordinary registered
actor/thread, machine/runtime and explicit registry grant checks are unchanged.
The service still mints/verifies the actual caller's session. A scoped token
still carries its original scope; possession never supplies a missing grant.
Missing direct authority, disabled/cancelled intent or drift refuses the action.
Ordinary modern refresh continues using its existing authority selection.

That direct route resolves the held-spool dependency for the registered-agent
controller. The separate ordinary-app adapter uses only narrow OS-authorized
local root authority and the protected engine; it does not invent a GUI
actor/thread, change agent authorization or manufacture a rootless registry.

Journal damage, unknown retirement, publication/permission failure or unmatched
recovery identity remains explicit recovery debt. Recovery reuses this exact
transaction, never blindly restores the old payload or clears unknown envelopes.
Point-in-time retained-descriptor checks cover cooperative ordinary substitution;
they do not prove exclusion against an independently authorized root/same-UID
writer after the final check. The kernel code-identity bridge and parsed native
inventory are engineering candidates, not executed host evidence.

All source, native fixture, signed successor, application integration and actual
installed transition qualification remain separately accountable. No service,
host security setting, process, model, credential or live spool is changed by
authoring this source. SSA remains publisher; SHA owns Studio-first adoption.
