# Sirsi Pantheon — Packaging Guide

Development and commercial outputs are deliberately distinct: an ad-hoc local
artifact must never occupy a commercial-release name or be uploaded as one.

## macOS development artifacts

Build a local DMG containing `Pantheon.app`, the native menu bar app, and the
canonical `sirsi` CLI:

```bash
make dmg-dev
# Or directly:
scripts/build-dmg.sh --development --version 0.24.14 --arch arm64
```

Output: `bin/SirsiPantheon-VERSION-dev-ARCH.dmg`

Build an installer from that exact application bundle:

```bash
make pkg-dev
# Or after building the DMG/app bundle:
scripts/build-pkg.sh --development --version 0.24.14 --arch arm64 --app Pantheon.app
```

Output: `bin/SirsiPantheon-VERSION-dev-ARCH.pkg`

Development artifacts are ad-hoc/unsigned local package work. They are not
commercial releases or release candidates.

## macOS commercial release artifacts

```bash
make release-dmg
make release-pkg
```

Only these targets produce `bin/SirsiPantheon-VERSION-ARCH.{dmg,pkg}`. They
require macOS, the Go toolchain, a Developer ID Application identity, a
Developer ID Installer identity for PKG, and complete Apple notarization
credentials. Both outputs are signed, notarized, stapled, and validated.

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
