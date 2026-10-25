# Root relay loaded-client refresh — corrective recipe

## Released136 continuation: actual producer and pass boundary

The signed public136 files are installed on M1, M5 and Studio and the installed
client executed its own read-only task/inspection calls on each. M5's GUI was
normally quit and reopened into the exact136 image. These are separate facts
from refreshing older loaded relay, Horus, router surface and Gemma roles.
Installation alone does not implement a drain in the old process.

SSA owns the remaining correction under the original
`native-service-lifecycle-productization-20261022` task; SHA remains its native
operator. Main's current product composition does not implement this boundary.
The canonical baseline is released136 eff2c45d, not an unreleased development
binary. Existing session/data/queue protection and selected-service pins below
remain mandatory; this plan does not authorize a live apply.

### Required state and evidence

1. Identify every actual selected producer/runtime, including Go duties and
   background polling controllers. Record which installed/loaded implementation
   actually cooperates with the existing retained-directory admission lock;
   unknown or legacy code is not silently classified as cooperative.
2. Stop admission of new passes/forwards through a supported, instance-bound
   control. Acknowledge only after every previously admitted pass/forward has
   finished its response/outcome recording. No OS-child sample substitutes for
   this acknowledgment. The foreground lane's promise is not a background
   controller's acknowledgment.
3. Hold that no-new-work boundary across the pinned lifecycle transaction and
   queue/namespace checks. Preserve queued requests and outbox order. Any
   ambiguous mutation outcome remains parked for canonical readback, never
   replayed to make a drain appear successful.
4. Resume ingress on normal completion and on bounded failure/recovery. Keep
   original stand-down intent; do not enable an already-disabled job. A timeout
   or missing acknowledgment is a failed drain, not permission to signal work.
5. Verify the actual replacement image, singleton/role identity and harmless
   canonical queue operation on the destination. Retain rollback and every
   failure receipt; do not equate a fresh process or heartbeat with completion.

### Implementation order and legacy constraint

First close the relay's source-level cancellation gap: current `Relay.Serve`
can return while lane workers/forward goroutines still own requests. New-runtime
completion must distinguish fast cancellation from an acknowledged drain and
must account for both fresh requests and serial held-outbox retries. Preserve
finite deadlines, at-most-once handling and independent lane progress.

Then integrate the supported producer/pass control with the existing lifecycle
controller and each actual role. Use closed owner-authorized control, not a
second daemon, copied token or arbitrary-command socket. Reuse the existing
canonical runtime authorization and exact instance/configuration pins.

Finally qualify the **first migration of each legacy loaded implementation**.
A new API or lock in replacement bytes cannot retroactively govern old memory.
Establish that legacy runtime's real supported completion/no-new-work boundary
before retiring it; preserve it if the predicate cannot be established. This
must be solved as migration work, not relabeled as completed by a new-runtime
unit test, an empty queue sample, or a caller-provided `--drained` flag.

### Acceptance controls

#### Source implementation checkpoint: joined relay duties

`Relay.ServeDrained(ctx, drainTimeout)` is the explicit in-process completion
API. It tracks the actual lane workers and fresh forward goroutines; a worker
also owns its serial held-outbox retry until the existing disposition finishes.
Discovery stops on cancellation, canceled workers do not start another queued
item, and cancellation leaves the next held-outbox item untouched. A positive
bounded shutdown deadline is required. A missed deadline returns
`ErrRelayDrainIncomplete`, never success or authority to retire the process.
The already-admitted HTTP request is not canceled or replayed by that deadline.
The original `Serve` retains its prompt cancellation contract and is explicitly
not a drain acknowledgment. Each Relay instance has one serving lifetime;
reusing retired lane channels refuses instead of silently losing wakes.

This checkpoint does **not** switch the installed CLI to terminate on a drain
timeout. The remaining authorized control must keep the process alive on a
failed drain, fence producer ingress, bind the actual instance, and satisfy
the legacy migration requirement. Neither this API's unit tests nor fast SIGTERM
handling is a substitute for those next steps. Existing on-disk outcome-record
durability is not strengthened or newly certified by a goroutine join.

Use owned fixtures to hold a forward and a held-outbox retry across a drain;
the acknowledgment must wait, prevent a late new forward and preserve all
pending bytes. Cover timeout, interrupted caller, owner/instance substitution,
concurrent lifecycle attempts, outcome-unknown retention, resume after failed
apply and a successful normal replacement. Show the old implementation fails
the missing-completion control before the new implementation passes it. Existing
short-cancel callers must not receive a false drain-success result.

Source review and protected CI precede the next signed commercial candidate's
package/catalog/tag/release. Fresh SHA per-host loaded-role/readback and actual
queue receipts finish the operational requirement. Whole-product, remote GUI
and M5-off autonomy claims remain separate and unproven.

Native Ra outbox **Guided resolution** retains its selected agent/root in Ma'at; component mapping and observation/confirmation semantics live in [Relay-owned outbox observation](../../../runbooks/RELAY-OUTBOX-OBSERVATION.md). The guide may re-observe or invoke the existing confirmed same-user repair only. Service-private publisher upgrades remain operator-managed under this recipe; navigation to the guide does not authorize any lifecycle step below.

