# Production runbook — Ra / Horus

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
