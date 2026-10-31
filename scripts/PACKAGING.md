# Sirsi Pantheon — Packaging Guide

Development and commercial outputs are deliberately distinct: an ad-hoc local
artifact must never occupy a commercial-release name or be uploaded as one.

## macOS development artifacts

Build a local DMG containing `Pantheon.app`, the native menu bar app, and the
canonical `sirsi` CLI:

```bash
make dmg-dev
```

Output: `bin/SirsiPantheon-VERSION-dev-ARCH.dmg`

Build an installer from that exact application bundle:

```bash
make pkg-dev
# Or directly, selecting once for both producer and consumer:
receipt="$(bash scripts/new-apollo-digest-receipt.sh "$PWD/bin")"
PANTHEON_APOLLO_DIGEST_RECEIPT="$receipt" scripts/build-dmg.sh --development --version 0.24.14 --arch arm64
PANTHEON_APOLLO_DIGEST_RECEIPT="$receipt" scripts/build-pkg.sh --development --version 0.24.14 --arch arm64 --app Pantheon.app
```

Output: `bin/SirsiPantheon-VERSION-dev-ARCH.pkg`

Development artifacts are ad-hoc/unsigned local package work. They are not
commercial releases or release candidates. The native profile remains optional
for development: supplying `PANTHEON_APOLLO_PROFILE` opts that local bundle into
the enabled capability; omitting it records disabled in the app Info.plist.
Each supported Make target selects a fresh private digest-receipt path. The PKG
builder requires that exact existing pre-sign receipt and does not recapture
the built app. Direct paired invocations must pass one path from
`scripts/new-apollo-digest-receipt.sh` to both commands; old and incomplete
receipts are retained. Both builders validate the complete two-key receipt
through a checked parser result before continuing; parser failures and malformed
or incomplete digest pairs stop the build under the supported macOS Bash 3.2.

## macOS commercial release artifacts

```bash
PANTHEON_APOLLO_CAPABILITY=disabled make release-dmg
PANTHEON_APOLLO_CAPABILITY=disabled make release-pkg
```

Only these targets produce `bin/SirsiPantheon-VERSION-ARCH.{dmg,pkg}`. They
require macOS, the Go toolchain, a Developer ID Application identity, a
Developer ID Installer identity for PKG, and complete Apple notarization
credentials. Both outputs are signed, notarized, stapled, and validated.
The release workflow and Make targets explicitly declare `disabled`, matching
published v147, whose app contains no native Apollo profile. To intentionally
produce a native-enabled release, declare `PANTHEON_APOLLO_CAPABILITY=enabled`
and provide the protected absolute regular profile path in
`PANTHEON_APOLLO_PROFILE`. Direct release-script calls require the capability
variable and one fresh shared receipt selected by
`scripts/new-apollo-digest-receipt.sh`; disabled releases reject a supplied
profile. The Go inventory checks
the independent expected mode and exact profile before signing, after app
signing, and against the expanded PKG. Release-mode signed-payload checks also
require the Info.plist/profile digest pair captured by the initial pre-sign
inventory. Developer signed-app checks may validate signature structure without
the release-only frozen-digest flag.

## Supported installation and upgrades

The commercial release publishes one macOS Apple-silicon `Pantheon.app`
payload. Homebrew installs that same application, links the bundled `sirsi`
CLI, and owns upgrades and removal:

```bash
brew install --cask sirsimaster/tools/sirsi-pantheon
brew upgrade --cask sirsimaster/tools/sirsi-pantheon
brew uninstall --cask sirsimaster/tools/sirsi-pantheon
```

The bootstrap script follows this exact Cask route. It does not download a
separate CLI archive, produce Linux packages, or create a Windows installer.
Those platform products require separate release contracts and are not part of
the Pantheon commercial route.

## iOS (xcframework)

Builds the PantheonCore Go mobile framework for the SwiftUI app.

```bash
make ios-framework
```

Output: `bin/ios/PantheonCore.xcframework`

Requirements: gomobile (`go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`), Xcode.

## Android (AAR)

Builds the Go mobile AAR for the Android app.

```bash
make android-aar
```

Output: `bin/android/pantheon.aar`

Requirements: gomobile, Android SDK/NDK.
