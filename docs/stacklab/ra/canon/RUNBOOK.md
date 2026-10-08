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
