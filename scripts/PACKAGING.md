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

## Linux (deb / rpm)

Uses goreleaser's `nfpms` section to produce `.deb` and `.rpm` packages for the CLI binaries (no menubar — that is macOS-only).

```bash
goreleaser release --snapshot --clean
# Or for local testing:
goreleaser build --snapshot --clean
```

Output: `dist/sirsi-pantheon_VERSION_amd64.deb`, `dist/sirsi-pantheon_VERSION_amd64.rpm`

## Windows (zip)

Currently produces a zip with CLI binaries. MSIX/WiX installers are planned.

```powershell
.\scripts\build-windows.ps1 -Version "0.17.0" -Arch "amd64"
```

Output: `bin/SirsiPantheon-VERSION-windows-ARCH.zip`

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
