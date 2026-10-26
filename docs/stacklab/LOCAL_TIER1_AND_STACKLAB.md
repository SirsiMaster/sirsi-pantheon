# Tier1 routing, local execution custody and opt-in Stack Lab

This successor continues SSA's existing `maat-local-first-admission-20261022`
obligation and original132411/132434/104659. It follows the owner-directed
service-tier contract without creating a new ADR, router database, scheduler
backend, Ra session, remote permission, daemon or model service.

## Normal local commands

The front door defaults to `--scope auto`. A regular executable in the known
installed Sirsi layout is a candidate only for Tier1 command families. A bounded
local version read must advertise the same local-execution schema. Unsupported
or older engines do not receive invented authority. Explicit `--scope fabric`,
executor registration, recurring/quiet/loaded work and other commands retain the
normal fabric path; transport failure never changes their scope.

The executing CLI is authoritative about its closed operation set. Before any
Cobra command, it validates the private immutable local fact's exact parent PID,
kernel start, argv hash, ordinary UID, CPU budget, schema and finite deadline.
It then consumes that direct-child fact so descendants cannot inherit it as their
own execution. Root/elevated/mismatched identities are refused in local scope,
including before the local capability probe. No credential, router identity or
configuration is swapped. Local facts never satisfy native management budget or
remote/SNE authority; those existing consumers are unchanged.

The closed set includes version/vitals/diagnose, basic local Ma'at triage and
decision/casebook reads, basic Thoth status/init/sync/compact, local liveness reads,
and local Stack Lab commands. Accepted flags are deliberately enumerated in
localreceipt.Allowed. Unknown flags/commands are refused rather than promoted to
global or model scope. Thoth sync/compact use the existing Go implementation for
a verified local invocation rather than delegating to an external executable.
Direct ordinary CLI use and normal fabric invocation retain their existing paths.

## LF01–03 custody corrections

Local scope refuses real/effective root and mismatched user/group identities.
The child leader's start/parent is retained through Darwin proc_pidinfo before
payload release. Terminal observation uses public libSystem waitid with
WEXITED|WNOHANG|WNOWAIT and the SDK's LP64 siginfo_t ABI; Python's absent
os.waitid wrapper is not assumed. The actual Popen handle, PID, parent, start,
effective/real UID and process group are bound from live libproc before release.
A positive exact owned-child PID/status receipt keeps that child waitable;
ECHILD, missing/drifted/reaped handle custody, malformed/foreign results or an
unavailable libproc read alone never prove exit. Every local group signal checks
its retained leader binding and member UIDs. Quiescence is checked before reap
and actual group absence after reap. Unknown/reused leaders are never signalled.

A durable per-execution refusal fence is written under the existing kernel lock
before spawn. Cleanup uncertainty retains that fence and exact debt; another
local admission refuses. `--reconcile-local <exact execution>` performs only
bounded kernel/readback checks, no signal/replay, and resolves only matching debt
whose original group is actually absent. Unreadable/foreign/mismatched debt is
not absence and is never silently cleared. The existing fabric cleanup is not
rewritten as part of this local correction. Cooperative same-user behavior is
the boundary; no hostile-UID atomic isolation is claimed.

The owned child initially waits on a private pipe in this same script. It keeps
the kernel-lock descriptor. Only after its PID/start and running phase are fully
durable does the parent release it to exec the exact payload under the same
PID/start/group. EOF or a bounded missing-release timeout never execs the payload.
If a supervisor dies before retaining running identity, reconciliation requires
the exact protocol-tagged immutable intent and its execution fence; it records
payload-not-released by protocol, not absence-as-safe or an invented run.
If identity was retained, kernel group absence is required. Neither path resends.

Local receipts now have create-only immutable pending, intent, running and
terminal phases. A full private stage is written/fsynced before atomic no-replace
publication and directory fsync. Failed short writes, fsyncs or publication keep
the prior complete intent. The default fabric writer is unchanged. Fault fixtures
exercise both the phase primitive and a running update after a mocked spawn.

