# Pantheon installed Apollo client v1

This document records Pantheon's installed-client projection of the Apollo
cluster inference proposal in
`sirsi-inference/docs/architecture/APOLLO_CLUSTER_INFERENCE_V1.md` at
`86f92d0d32fc41354f77057e72c8b93fa44d2d62`.

It adds no inference engine, model loader, rank process, SSH route, or local
fallback. `sne.openai-chat.v3` remains its existing plain/MTP surface. The
new CLI is currently disabled until an enrolled authentication transport and
authenticated plan inventory are actually integrated. HTTPS alone does not
authenticate a caller. The intended command projection is:

```text
sirsi apollo capabilities --endpoint https://apollo.example.ts.net
sirsi apollo chat --endpoint https://apollo.example.ts.net ...
sirsi apollo admit --endpoint https://apollo.example.ts.net ... # chat alias
sirsi apollo status --endpoint https://apollo.example.ts.net --job-id JOB --job-receipt admission.json
sirsi apollo events --endpoint https://apollo.example.ts.net --job-id JOB --job-receipt admission.json --after N
sirsi apollo cancel --endpoint https://apollo.example.ts.net --job-id JOB --job-receipt admission.json
```

The user supplies a frozen model/plan/session/token request to `chat`; the
authenticated transport supplies caller identity. There is no `--token`,
`--ssh`, subprocess, model-path, or unauthenticated local fallback flag.

## Required service response projection

The proposal currently describes routes and invariants but does not yet define
closed capability/job response objects. Pantheon therefore accepts a create
response only when it has this projection (additional or implicit fields are
not accepted):

```json
{
  "contract": "apollo.cluster-inference.v1",
  "job_id": "authoritative-task-id",
  "execution_id": "128-bit-execution-id",
  "request_id": "same-client-request-id",
  "plan_sha256": "64-lowercase-hex",
  "epoch": 19,
  "state": "admitting",
  "rank_fences": [{
    "rank": 0,
    "host_id": "m5",
    "fence": "opaque-fence",
    "generation": "opaque-generation",
    "expires_at": "2026-10-15T18:00:00Z",
    "memory_admission_bytes": 17179869184
  }]
}
```

Every rank must appear exactly once, have a live RFC3339 expiry, positive
admitted memory and a nonempty host/fence/generation. A router task lease,
placement-plan digest, or a claimed machine name is insufficient. The client
calls `GET /v1/apollo/capabilities` first and requires exact contract plus
`launch_available`, `stream_available`, and `cancel_available` all true for
the requested mode. A planned-only service is refused before the job POST.

`status` and `cancel` preserve the same identity checks. `events` is strict
SSE: each event must name the requested job, carry the exact contract and a
strictly increasing sequence after the caller's persisted sequence. Redirects,
non-HTTPS endpoints, response trailing bytes, and incomplete admission data
are rejected.

## API corrections pending Apollo/Pantheon/Ra agreement

Before an installed service is advertised as available, the PR #61 document
needs to adopt the response projection above (or supply an equivalent
versioned closed schema) and define:

1. exact `capabilities` JSON fields and the distinction between planned and
   launchable modes;
2. an exact job/status/cancel response envelope including all rank fences,
   admission bytes, expiry, and terminal release/orphan evidence;
3. a closed error envelope and stable HTTP status mapping; and
4. the installed authenticated transport configuration and trust anchor.

Those corrections are protocol work, not a reason to weaken the client. Until
they land in the service, `sirsi apollo chat` correctly fails closed rather
than claiming a model was launched.

## SSA correction, 2026-10-15

The original f6144e54 CLI constructed an ordinary HTTP client. It did not
authenticate callers. The corrected CLI refuses every remote operation before
network access. A non-nil library HTTP client is dependency injection, not proof
of enrollment, endpoint authorization, or transport identity. No credentials or
trust anchors are minted, copied, logged, or borrowed from the router.

`CreateJob` additionally requires a separate local rank/host inventory obtained
from the authenticated frozen plan matching the request digest. Missing inventory
is refused before network access. Admission requires exact set equality: missing,
extra, substituted ranks and changed host identities fail. The job response must
never supply its own expected inventory. CLI integration of the actual verified
plan remains blocked on the implemented service contract.

`Events` takes the original admitted Job rather than an unbound job ID. Contract,
job, execution, request, plan digest and epoch must match on every event before
the callback receives text. Sequences must increase past the supplied cursor.
The planned CLI requires a frozen admission receipt and checks its job ID; a
fresh status response must not replace that receipt on reconnect. Token text,
token ID, position and committed count are typed optional fields, preserving
zero IDs/positions. They are emitted only on the authorized command output.
No general telemetry gains prompt or reply content.

This is platform groundwork, not installed-path qualification. Native enrolled
authentication, foreign-endpoint/unauthorized refusal, authoritative preparation,
plan schemas and signed-plan verification, normal human text chat, signing,
installation and real streamed reply/cancellation acceptance remain unqualified.
The prior claim that a nonempty fence list alone verified every planned rank is
withdrawn. No local tokenizer, single-host substitution or new wire schema is
introduced by this correction.

## Recovery and stream completion correction

Historical admission receipt validation checks immutable tuple and fence
structure, permitting expired original expiry timestamps for status, cancel and
event reconciliation. Only fresh admission and live response validation require
live fences. This does not renew resources or authorize continued computation.

Live streams have a per-event bound, with no cumulative byte cap and no retained
stream history in the client. A recognized terminal event must be delivered
before Events returns nil. Empty/early EOF, transport break and oversized lines
return IncompleteStreamError carrying the last accepted sequence. Reconnect
uses that cursor and the same frozen receipt. Terminal stream delivery is not
a claim of successful inference: the terminal event names the job outcome.
The provisional terminal event vocabulary still needs the implemented service's
closed schema before this disabled client can be enabled.

Cancel currently transmits only the existing contract body. Response tuple
validation cannot undo a mutation sent to a rebound job ID. Before enabling any
normal path the service must enforce immutable, owner/session-authorized job IDs
or an agreed atomic tuple precondition, with a stale-target fixture proving
zero worker mutation. No unilateral wire precondition fields are invented here.
