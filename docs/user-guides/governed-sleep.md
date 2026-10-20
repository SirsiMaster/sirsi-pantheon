# Governed sleep: operator workflow and qualification

The controller is implemented in source; it is not yet qualified for a live
sleep on this fleet. Existing owner intent does not require repeating, but an
expired quiet window, a transport credential or a test success cannot replace
fresh actual custody and reviewed production installation.

## Management preparation

SSA reviews the exact source and normal release. Existing service management
may configure one finite nonsecret `SIRSI_MANAGED_SLEEP_DELEGATION` on the
canonical router revision. It includes the exact owner-request reference/hash,
request/nonce, sleep-only admission, target platform/boot, reviewed inventory,
executable/candidate hashes, bounded wake/window/generation and exact enrolled
coordinator, custodian and independent M1 observer participants. The optional
controller admission exception binds one reservation ID, holder and work.

Creation/revocation are concrete-backend management operations, never lane RPCs.
Identical revision restart is safe: it cannot replace an existing request or
revive a consumed/revoked nonce. No new keys, IAM identities, root privileges,
firewall changes or guard relaxation are supplied by this feature.

## Supported commands

- `sirsi desktop-sleep inventory m5`: actual native identity and complete reviewed
  process/writer digest. Requires the existing privileged native custody context;
  it performs no power operation and does not create authority.
- `sirsi desktop-sleep status REQUEST --role custodian`: inspect the durable
  canonical lifecycle as the exact delegated participant. Coordinator/observer
  may use their own role; a role flag grants no access.
- `sirsi desktop-sleep preflight REQUEST`: verify native boot/executable,
  reviewed process/writer inventory and live canonical binding. No power.
- `sirsi desktop-sleep ready REQUEST --role coordinator`: the exact coordinator
  declares current readiness. This is not proof that cloud child jobs ran.
- `sirsi desktop-sleep observe REQUEST`: the exact M1 observer establishes the
  enrolled M5 boot baseline, refreshes readiness, observes within the finite
  window and records independent same-boot loss/return. It starts no M5 payload.
- `sirsi desktop-sleep dispatch REQUEST`: native custodian checks current
  inventory/assertions, journals once, schedules/reads the owned wake, requires
  fresh three-party custody and atomically consumes the finite authorization
  before the single native sleep callback. A successful native return still
  reports return-verification pending, not a physical success.
- `sirsi desktop-sleep return REQUEST`: require native Sleep/full-Wake logs,
  same boot/executable, process/writer continuity and matching independent
  observer evidence; record canonical return and clean only the owned wake.
- `sirsi desktop-sleep reconcile REQUEST`: resolve a retained unknown only with
  prior persisted invocation/readback and the same authenticated return proof.
  It never dispatches or retries sleep.

Use existing normal admissions and runtime/thread registration. Every foreign
active/queued workload, including future overlapping work and floor shares,
refuses sleep. An unresolved consumed host fence prevents new admissions; it is
not silently removed by timeout or revocation. Unknown-before-invocation requires
management incident review, not a substitute request ID.

## Deployment boundary

The release fixes the existing canonical HTTPS origin in
`main.desktopCustodyServiceOrigin`; callers cannot choose another issuer by a
flag or environment. The existing RemoteStore pin/proxy/session transport is
retained and redirects are refused. Missing direct enrolled service credentials,
spool-only configuration, incomplete privileged inventory visibility or an
unsupported native log format fails closed. These are qualification failures,
not a reason to mint credentials, elevate privileges or bypass the guard.

No source test sleeps a Mac, schedules a real wake, enrolls a key or changes
system settings. Source/release acceptance and an actual fresh native
Sleep/full-Wake/workload-continuity test remain distinct. Reboot, shutdown,
FileVault changes and unattended cold unlock are outside this workflow.

Publication: repository draft updated; owner Reading Room and native Workspace
mirrors remain pending independent review. No deployment or physical completion
claim follows from this document.
