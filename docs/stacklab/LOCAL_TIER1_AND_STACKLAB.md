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
The child leader is observed without reaping through the already-used Darwin
proc_pidinfo API. Darwin Python has wait flags but no os.waitid entrypoint, so no
unavailable waitid primitive is assumed. Every local group signal rechecks its
retained leader start/parent and member UIDs. Quiescence is checked before reap
and actual group absence after reap. Unknown/reused leaders are never signalled.

A durable per-execution refusal fence is written under the existing kernel lock
before spawn. Cleanup uncertainty retains that fence and exact debt; another
local admission refuses. `--reconcile-local <exact execution>` performs only
bounded kernel/readback checks, no signal/replay, and resolves only matching debt
whose original group is actually absent. Unreadable/foreign/mismatched debt is
not absence and is never silently cleared. The existing fabric cleanup is not
rewritten as part of this local correction. Cooperative same-user behavior is
the boundary; no hostile-UID atomic isolation is claimed.

Local receipts now have create-only immutable pending, intent, running and
terminal phases. A full private stage is written/fsynced before atomic no-replace
publication and directory fsync. Failed short writes, fsyncs or publication keep
the prior complete intent. The default fabric writer is unchanged. Fault fixtures
exercise both the phase primitive and a running update after a mocked spawn.

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
