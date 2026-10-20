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
coordinator, custodian and independent M1 observer participants. The controller binding includes its exact real reservation, holder/work and
private execution fact/digest, including actual supervisor and bounded lifetime.

Creation/revocation are concrete-backend management operations, never lane RPCs.
Identical revision restart is safe: it cannot replace an existing request or
revive a consumed/revoked nonce. No new keys, IAM identities, root privileges,
firewall changes or guard relaxation are supplied by this feature.

## Supported commands

- `sirsi desktop-sleep inventory m5`: actual native identity and complete reviewed
  process/writer digest. Requires the existing privileged native custody context;
  it performs no power operation and does not create authority.
- `sirsi desktop-sleep status REQUEST --role custodian`: inspect the durable
  canonical lifecycle as the exact delegated participant. The custodian also sees retained local journal events. Coordinator/observer
  may use their own role; a role flag grants no access.
- `sirsi desktop-sleep preflight REQUEST`: verify native boot/executable,
  reviewed process/writer inventory and live canonical binding. No power.
- `sirsi desktop-sleep ready REQUEST --role coordinator`: the exact coordinator
  declares current readiness. This is not proof that cloud child jobs ran.
- `sirsi desktop-sleep coordinate REQUEST`: the existing admitted coordinator
  refreshes its actual readiness while waiting in the finite window; it does not
  claim that cloud child jobs have executed.
- `sirsi desktop-sleep observe REQUEST`: the exact M1 observer establishes the
  enrolled M5 boot baseline, refreshes readiness, observes within the finite
  window and records independent same-boot loss/return. It starts no M5 payload.
- `sirsi desktop-sleep dispatch REQUEST`: native custodian checks current
  inventory/assertions, journals once, schedules/reads the owned wake, requires
  fresh three-party custody and atomically consumes the finite authorization
  then persists invocation intent and separately authorizes native invocation before the single native sleep callback. The same controller remains alive
  until independently verified return; native command success alone is pending.
  Invoked uncertainty can only resolve with conclusive return proof, never a
  repeated invocation.
- `sirsi desktop-sleep return REQUEST`: require native Sleep/full-Wake logs,
  same boot/executable, process/writer continuity and matching independent
  observer evidence; record canonical return and clean only the owned wake.
- `sirsi desktop-sleep reconcile REQUEST`: resolve a retained unknown only with
  prior persisted invocation/readback and the same authenticated return proof.
  It never dispatches or retries sleep.

- `sirsi desktop-sleep abort-before-native REQUEST`: explicit recovery for the
  exact consumed custodian. The server serializes this against native invocation
  on the same transaction/fence. Only a server-created versioned pre-native record
  with no native authorization can be terminalized, even after expiry/revocation.
  The command cleans only its owned wake and records local abort; repeated abort
  readback is idempotent. The nonce stays spent. Native-authorized and legacy
  unknown records cannot be cleared this way. A lost invocation response never
  triggers automatic abort, sleep or invocation retry.

Use existing normal admissions and runtime/thread registration. Every foreign
active/queued workload, including future overlapping work and floor shares,
refuses sleep. An unresolved consumed host fence prevents new admissions; it is
not silently removed by timeout or revocation. An explicit pre-native recovery can release a consumed fence only when the
canonical record proves native authorization was never issued. A missing field
in a legacy record is not that proof. Authorized or uncertain native invocation
requires incident review/full return proof, never a substitute request ID.

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

## Planned-sleep budget

Run the native controller directly as the normal admitted supervisor's actual
child in the existing privileged custody context. The supervisor emits a private
execution fact; it does not grant sleep. `dispatch` can publish public identity,
inventory and execution-binding facts and wait within its real job budget for
canonical management to provision the exact delegation. It never self-provisions.

Native v1 allows at most180seconds to the scheduled wake and210seconds total
lifecycle, and only when the actual confirmed controller lease and execution
budget cover that time plus margin. The existing normal supervisor uses240second
leases; no assumed lease, borrowed PID, fabricated heartbeat or global freshness
waiver is accepted. After wake, the actual supervisor must genuinely refresh
before stale custody resumes. Preserve the original controller context during
return/reconciliation; lost controller identity, missed budget or missing native
proof requires incident review, not a substitute request or a success claim.

SSA must schedule both actual existing warm cloud actors so their work starts
and finishes inside the measured physical off interval. Source tests and a planned
180second interval do not prove that overlap. Do not sleep while any actual
foreign release, signing, model or payload is active. Review and normal release
must precede all real policy creation, ARM or native power.
