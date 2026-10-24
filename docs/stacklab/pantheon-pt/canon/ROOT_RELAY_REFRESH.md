# Root relay loaded-client refresh — corrective recipe

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
