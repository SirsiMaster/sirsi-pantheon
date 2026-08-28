# Pantheon v0.23.14 Native Package Evidence

**Status:** local ad-hoc package inspection; not a distributable release

## Source identity

- Source commit: `7b103308120351066dd7ec2adc371514541bee6f`
- Source tree: `42504036f77d7ea5b9790affb23415df38d44cbc`
- Build command:

  ```text
  BUILD_NUMBER=202608281955 scripts/build-dmg.sh --version 0.23.14-beta --arch arm64
  ```

- Build exit: `0`

## Generated, uninstalled artifacts

| Artifact | SHA-256 |
| --- | --- |
| `SirsiPantheon-0.23.14-beta-arm64.dmg` | `16b1b92c31bf0a5fe24e491ea7494c60f14e4694a15c887f5fe22b8fd7a24cf3` |
| `Contents/MacOS/SirsiMenubar` | `097486581316874aae98b3aa308989d30b4b5455115a2a5b07c909e425e314a0` |
| `Contents/MacOS/sirsi` | `ebc39b54b5d109a4fdeed789da4d34d6d56afe586ee9caae37084c4bbd10d011` |
| `Contents/Library/Helpers/pantheon-engine` | `a2051d34a5d984f5bccc1fe84bdcc92f2e49f6be3b57f4792e091ca09e2ef689` |

The app bundle reports `CFBundleExecutable=SirsiMenubar`,
`CFBundleIdentifier=ai.sirsi.pantheon`, version `0.23.14-beta`, and build
`202608281955`. Its layout contains the native shell, the scriptable CLI, and
the separate embedded loopback-control helper.

## Signing boundary

Static `codesign` inspection reports `Signature=adhoc` and
`TeamIdentifier=not set`. This proves only a local development package. It
does not satisfy Developer ID signing, notarization, stapling, Gatekeeper,
GitHub asset, cask, installation, rollback/uninstall, or sustained-host gates.

## Containment

The DMG was neither mounted nor installed. No binary was launched. No service,
launch agent, SNE workload, Tailscale setting, security state, or credential was
changed.
