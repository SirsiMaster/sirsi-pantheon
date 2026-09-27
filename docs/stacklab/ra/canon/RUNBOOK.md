# Production runbook — Ra / Horus

Verify the canonical store, registry identity, host transport, and protected baseline. Claim one bounded item, preserve the lease, write evidence to an isolated root, and close with the completion proof. On timeout or failure, let the fenced lease expire or hand back explicitly; never copy the router database or silently overwrite a peer's state.
