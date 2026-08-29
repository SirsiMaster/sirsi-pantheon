# Pantheon v0.23.14 Packaged MCP Handshake — 2026-08-28

## Source identity

- Source commit: `952f3d3459ccf8c777fa34047e29ccb67fe51a1e`
- Source tree: `f787c41837658e7af46b631054252e4a6ed6b7d1`
- App candidate: `bin/Pantheon-0.23.14-beta-202608281942-arm64.app`
- DMG: `bin/SirsiPantheon-0.23.14-beta-arm64.dmg`

## Regression repaired

The first bundled MCP probe exposed stale inherited metadata: the server had
the right `sirsi-pantheon` name but reported `0.4.0-alpha` and Anubis
instructions. The repair makes the MCP default identity use the same
build-stamped `internal/version.Version` as the CLI and removes the retired
Anubis copy from the default server/logging path.

## Exact packaged probe

One JSON-RPC `initialize` request was sent to the embedded CLI's `mcp`
subcommand over stdin, then stdin was closed. The command exited `0` and
returned:

```json
{
  "serverInfo": {
    "name": "sirsi-pantheon",
    "version": "v0.23.14-beta"
  },
  "protocolVersion": "2025-03-26"
}
```

The stderr log prefix was `[pantheon-mcp]`; the returned instructions identify
Sirsi Pantheon and state that operations are local and require explicit owner
action for data to leave the machine. The server exited on EOF. It did not
create a listener, start a daemon, launch a model, or mutate host state.

## Verification

- `go test ./internal/mcp ./cmd/sirsi` exited `0`.
- The fresh local ad-hoc assembly exited `0` and its package-identity verifier
  accepted bundle `ai.sirsi.pantheon` at `0.23.14-beta` build `202608281942`.

## Boundary

The assembled bytes are ad-hoc signed and local only. This handshake is
package-surface evidence, not Developer ID/notarization, publication, cask,
installation, or sustained-host qualification.