## Resumable local recovery

The LFS recovery successor completes the final SIGKILL wait through the same
identity, quiescence, reap and absence checks. Typed immutable intent is durable
before its execution fence, and that fence remains durable before spawn.
Reconciliation atomically publishes a resolution bound to the exact original
evidence hash. A matching resolution resumes interrupted archive/unlink/fence
durability steps under the same lock; original debt bytes remain archived.
Conflicting, malformed, foreign or still-live evidence refuses. No retry signals
or replays the payload, and empty state alone cannot authorize clearing a fence.
The new fault fixtures are authored evidence, not executed native qualification.

The first native owned-true qualification of1c compiled the Go CLI but refused
the local execution: libproc did not supply the terminal process identity, and
the process-table parser rejected an unrelated macOS UID -2 row. The successor
accepts signed UID syntax without treating a negative/foreign UID as eligible.
It uses the non-reaping owned-child receipt above for terminal observation and
exited-anchor custody. That actual failed run, its private debt and fence remain
retained; no author cleanup or retroactive success is claimed. Main must prove
the real wait ABI and lifecycle again before crediting the larger suites.

Apple's published XNU waitid exit branch zeroes siginfo but does not fill si_uid
([kern_exit.c, xnu-11215.81.4](https://github.com/apple-oss-distributions/xnu/blob/xnu-11215.81.4/bsd/kern/kern_exit.c#L2960-L2997)).
Thus that zero is not a root/owner credential. Terminal observation requires the
retained live-child binding above; a populated foreign UID still refuses.
Cleanup independently requires every observed group member's UID to match before
signal/reap/release. This explains8eb's observed fields without claiming the
published source tag is the exact installed kernel. Its diagnostic debt remains
retained; neither source inspection nor inert fixtures replace Main's real probe.

## Installed Stack Lab, off by default

`sirsi stacklab local inspect|enable|disable|catalog` uses private per-user
preferences. Missing settings inspect as disabled/local without creating files.
Only `local`, `frontier`, and `self-hosted` provider kinds are accepted. Settings
contain no endpoints, tokens, prompts or model credentials. This is functional
provider-kind selection, not provider enrollment, model execution, or proof of
connectivity. Existing corrupt/ambiguous/linked state is refused, not replaced
with a manufactured default. Writes use the retained local root, an exclusive
private file lock, a private stage, atomic publication and exact readback.

The installed app's recipe payload is projected by the existing Go catalogue
parser. No developer checkout is needed for `local catalog`; source `stacklab
catalog` and explicit canonical fabric `doctor` keep their existing contracts.
The native Stack Lab root inspects only local preferences on opening and offers
enable/disable, provider selection and installed-recipe inspection without a
project. Fabric wing checks are a deliberate separate navigation. Missing
installed payload offers the existing release workspace and retry. A failed
read labels prior state/catalogue as retained; disabled/unknown is never green.

## Proof boundaries

Authored fixtures cover normal automatic routing and exact immutable producer
facts; Go validation/real entry refusal and native-budget non-authority; root/
elevation, anchor reuse, group quiescence, cleanup-debt reconciliation and partial
phase failures; provider persistence, off-default reads, linked/corrupt/ambiguous
configuration and a real versioned recipe/wing roundtrip; native local commands,
provider selection, unavailable state and catalogue/disable behavior.

No author tests/builds, process signals, network changes, installed service/model
actions, GUI execution, current-host settings or canonical task/store mutations.
This candidate needs independent source review and Main's normal isolated native
qualification. It does not claim full local-first delivery, all resource classes,
three-Mac rollout, physical network-down acceptance, ingredient/comparison/run
execution, peer sync or global permission/enrollment. Original owner obligations
and existing remote authority remain open where not actually proved.
