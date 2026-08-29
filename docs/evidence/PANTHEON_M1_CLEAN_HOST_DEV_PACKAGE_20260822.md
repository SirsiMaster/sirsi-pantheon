# Pantheon M1 Clean-Host Development Package Evidence

Date: 2026-08-22

## Scope

This record proves the non-GA development package lifecycle on the authorized M1 target. It does not claim Developer ID signing, notarization, or public GA readiness.

## Source identity

- Repository commit before this packaging repair: `e554b07536e4b56618793aeeb984d976d3b0a6c1`
- Package version: `0.24.0-dev.20260822.5`
- Architecture: `arm64`
- Bundle identifier: `ai.sirsi.pantheon`
- Signing mode: ad hoc, development-only
- Package: `bin/SirsiPantheon-0.24.0-dev.20260822.5-arm64.dmg`
- DMG SHA-256: `2790750e2008ea5e0f3d45ad43953d104713f3b3e731b638433724ae18ce34b8`

## Immutable executable identity

- `sirsi` SHA-256: `cf553d47e4bc4e8c5a9e65907ed368e44eac2c39c65e7e8e5d03e65904a9f498`
- `sirsi-menubar` SHA-256: `7ce1e05ad3ccf0932545bd9f1f398b9b009959a9b5949a29bed551e548d8d5d0`
- The hashes matched on the mounted M5 image and the installed M1 app.

## M1 lifecycle result

- Transport: Tailscale SSH succeeded even when idle-state `tailscale ping` did not establish a direct connection. Reachability is based on the successful SSH probe, not the idle `Active` flag.
- Mounted-image strict code-sign verification: passed.
- Installed-app strict code-sign verification: passed.
- Upgrade from `0.24.0-dev.20260822.1` to `0.24.0-dev.20260822.5`: passed.
- Launch after replacement: passed.
- Running process: `~/Applications/Pantheon Dev.app/Contents/MacOS/sirsi-menubar`.
- Menu process RSS after launch: 69,776 KiB (about 68 MiB).
- Explicit JSON diagnostics: completed; sampled finding severity was `0`.
- Optional GUI caretaker installation: passed; generated `ai.sirsi.pantheon.plist` with `RunAtLoad=true`, `KeepAlive=true`, and the exact installed app executable.
- Supervised recovery: passed after launchd's 10-second minimum-runtime window; PID `94830` was deliberately terminated and replaced by PID `94909` within the 15-second gate.
- Recovered menu process RSS: 67,696 KiB.

## Defect and repair

Sandboxed `hdiutil create` reported `Device not configured`, while the same operation with DiskImages device access succeeded. The package builder now:

1. supplies a deterministic non-empty volume label;
2. invokes DiskImages with a minimal environment; and
3. preserves the exact staged payload if DMG creation fails.

An HFS hybrid fallback was investigated and rejected because mounted images synthesized resource-fork/Finder metadata inside signed executables, causing strict signature verification to fail. No such package is admitted.

## Remaining release gate

The Login keychain has Apple Development and Apple Distribution identities but no `Developer ID Application` private key. Public GA distribution remains blocked on that external credential. This development package is valid for controlled M1 testing only and must not be represented as notarized or GA.
