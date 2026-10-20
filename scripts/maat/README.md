# Fabric reconciliation census

## Native release executor (Oct20)

The self-hosted release job is an execution placement, not implicitly M5.
Its build/sign/notary payload runs through the checked-in `admitted-run.py`
as a destination-native `codex-pantheon` executor, with a fresh maintained
canonical CPU grant and a 1,500-second bound. Go and Swift compilation are
explicitly limited to one job. A registration/admission refusal launches no
build; terminal release or executor-close failure prevents publication. The
existing rail-lock start hook is not admission. No signing credentials,
runner services, firewall settings or foreign payloads are changed by this
wrapper. Retain the supervisor's `~/.sirsi/evidence/maat-runs/*/terminal.json`
and qualify the actual GitHub signing run, not merely its runner's idle flag.

Source controls: `python3 scripts/maat/test_release_admission.py` and the
existing `test_admitted_run.py` lifecycle/refusal/cleanup tests. The contract
tests execute no signing, model, network or release payload.

## Registry snapshot continuity (Oct19)

The CLI compares thread payloads at save time using its own decoded schema, not raw
service JSON. Key ordering, normalized timestamps and newer unknown fields do
not make unrelated rows dirty. Registration, heartbeat and close leave those
rows byte-exact in the canonical store; actual target lifecycle and host fences
remain enforced. A closed unrelated row must not block a new executor. This is
a CLI correction, not a runner retry or a second registration. Qualify with
`TestRegistrationLeavesUnchangedServicePayloadsByteExact`,
`TestUnchangedThreadIsNotDirtyAcrossEncodings` and the existing
thread lifecycle/authority tests before signed client rollout. Do not change
the installed CI CLI hash until the exact signed successor is qualified.

## Job stop ownership (Oct19)

A compute grant is not permission to stop a process by pattern or PID. The
native runner rejects known direct/wrapped process-stop commands before
admission; its own launched-child teardown is unchanged. See
`docs/runbooks/maat-job-stop-ownership.md` for the adopted-service controller
protocol and the explicit same-UID/non-sandbox limitation.

For the already-installed three-part Claude hook chain, stage this runner as
`~/.claude/hooks/maat-job-stop-policy.py` and `job-stop-hook-chain.py` as the
existing `guard-pretool`, preserving original bytes first. This chain requires
the existing regular `guard-pretool.original-20261016` and
`maat-heavy-launch.py`; do not install it over an unrelated hook layout.
Validate staged hashes, make the files private executable, and replace the
chain only after the policy is in place. `check-installed-stop-hook.py` submits
inert tool JSON to that chain: it executes none of the supplied command text.
Test on each host under CPU-only admission. Do not replace an active recurring
job's hash-bound helper or restart any job to install the hook policy.

## Safe native job and observer launchers

### Scoped native worker transport

Install `fabric-run.sh` as `~/.local/bin/sirsi-fabric-run` (0700). This is an
operator shell helper using macOS OpenSSH/netcat, not a new model or router
store. Newly started declared Codex consumers select the `sirsi-router` named
profile: command networking is proxy-enforced and allows only the three
enrolled Tailscale IPs and two existing pinned LAN paths. Broad private-network
access, upstream proxy chaining and SOCKS UDP stay disabled. There is no public
wildcard or direct fallback. Managed administrator restrictions still apply;
this does not rewrite an already-running session's immutable instruction mask.

For an existing runtime that supplies the scoped local SOCKS proxy, use:

    ~/.local/bin/sirsi-fabric-run m1 --work my-own-work --cpu-cores 1 --timeout 90 -- /usr/bin/true

The helper takes only the enrolled m1/m5/studio target names, uses existing
strict host pins and public-key-only authentication, quotes every destination
argv element as data, and retains the caller's own logical lane. Origin SSH is
supervised as one CPU; the destination's actual local executor independently
registers/admit/heartbeats/releases with the requested capacity. No credentials
are copied into another runtime, and no arbitrary destination shell is exposed
as a new authority. This is cooperative same-user execution attribution, not a
new command-restricted SSH principal or protection against a malicious owner
account. Missing pins, keys, helper, proxy or supported managed policy fail
closed. Native destinations retain their own CLI/runtime/identity and the one
canonical router; no router database is copied.

