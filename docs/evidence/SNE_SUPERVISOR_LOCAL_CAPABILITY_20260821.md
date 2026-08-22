# Pantheon-Supervised SNE Local Capability

Date: 2026-08-21

Pantheon's SNE supervisor now carries the existing durable private local capability across the process boundary without exposing token text in arguments or environment variables.

- `LaunchConfig.AccessTokenFile` is mandatory for supervised launch.
- The file must be regular, private, and contain a bounded capability.
- Pantheon reads the capability for its internal SNE client.
- Pantheon passes only `--access-token-file <path>` to `sned`.
- The client adds the bearer capability to SNE `/v1/*` calls, including readiness identity, model discovery, completion, and lifecycle operations.
- Health readiness remains loopback-only and does not disclose the capability.

Focused proof:

```text
go test ./internal/sne ./cmd/sirsi-sne-supervisor
ok github.com/SirsiMaster/sirsi-pantheon/internal/sne
```

This closes the source seam only. Copied SNE/Pantheon packages still require a live authorized/unauthorized integration gate, exact identity admission, and clean-host qualification. Existing immutable packages remain unchanged.
