# Governed sleep custody

The shared transaction, canonical online authorization, native adapters and
`desktop-sleep` CLI are implemented in source. This is not a deployed authority,
a physical power result, or permission to reuse an expired hardware window.

The canonical service creates finite policy only through its concrete management
backend. Revision startup may materialize the nonsecret
`SIRSI_MANAGED_SLEEP_DELEGATION`; lane environments and RPC bodies cannot create
it. Exact reapplication preserves consumed/revoked history. The schema-25 state
row is protected against generic state/CAS writes. No second signing issuer,
recovery-key promotion, IAM enrollment or plaintext grant fallback exists.

The single participant RPC derives token/session/adopted-machine/thread/runtime
identity from server authentication. Policy binds the owner request, source and
executable, target platform/boot, nonce, generation, exact wake/window, and three
participants. Role names in a body are not authority. All three readiness records
must be fresh. Native consumption and reservation admission share one transaction;
existing/future active or queued work refuses sleep. The exact reviewed
controller reservation is the only workload exception; its actual confirmed
lease and immutable normal-supervisor execution budget must cover the lifecycle. A consumed unresolved
host refuses new work even after its original window expires or is revoked.

The native adapter uses fixed command vectors, never a shell or sudo fallback.
It schedules and reads one owned wake, then permits one invocation. Native
identity comes from hardware UUID, boot-session UUID and executable bytes.
Complete process/open-write inventory needs the existing privileged custody
service. Matching a reviewed inventory does not by itself prove active payloads
are idle: fresh real participant custody remains mandatory. Every missing
binding or unknown parser format refuses execution.

M1 independently probes the enrolled M5 SSH alias for same-boot baseline and
bounded loss/return. Authentication or host-key denial is not sleep. Native
return additionally checks actual Sleep/full-Wake records, unchanged native
identity and reviewed process/writer continuity. DarkWake, ping success and a
successful command exit alone do not qualify.

The local no-follow, private, fsynced journal retains nonces and host/boot fences.
Unknown scheduling or invocation is never retried. Explicit `reconcile` is
return-only: it requires prior persisted invocation and owned-wake readback plus
the same live/native/independent proof. It preserves the original unknown/error
records. Native-authorized unknowns and legacy consumed-but-not-invoked cases
remain fenced for management incident review; no successful sleep is invented.

Production accepts only the release-compiled canonical HTTPS origin through the
existing RemoteStore pin/proxy/session transport, with redirects refused. A
local store or arbitrary/spool issuer override cannot become power authority.
The current release origin is documented in ADR-119. Native service deployment,
actual privileges/identity, registered participants, source review, normal release
and a fresh physical window must qualify before use. No live power ran in tests.

Focused source qualification (all power commands are fakes):

```sh
go test -race ./internal/desktopcustody ./internal/routerstore -run 'Sleep|DesktopCustody|Native' -count=1
go test ./cmd/sirsi -run 'ManagedDesktopSleep|DesktopSleep' -count=1
go vet ./internal/desktopcustody ./internal/routerstore
```

Full repo/release gates and independent SSA review remain required. Historical
transaction source provenance is9f8e63356896bdd06ca434992bca088273e17342, with SHA's
Oct13/Oct15 contract/review artifacts retained. ADR-119 and the implementation
plan define the current source scope. Reading Room/Workspace mirrors are pending
review; do not describe repository-only source as fully published product canon.

The normal supervisor now publishes an execution fact for its actual child,
not power authority. The privileged native reader checks its real parent/start,
exact argv, private file and deadline; canonical management freezes its digest.
The same transaction checks real reservation/lease budget. Native v1 supports
up to180seconds to wake /210seconds lifecycle only when proven budgets permit.
The controller stays alive through return; actual post-wake registration refresh
is required, never a synthetic heartbeat or a stale-session waiver. Configuration
revocation is management-only; unresolved consumed state survives revocation and
rollback must not restore a fence-unaware writer. All actual power remains unarmed.

### Reviewed pre-native recovery

Consumption and native invocation authorization are separate canonical phases.
The controller persists invocation intent between them. A journal failure there
can terminalize only the server-versioned, consumed-but-not-native-authorized
record; nonce history is never reset. Explicit `abort-before-native` recovery
also handles a lost pre-native response after consulting the canonical phase.
Invoke/abort serialize with reservation admission. Once native authorization is
issued, unknown responses or native/journal failures preserve the fence and owned
wake, even if no physical sleep can be proved. Legacy records without the server
phase version cannot be compensated. Physical proof always requires actual
Sleep/full Wake and independent observation after native authorization.