`--plan` renders literal argv without launching anything; it is not execution
evidence. Actual qualification requires each lane's destination terminal and
cleanup readback. Do not count a configured/wakeable lane or source fixture as
an actual three-host pass. Roll out by updating the canonical pinned registry
and letting existing attended workers finish; never create a competing consumer
or disable their managed mask merely to claim success.

Classification: internal-tool. Users are Sirsi operators and registered execution lanes. The pain is orphan expensive work and repeated unsigned-observer prompts; the workflow is register → admit → supervise → prove cleanup. This reduces interruptions and operational support risk, not a separate priced product. SSA/Ma’at owns the launcher; Ra owns tool-hook integration; Claude Apollo M5 Rail approves Apollo enrollment and private telemetry. No credentials, permission-mode changes, global firewall changes, model downloads or benchmark claims are part of these tools.

Install `admitted-run.py` as `~/.local/bin/sirsi-maat-run` (0700). A caller supplies its **own** registered `SIRSI_AGENT_ID` and `SIRSI_THREAD_ID`, not another lane’s IDs:

```
~/.local/bin/sirsi-maat-run --work exact-work-label --timeout 300 -- /absolute/program argument
```

The actual local host is mapped to m1/m5/studio. The registered process anchor must be in the launcher's ancestry with unchanged kernel start time. A current reservation can be borrowed only with exact resource/holder/work, full share and fresh heartbeat; it is never released by a borrower. If the resource is free, the runner requests its own reservation, refuses any floor/denial/malformed/error reply, heartbeats that own reservation, and releases only its own ID. Child exit, timeout, lost admission and covered interruption lead to process-group termination/reap. Private terminals under `~/.sirsi/evidence/maat-runs/` hash argv rather than copying its content; release failures record debt. No model is loaded by a refusal test. Registration is observation, never automatic impersonation.

### Any fabric agent on any enrolled execution host

Owner policy (2026-10-18): a lane is not restricted to the Mac named in its id.
Use the existing authenticated, pinned fabric transport to the destination and
run its installed supervisor with `--agent <own-logical-lane> --register-executor`.
This creates a job-lifetime execution thread anchored to the actual destination
supervisor process, heartbeats it, and closes it after cleanup. It does not start
a new inbox consumer, change the logical agent, or move the canonical router.
Never pass the origin Mac's PID/thread as if it existed on the destination.
For example, from an already authenticated Studio shell:

```
~/.local/bin/sirsi-maat-run --agent claude-apollo-m5-rail --register-executor --work exact-work --timeout 300 -- /absolute/program argument
```

The destination's actual m1/m5/studio resource still requires an exact fresh
full-share reservation. Another holder/work, an expired or unknown reservation,
wrong runtime, malformed registration, or unavailable canonical router refuses.
No home-machine affinity is imposed. This authenticates the destination execution
instance; it is NOT a portable assertion that a foreign origin PID is alive.
Existing transport credentials and service-side lane scopes still apply. No new
SSH trust, credential, permission-mode or model privileges are granted.

On Darwin the supervisor reads parent PID and start time from the public
`proc_pidinfo(PROC_PIDTBSDINFO)` kernel interface, not an external `ps` command.
This supports managed workers whose sandbox denies spawning `/bin/ps` while
retaining local ancestry/start verification. A denied, short, disappeared or
PID-mismatched kernel observation refuses; there is no cached-identity fallback.
The compatibility start string remains Go `time.ANSIC` in host-local time.

Use the installed CLI's `thread heartbeat --thread ID --json` as the exact
canonical row observation, avoiding the full-list OS reaper. This is an ordinary
own-session heartbeat, not a read-only fleet sweep: terminal/suspended records
cannot be revived. Destination executors recheck their retained handshake's local
kernel identity before that heartbeat and validate its returned row again after.
Every path checks exact owner/thread/status and actual kernel ancestry/start
before admission and immediately before child spawn. No foreign PID is trusted.
The selected observation is uncached and bounded to30seconds. Other control calls have
a60second bound because the deployed reservation command can publish its durable
grant before finishing its decision projection. Exact fresh admission and expiry
checks still run after every reply; no stale positive cache or lease grace is used.
The former whole-fleet registry read took18.29seconds for151 rows on M5, with a
separate SHA observation at45.289seconds, against the old eight-second deadline.

