# Lane-scoped host tokens (schema v25)

Status: implemented 2026-10-12 (Ra). Owner direction: cloud-based lanes must message Mac lanes and each other
through the one cloud ledger without being able to act as any other lane.

## The problem
A host token is a host-wide credential, and the agent id is chosen by the client when it mints a session. Nothing
in the service limited which agent a token could claim, who a send was "from", or what a session could read
(`ListAll`, `Get` and `Inbox(any)` are open to every session). A token placed in a cloud sandbox could therefore act as any
lane, including `ra` and `owner`, and read the entire ledger. Mac tokens carry the same exposure; they sit in 0600 files
on machines the owner controls, which a cloud sandbox is not.

## The mechanism
`host_token_scopes(token_id, agent)` (v25). A token with rows is **lane-scoped**: it may mint sessions for, and act as, only
the listed agents, through a **deny-by-default lane API** (`internal/routerstore/lanescope.go`, one table). A token with no
rows is unscoped and behaves exactly as before (every existing Mac token). The bootstrap/operator token is unaffected.

Mint (service side only, never over the wire or the relay):

    sirsi router token mint <host> --label "<what it is>" --agents claude-finalwishes-m1,claude-finalwishes-web

Use a dedicated host name per cloud environment, never a Mac's host; revoke with `sirsi router token revoke <id>`
(effective on the next request; every session minted under that host dies with it).

## The lane API (everything else is refused with HTTP 403 `ErrLaneScope`)
| Group | Methods | Rule |
|---|---|---|
| Identity | MintSession, MintSessionForThread, RegisterAgent, Heartbeat | agent argument must be in scope |
| Threads | UpsertThreads, UpsertThreadCAS, ResumeThreadCAS, DeleteThreadCAS | the record's agent in scope AND, if the thread id already exists, its current owner in scope (no taking over another agent's thread by reusing its id); a deleted thread's owner in scope |
| Send | Send, SendGuarded | sender in scope; the recipient may be ANY agent |
| Read own | Inbox, Wait, ListenNotify | agent in scope |
| Items | Get, Render | caller must be the sender or the recipient |
| Work an item | AckItem, CloseItem, Complete, Fail, Block, StartWork, RenewLease, VerifyLease, SetBlockedBy, ReassignItem | caller must be the RECIPIENT |
| Claim | ClaimNext | agent in scope |
| Tasks | AddTask, GetTask, ListTasks, ClaimTask, ClaimNextTask, Complete/Release/RenewTaskLease, UpdateTask, ResetTaskAttempts, TaskEligibility, TaskLeaseEventsSince, VerifyCompletion, RunnableFor | agent in scope |
| Read-only context | ThreadBinding, ListThreads, ListAgents, GetAgent, OperationalAgents, ClassifyLane, NotifyAgent, NotifyPath | no restriction (registry and liveness metadata, a wake nudge) |

Refused by default: ListAll, ListPage, ListActive, ListSince, CountClosed (whole-ledger reads), another lane's Inbox, Get of a
stranger's item, ForceOwner, Counters, Breakers, requirements, identifiers, exports, and any method added later until someone
decides what scoping means for it. "No such item" and "not yours" return the same error so ids cannot be probed.
Every refusal is written to the audience log as `refused` (shown by `sirsi router audience`) and to the service log.

## What this does NOT fix (stated, not hidden)
- The token plus the session secret still authenticate the lane. A compromised cloud session is a compromised
  `claude-finalwishes-m1`; the scope bounds the blast radius to that lane, it does not make the lane trustworthy.
- Mac tokens are still unscoped. Scoping them is a separate, deliberate migration (it needs the per-lane agent lists for
  each host's registry sync).
- The owner console's local origin check is a different problem (same-user processes); see the F1 review.

## Verification (2026-10-12)
- 9 unit/wire tests in `lanescope_test.go`, each with a negative control (enforcement off: the 5 confinement tests fail; audit
  write removed: the audit test fails; both restored: green). Race suite for routerstore and dispatch green.
- End-to-end with the real CLI against a local service: a scoped token completed pull, show, acknowledge, respond, close, and was
  refused for send-as-`ra`, `pull ra`, and a full-ledger dump.

## Rollout (production) and rollback
The Postgres open check requires the schema version to match EXACTLY (`open_postgres.go`): a v25 database rejects the old
service on a cold start, and a v24 database rejects the new one. Procedure, in order:
1. `gcloud run services update sirsi-router --min-instances=1` so an already-open v24 instance stays warm.
2. Apply the schema job (adds the table, version 25). Running instances keep serving; the check runs at open only.
3. Deploy the new image immediately (`scripts/router-service/deploy.sh`, which also resets min-instances to 0).
4. Verify `router status` from a Mac, then mint the first scoped token.
Rollback: route traffic to the previous revision AND set the version back (`UPDATE router.schema_version SET version = 24`);
the extra table is harmless to the old binary. The reverse (only reverting traffic) fails the version check.

## Consumer path for a cloud lane
SIRSI_ROUTER_URL = the service HTTPS URL, SIRSI_ROUTER_TOKEN = the scoped token (cloud environment secret, never in a
repository or transcript), SIRSI_AGENT_ID = the lane. The CLI cross-builds statically for Linux (`CGO_ENABLED=0 go build
./cmd/sirsi`, verified linux/amd64 and linux/arm64). The registry pin (`sirsi router registry sync` from a clone of origin/main)
is client-side. A cloud lane cannot be woken by launchd: it needs a held `Wait` or a schedule (see M0_BASELINE.md, the wake tick).