Status: corrective source work after published v0.24.124; not installed proof.
Owner: SSA integrates/releases; SSA or SHA performs one nominated host operation.
Classification: platform-foundation, repairing an existing installed workflow.

## Contract

The user needs an upgrade to reach the running relay, not merely replace its
on-disk executable. Reuse `sirsi launchd restart`, `ServiceController`, the
canonical remote actor/thread check, and explicit registry grants. No second
router, new background helper, alternate installer or privileged prompt exists.

The profile is exactly `system/ai.sirsi.router.relay`, with the existing root
LaunchDaemon and protected `/usr/local/libexec/sirsi-pantheon/sirsi` program.
Its existing six arguments end in `--spool` followed by exactly one observed
literal: `/var/sirsipantheon/relay` or `/private/var/sirsipantheon/relay`. These
are the only accepted spellings; no normalization, symlink resolution, alternate
alias, trailing separator or extra flag is admitted. The selected plist is not
rewritten. Its service user is exactly `_sirsipantheon`. Controller privilege does not change
the relay's run-as identity. The credential-bearing plist remains root:wheel0600.
The five existing maintenance profiles retain their behavior. Only SSA and SHA
receive the additional exact-label grant; the source lane receives none.

## Sequence and evidence

1. Inspect through the existing qualified privileged controller. Retain the
   returned PID, start identity, plist digest and protected-program digest.
   Never export the credential-bearing plist or launchd environment.
2. The owner quiesces request producers, drains the selected relay's application work and retains its
   recovery plan. `--drained` records that assertion; it does not manufacture
   a global dispatch drain or permission to stop another lane.
3. Apply the four pins through the existing command. Under the selected-service
   lock, re-inspect the exact instance and refuse children or an indeterminate
   child probe before retirement. Inspect queue directory metadata without reading
   request contents; refuse pending `req`, `outbox` or `inflight` entries, unsafe
   directory paths and unreadable state. Refuse drift rather than retiring a replacement.
4. Retire only the selected launchd job, bootstrap the same pinned artifacts,
   and bound the fresh readback/recovery by the command context. Never enable
   a disabled job, signal an arbitrary PID, or widen the service allowlist.
5. Record the new instance, same artifact digests, loaded executable identity,
   and an actual harmless canonical-router request. Installation/version text
   alone is not loaded-client or delivery proof.

Relay forwards are Go goroutines, not OS children. An empty process-child list
alone is never a drain proof. The existing relay atomically moves requests into
`inflight` before forwarding; startup recovery reports surviving in-flight work
as outcome-unknown and never automatically replays it. Preserve that spool and
re-query canonical mutation outcomes before any retry. Legacy relay versions have
no atomic drain handshake: the first cutover requires real bounded owner-managed
producer quiescence as well as the sampled queue checks, not a fabricated flag.

The lock and repeated snapshots coordinate cooperative lifecycle callers; they
do not prevent an independent account owner changing launchd or a drained process
forking after its last observation. Owner drain is therefore load-bearing. A
failed bootstrap/readback must remain a failed operation with its exact recovery
outcome, never a successful refresh inferred from the old process disappearing.
An error or timeout from `bootout` is itself ambiguous: an independent bounded
read-only probe distinguishes the original live instance, a concurrent changed
instance, or outcome-unknown. None authorizes another mutation. A failed native
query is not proof that the job is absent; preserve the original failure and
probe result rather than repeating retirement or inferring successful recovery.
Backend recovery has its own bounded context after retirement; if the original
caller is canceled, the outer controller can still report failure after recovery
completed. That outcome is not permission to restart the recovered instance again:
inspect afresh and record the recovered identity first.

## Verification and release boundary

### Actual producer custody, not foreground-only readiness

The2026-10-24 M5 operation reached its02:10:30Z window on time but refused
before retirement. A separate `claude-finalwishes-web` Inbox request created
at02:10:30.597Z completed at02:10:30.707Z, overlapping the controller's
empty-queue refusal. Other background Pantheon and CompassPoint polls followed.
The later no-follow filesystem snapshot found all forwarding queues empty.
This is evidence of live producers outside the foreground acknowledgments,
not proof of stale residue, an unreadable directory or a defective guard.

Identify actual background controllers and their owners independently of
interactive chats. A foreground promise, thread suspension, dispatch hold or
measurement window does not prove a polling controller stopped making reads.
Use an existing supported finite no-new-poll control, preserving active payloads,
models, builds, sessions and heartbeat requirements. If no such control exists,
repair that concrete producer-custody gap before another maintenance attempt;
never force retirement, delete markers or replay ambiguous requests.

Launch a prepared finite operator before its observation deadline, then wait
for the future window in bounded clock chunks. Record the actual live handle,
prepared UTC and selected process identity, not merely an intention to launch.
Timing deferral before apply and native empty-queue refusal are distinct
outcomes. Neither is a completed refresh or authority to repeat blindly.

### Existing privileged invocation and read-only qualification