These are cooperative operational controls, not an OS security sandbox against same-user tampering, deliberately hidden workloads, escaped descendant sessions or supervisor SIGKILL. A native tool hook and per-runtime adoption must be proven separately. The execution timeout begins at child spawn; metadata and cleanup calls have their separate bounds. Do not mistake a receipt file's existence for live admission.

### Long runs, transport uncertainty and recurring jobs

`--timeout` is a total **per-child execution budget**, not a heartbeat TTL.
The 900- and 1700-second Studio preparation attempts stopped at those configured
limits despite continuing heartbeat renewal. Set an appropriate explicit budget
(at most 7200 seconds) for a long transfer. For large transfers use an owned
partial file, resumable transport and complete hash readback before publication;
a timeout is not permission to overwrite a destination or discard its partial.
The abandoned optional Studio training transaction must not be restarted.

New owned grants explicitly request a 240-second TTL. Only the server-confirmed
grant establishes authority; a shorter or different returned grant is not
silently extended. The supervisor retries transient control failures within the
remaining proven grant, with exact readback before renewal. A recovered read
alone never extends its deadline. Explicit denial, revocation, scope change or
expiry still stops immediately. Initial registration observations retry for at
most 120 seconds without starting compute; an uncertain registration is resolved
using the same generated thread ID, not a second registration.

Terminal evidence distinguishes `execution_timeout`, `proven_lease_expired`
and a recurring `job_timeout`; it includes the configured execution budget,
elapsed time and remaining proven grant. Reservation decisions preserve the
actual conflict's ID, holder, resource and expiry inputs plus pending cede IDs.
An uncertain release is resolved by exact ID and retained ownership fields.
Only a read proving that same owned grant remains active permits a release
retry; unavailable or changed readback remains explicit cleanup debt. This does
not authorize releasing other jobs or guessing the ID of an uncertain create.

`--register-executor` always requests its own canonical reservation, even when
`who-is-on` selects a foreign or old same-lane/work row. That observation is not
an admission decision for a new executor. The scheduler still determines the
actual permitted share; a returned CPU floor is never promoted to model/GPU
permission. A new executor does not borrow another job's unrenewable grant.

One foreground supervisor can own a finite recurring job without depending on
an IDE process remaining in its ancestry:

```
~/.local/bin/sirsi-maat-run --agent OWN-LANE --register-executor \
  --work OWN-WORK --iterations 18 --interval 60 --job-timeout 28800 \
  --timeout 1500 -- /absolute/program argument
```

Use the actual enrolled host's existing authenticated service environment.
A supported service manager may own this foreground process; do not use a
foreign desktop thread or wrap detached payloads in shell loops. Each iteration
obtains a new admission, records its result durably and releases its own grant.
Waiting retains no compute grant and only maintains executor liveness. The first
failed iteration or explicit control denial stops the entire job. Maximums are
1000 iterations, 3600 seconds between iterations and 86400 seconds total. There
is no automatic payload retry, indefinite lease, second inbox consumer, model
grant or silent background service installation. Actual service-manager and
lane-owned qualification must be recorded separately from source tests.

Install `observer-launch.sh` as `~/.local/bin/sirsi-apollo-observer-launch` (0700). Existing observer LaunchAgents prepend `/bin/sh` and this launcher to their unchanged executable/arguments. Only the enrolled executable locations, strict Sirsi Team9D382WV988/identifier requirement and lead-approved CDHash are executable; unsigned or different signed replacements refuse **before** opening a listener. `--verify-only <file>` checks staged/rollback files without executing them. The deployment preserves original binaries/plists. Rollback of the launcher means restoring its original plist while retaining the verified signed binary; never silently restore an unsigned listening service. Approval for the three unchanged private Tailscale endpoints is a separate durable lead decision, not a global firewall exception.

