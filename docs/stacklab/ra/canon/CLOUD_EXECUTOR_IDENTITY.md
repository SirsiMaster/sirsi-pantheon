# Logical cloud executor identity

An enrolled cloud executor uses its existing `SIRSI_ROUTER_CLOUD_HOST` value
for both authentication and thread-record ownership. Ephemeral container
hostnames are not stable enrollment identities. This does not claim a physical
machine identity: the router still authenticates the exact configured host token
and enforces its existing lane scope. It does not mint a token or grant access.

Names must match `cloud-[a-z0-9][a-z0-9-]{0,120}`. Simultaneously requesting
physical-machine-ID authentication is rejected before any HTTP operation.
Without the cloud declaration, existing native hostname behavior is unchanged.

Registration, heartbeat/save, task claim/completion and close must retain the
same logical owner. A different supplied cloud thread host is refused. Foreign
host tokens and foreign lanes remain refused by the server. The native
registration/claim/close regression and no-network invalid-identity controls
exercise this contract. The exact uncorrected120 baseline failed with the same
host-binding403 reproduced in both existing private cloud environments.

Rollout: qualify the corrected client against existing scoped credentials before
advancing startup. Keep the previous working client until successful actual
registration, claim and close readback. No token reissue, OS hostname change,
expanded domain policy, account-security change or local database fallback is
required. A source or fixture PASS does not prove scheduled execution or M5-off
continuity; those require independent live runtime and interval evidence.
