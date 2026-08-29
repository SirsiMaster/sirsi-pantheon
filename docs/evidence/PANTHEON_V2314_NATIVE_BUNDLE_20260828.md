# Pantheon v0.23.14 Native Bundle Evidence — 2026-08-28

## Source identity

- Package-source commit: `901b4703dfdcf09f9c67d84690203b9fa7f1d7b4`
- Package-source tree: `549c70adc6282ee7e90201d01a3de812a5f9def4`
- Scope: Swift is the sole `Pantheon.app` executable; Go remains the embedded
  authoritative CLI/control helper. No service, Tailscale, security, SNE, or
  installed application state was changed.

## Local ad-hoc assembly

```text
BUILD_NUMBER=202608281936 scripts/build-dmg.sh \
  --version 0.23.14-beta --arch arm64
```

The command exited `0`. It compiled the native Swift shell, the Go CLI, and
the Go control helper; assembled an ad-hoc-signed app; passed the package
identity verifier; and created a local DMG. It did not notarize, publish, or
install anything.

| Item | Exact path | SHA-256 |
| --- | --- | --- |
| DMG | `/private/tmp/pantheon-release-hardening-20260828/bin/SirsiPantheon-0.23.14-beta-arm64.dmg` | `b6a6264950370ca8d2911e7a94c9307dcaad4e7039f167eac5fe2598d64315da` |
| Native shell | `.../Pantheon-0.23.14-beta-202608281936-arm64.app/Contents/MacOS/SirsiMenubar` | `601fb400b398cab56459741f99628dd5a8db8748b20e13f2cc6826b7e9f5ba6f` |
| Embedded CLI | `.../Pantheon-0.23.14-beta-202608281936-arm64.app/Contents/MacOS/sirsi` | `f91de1f109fc9f9077e6f0842e4d6c90e92b1065805a855f1b6b949b00ab8e39` |
| Go control helper | `.../Pantheon-0.23.14-beta-202608281936-arm64.app/Contents/Library/Helpers/pantheon-engine` | `b4b6137d38afb9a24b71569c5b1cd7415a57f7f446fda955db7c66c4c9c277fd` |

## Verified bundle behavior

- `CFBundleExecutable` is exactly `SirsiMenubar`.
- `SirsiMenubar --help` exited `0` and printed only CLI usage; it did not open
  the menubar surface.
- The embedded CLI reported `v0.23.14-beta` from inside the bundle.
- `codesign --verify --deep --strict` exited `0`; inspection reported
  `Identifier=ai.sirsi.pantheon`, `Signature=adhoc`, and `TeamIdentifier=not set`.
- The bundled optional LaunchAgent has no `KeepAlive` key and targets the
  native `SirsiMenubar` executable. It was not installed or loaded.
- `swift test --package-path macapp` exited `0` with ten XCTest cases passing.
- `bash scripts/verify-menubar-release-contract.sh` and `git diff --check`
  exited `0` before the source commit.

## Release boundary

This is proof of a coherent unsigned local bundle, not a public release.
Developer ID signing, hardened-runtime notarization/stapling, exact published
asset/cask binding, clean install/upgrade/uninstall/rollback, and sustained
M1/M5 resource/crash evidence remain required.