Verification: `python3 -B -m unittest discover -s scripts/maat -p 'test_*.py' -v`, `sh -n scripts/maat/observer-launch.sh`, actual signed/unsigned verify-only controls, actual registration/refusal/success/timeout controls, per-host binary and launcher hashes, strict signature checks, private-listener and peer health readback. Report native hook adoption gaps explicitly; source tests alone do not establish full fleet enforcement.

`python3 -B scripts/maat/fabric-reconcile.py > census.json` inspects the current host's pinned lane registry, conventional project roots, and every worktree linked by their Git common directories. Run it separately on each host. It makes no Git changes and never stages, commits, resets, cleans, prunes or deletes work.

`python3 -B -m unittest discover -s scripts/maat -p test_fabric_reconcile.py -v` verifies dirty/clean classification, unusual status names, empty-inventory refusal, and preservation of the real Git index.

Counts are **Git status entries**, not requirements, bytes, or logically distinct files: renames are deliberately represented without expensive rename detection. A clean worktree may still be detached or ahead of its upstream. Missing temporary worktree paths do not prove lost changes; reconcile their retained refs/objects before any administrative cleanup. Repeated worktrees sharing one common directory are enumerated once.

Reports contain private local path metadata and must remain in the operator's private evidence home, not public issues. They never read source contents or credentials. Ignored caches are not source work; private non-project home untracked files are deliberately excluded. A missing registry, unreadable repository, changing HEAD, unfinished Git operation or missing linked worktree prevents a global clean claim. Cloud helpers and other hosts require separate receipts.

Disposition order: observe → identify the real writer and ledger → checkpoint a coherent source unit → validate and scan → commit/publish through normal authority → repeat the census. Never manufacture a clean result by deleting work, hiding a stash, mixing unrelated changes, or closing the parent implementation request after a partial handoff.
# CPU build floor and exact grant lookup (Oct18)

`--cpu-cores N` explicitly declares a cooperative **CPU-only build** mask. It
may consume an active `regime=build` floor only when the canonical granted core
count is at least N. The child receives matching Go/OpenMP/BLAS thread limits.
This is not an OS sandbox or GPU/model permission; model launches retain the
default full-share requirement. Unknown, foreign, insufficient, stale, or
revoked grants still refuse. Exact newly acquired grants are observed through
their own heartbeat response, not the first holder on a shared machine.


### Bounded heartbeat transport recovery

The supervisor reads the exact existing ID through `maat status <resource>`;
reads never refresh a lease. Only a confirmed reserve/heartbeat response advances
a monotonic deadline anchored before that request, discounted by the returned
heartbeat age and bounded by the actual accepted TTL. Each renewal requires
fresh matching active ID/resource/holder/work/executor/share/capacity readback
and unchanged kernel executor ancestry. A failed mutation is never blindly
replayed and never triggers reserve, a new ID, or a share promotion.

Healthy remote exact-ID read/renewal runs at most once per30 seconds, shortened
for remaining accepted TTL. Local child completion is checked at most every two
seconds without inventory RPCs. Explicit revocation is observed on the next
healthy remote check (up to30 seconds plus bounded RPC latency, always before
the proven deadline); terminal acceptance requires its own exact readback.
After transport uncertainty, remote readback retries on1/2/4/8-second backoff.

Recognized HTTP 502/503/504, TLS handshake timeout, relay outcome-unknown and
bounded RPC timeouts may retry with 1/2/4/8-second backoff only inside the proven
interval. RPC timeouts and child waits are clamped to the earlier execution/lease
deadline. Reads do not extend that interval even if an uncertain heartbeat may
have committed. Explicit denial, revocation, missing/expired/mismatched state,
malformed JSON and unclassified errors stop immediately. A borrowed reservation
is neither renewed nor released, and cannot extend the initial proven interval.
A terminal child receives no subsequent renewal. Cleanup terminates/reaps only
the launched child group and releases only the acquired reservation; unresolved
release or executor-close failures remain explicit cleanup debt.

Reproduce the deterministic clock/transport fixtures with:
`python3 -m unittest discover -s scripts/maat -p test_admitted_run.py -v`.
Tests mock transport/time for outages and also exercise a harmless real child
termination/reap and Darwin kernel identity. Run them under ordinary own Ma'at
admission. They do not qualify deployed long-running Apollo workloads; SSA owns
independent review, release and installed qualification.
