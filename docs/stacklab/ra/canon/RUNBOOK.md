# Production runbook — Ra / Horus

## Selected web gateway update

The exact user LaunchAgent `ai.sirsi.router.surface` is distinct from Horus and
the protected root relay. After installing verified signed product bytes, its
native owner may obtain a read-only plan with
`sirsi launchd restart ai.sirsi.router.surface --json`. Only SSA/SHA have the
explicit lifecycle grant. A plan is not a restart or proof of current loaded
bytes: retain its PID, start identity, plist and executable SHA256, then observe
the selected gateway's real duty boundary and recovery path before apply.

Apply requires `--apply --drained` and all four exact expected pins. The
controller rechecks those under its service lock and refuses changed instances;
post-readback requires the same selected artifacts and a new actual instance.
Never claim drained from a clock window, use raw launchctl as a substitute,
restart other lanes, enable disabled jobs, or relax the root relay's queue guards.
Verify actual loaded image and one authenticated harmless board read/action
afterward. Failure retains the selected service's explicit recovery obligation.

1. **Authority check:** verify `SIRSI_ROUTER_URL` or the explicit disposable
   `SIRSI_ROUTER_DB`; run `sirsi router doctor`. A regular production
   `~/.sirsi/router.db` is a defect, not a fallback.
2. **Identity check:** verify agent, thread, host and runtime identity; register
   the current thread before mutation. Do not reuse another session's marker.
3. **Transport check:** for Codex, verify the relay path, trust group and
   token-free lane environment. For direct clients, verify the pinned service
   endpoint and session nonce path.
4. **Work:** claim one bounded item, renew the lease, acknowledge after reading,
   write evidence to an isolated root, and close with a validated proof.
5. **Failure:** let the fenced lease expire or hand back explicitly. Never
   retry a mutating outcome-unknown blindly; re-read the item and decide.
6. **Release:** preserve source/binary/image hashes, revision, traffic and
   rollback target. If a cloud or host fact cannot be read, record OPEN.

Never copy a router database, silently overwrite a peer, or treat a heartbeat,
PID, or successful process spawn as proof that the lane consumed its work.

## Executable upgrades and controller ownership

Compare each loaded executable with the signed installed release, not just its
plist path. Older generations need one guarded service rollout because they do
not acquire new observer behavior from an on-disk replacement. Preserve each
active detached consumer's native PID/start, process group and durable marker.
Never stop payloads merely to make the installed-version report green.

The successor controller reads its **loaded** mapping's kernel device/inode:
macOS uses libproc for its own executable code mapping (native cgo product build),
Linux uses `/proc/self/exe`. It compares that identity to the stable direct
executable argv installed in its service. This catches a replacement during
initial registration too; a late pathname snapshot is not a loaded identity.
Missing, symlinked or non-executable replacement names retain the current image.
After replacement, keep the observer heartbeat alive but dispatch no new work
until the active consumer finishes, then yield to the existing KeepAlive restart.

Observer rows bind their actual local machine/PID/start and surface; a different
generation mints a new ID. Read back that authoritative binding before dispatch.
A transient registry failure holds new dispatch without stopping detached work;
a terminal row is never revived. Deliberate stop is the exact service lifecycle
operation, not child cleanup pretending to own its parent's thread. The shared
Go close guard prevents that accidental close; independent child identity and
raw-service authorization are separate, unclaimed work.

After rollout, prove two sequential canonical requests, recipient ack/results,
the persisted observer generation and heartbeat between/after both, plus current
loaded executable and cleanup. Registration, a one-shot answer or a source-test
pass alone is not durable queue qualification.

## Restricted native workers

New Codex consumers use the named `sirsi-router` profile and command proxy with
only the three enrolled Tailscale IPs plus the existing M1/Studio LAN paths.
Router requests still use the physical relay without a bearer in the lane.
Install `scripts/maat/fabric-run.sh` as `~/.local/bin/sirsi-fabric-run` (0700);
then use `sirsi-fabric-run m1|m5|studio --work <own-work> --cpu-cores 1 --timeout 90 -- /usr/bin/true`
for a harmless proof. Local commands retain the installed `sirsi-maat-run`.

The helper requires the runtime's loopback SOCKS proxy and preserves existing
strict SSH pins/public-key authentication. It supervises the actual origin and
destination separately and quotes all remote argv. Do not change an active
worker's immutable managed mask, copy keys, bypass managed requirements or
silently restart attended sessions. Update the pinned canonical registry and
let the existing invocation finish before its ordinary next wake. A native
lane without an enrolled route/pin remains unverified, not a destination outage.
Record actual per-lane terminal/exit/release readback; a configured profile or
finite source inspection is not a three-host PASS. See `scripts/maat/README.md`
for the bounded transport contract and same-user attribution limitations.
