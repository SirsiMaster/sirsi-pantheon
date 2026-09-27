# Sirsi Pantheon — Packaging Guide

How to build distributable packages for each platform.

## macOS (DMG and PKG)

Produces a drag-and-drop DMG installer containing `Pantheon.app` with both the menu bar app and the CLI binary.

```bash
make dmg
# Or directly:
scripts/build-dmg.sh --version 0.17.0 --arch arm64
```

Output: `bin/SirsiPantheon-VERSION-ARCH.dmg`

Build the installer package from that exact same application bundle:

```bash
make pkg
# Or after building the DMG/app bundle:
scripts/build-pkg.sh --version 0.23.9-beta --app Pantheon.app
```

Output: `bin/SirsiPantheon-VERSION-arm64.pkg`

Requirements: macOS (hdiutil/pkgbuild), Go toolchain. A distributable DMG
requires a Developer ID Application identity and notarization credentials; a
distributable PKG additionally requires a Developer ID Installer identity.

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