Use the existing `sudo -n` route to the reviewed, signed installed CLI; PR1191
is the existing controller implementation, not a separate daemon or PID. Do not
execute a development build as the privileged production controller. Before
`--apply`, the exact root-relay read-only plan must run the same actor/runtime
authorizer and then inspect, without refreshing. It must fail before inspection
if the canonical service, own live runtime or exact grant is unavailable.

Pass only the actual non-secret actor/thread and canonical spool selection needed
by the existing route, not an unrestricted copied environment, token or key. The
physical spool remains `/private/var/sirsipantheon/relay`; deployed CLI argv may
retain either of the two exact observed literals above. The retained no-follow
physical-spool probe and quiescence checks are unchanged. No HOME override is part of this
recipe. Actor ancestry and start identity must really match; environment labels
alone are not authority.

`FindRepoRoot` prefers Git's common main worktree even from a clean linked
worktree. A clean CWD therefore does not itself establish the effective registry.
`LoadRegistry` prefers that root's origin-pinned snapshot under the effective
root-context home (normally `/var/root/.sirsi/registry/agents.json` with
`agents.meta.json`). Inspect that actual source, matching RouterRoot and accepted
origin commit/digest containing the exact SSA/SHA grant, through the normal
qualified root route. Do not silently unpin or consume dirty primary edits. No
root configuration is created, changed or implicitly qualified by this source
recipe; absent/unusable canonical transport or runtime is a failed preflight,
not permission to create a local router or mint an actor.

If the root snapshot does not yet exist, the normal post-merge operator route
may install only the public, hash-verified accepted-origin registry and matching
metadata into root-owned cache paths. Fetch/verify those public bytes through
the existing authenticated user route; do not presume root GitHub/SSH credentials,
copy credentials or alter Git trust, HOME, sudoers or transport policy to perform
the initialization. Record the actual installed cache paths, owners, digest,
source commit and RouterRoot, then retry only the read-only authenticated plan.
This is configuration delivery for already accepted grants, not a new approval
request or permission to assert drain readiness.

Use injected backend/native dependencies for wrong privilege, path/argv,
children, unavailable probes, instance/artifact substitution, concurrent apply,
bootstrap failure and fresh-instance success. No fixture may operate live launchd.
Run normal/race focused suites and independent source review before integration.
SSA's composed protected CI and signed release remain separate from those proofs.
Live host rollout and rollback evidence remain open until actually executed.

No pricing or performance claim is made. This repairs the free operational base;
it adds no paid vendor, model requirement or new feature scope during the freeze.

## Reserved publisher metadata compatibility (released135 correction)

The existing publisher installs `.relay-outbox-health-v1.json` by create-only
staging and `Renameat`. It is a regular metadata file, not a lane. The supported
reader classifies only that exact name, using the shared observation closed
decoder and independently derived producer/root file policy. Canonical physical
root ownership comes from `_sirsipantheon` lookup and the retained root/parent;
the root's declared gid/mode determines the exact file gid and private0600 or
group0640 policy. JSON and controller privilege never select the producer UID.
The retained file must be regular, nlink1, bounded to131072 bytes, no-follow,
nonblocking, and without extra ACLs. The reader does not repair permissions.

Structural/identity validation is separate from health freshness. Valid stale,
partial or nonzero-count metadata is recognized but supplies no drain proof:
the live `req`, `outbox` and `inflight` checks remain authoritative. Legacy empty
roots need no summary. The exact `.relay-health-new-<32lowerhex>` publication
name is explicit in-progress/retained debt; neither it nor lookalikes are ignored
or deleted. Other files still refuse through the existing lane contract.

Each probe takes three bounded full root namespace observations (1024 entries,
16 entries per lane) around two executions of the existing queue probe. Lane
and queue name/identity/owner/mode/size/mtime/ctime continuity is retained.
Only a validated finished-summary addition or atomic inode replacement receives
the root mtime/size exception. In-place writes, disappearance, unsafe metadata,
new/changed lanes or queues, and unknown/staging arrivals refuse. A replacement
racing one metadata read returns changed; it does not spin or retry lifecycle
actions. Root/ancestor identity and permissions remain checked. This is still
cooperative sampling under the existing admission/drain contract, not a kernel
atomic-drain guarantee or the separate future legacy-maintenance protocol.

`internal/routerstore/spoolobservation{,_unix}.go` supplies the shared schema,
identity and file-policy boundary; the existing Darwin ACL implementation remains
fail-closed without CGO. `internal/setup/launchdrelayspool_darwin.go` owns only
namespace/queue continuity. The immutable `9238be90` regression file is retained
byte-for-byte for green attribution; added metadata continuity/shared-policy/ACL
fixtures cover stale/partial/count separation, atomic replacement, in-read races,
late queue arrivals, staging, schema/type/identity and synthetic ownership policy.
Only Main executes qualification. Main reports actual `9238be90` red execution
`a17376ae7d404f71931758079f6c8f9a` at
`/private/tmp/pantheon-relay-metadata-red-qualification.6Q10GX/receipt.json`:
both private/group fixtures and actual canonical publisher compiled and refused
with ENOTDIR before bootout. No corrected-source runtime or host retry follows
from that red receipt.
