# Restricted Codex relay access

The M5 system spool's real path is `/private/var/sirsipantheon/relay`.
`~/.sirsi/relay` is a symlink to it. Granting the home directory alone does not
grant the external target. An exclusive slot open denied by the sandbox is not
evidence that 64 requests exist or that the cloud relay is stalled.

Use a named permission profile that extends `:workspace`, grants write access
only to the canonical real relay directory, and keeps direct networking disabled.
The filesystem ACL/group membership still applies. No router token is given to
the lane. Do not use danger-full-access or recreate a local router ledger.

Qualification against the installed Codex launcher:

```sh
codex sandbox -P sirsi-relay -C "$PWD" \
  -c 'permissions.sirsi-relay.extends=":workspace"' \
  -c 'permissions.sirsi-relay.filesystem={"/private/var/sirsipantheon/relay"="write"}' \
  -c 'permissions.sirsi-relay.network.enabled=false' \
  -- env SIRSI_ROUTER_URL=spool:///private/var/sirsipantheon/relay \
     SIRSI_AGENT_ID=sirsi-software-admin \
     /usr/local/libexec/sirsi-pantheon/sirsi router pull sirsi-software-admin
```

Launch configuration must select this profile before creating the agent sandbox.
This cannot modify the permissions of a sandbox that is already running.
For another host, discover and bind its actual configured spool; do not copy M5's
path blindly. Keep archived requests and all uncertain outcomes for reconciliation.

Qualification performed 2026-10-12: an ordinary restricted call was denied at
slot open with EPERM; a fresh named-profile sandbox with the precise relay grant
returned the cloud inbox successfully, with network disabled.

Fleet rollout, every historical migration mapping, and real M1/M5 outage-recovery
evidence remain separate until recorded. A passing pull alone is not that proof.
