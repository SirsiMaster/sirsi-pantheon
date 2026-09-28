# Release Signing & Notarization

How a tagged release becomes a **Developer-ID-signed, notarized, stapled** DMG —
so `brew install` / `brew upgrade` is Gatekeeper-clean and the user's **Full Disk
Access grant survives upgrades** (one stable signing identity = one stable TCC
identity = no re-grant, no duplicate FDA rows).

> A tag-triggered commercial pipeline fails closed until every required protected
> input is present. Development builds may use ad-hoc signing locally, but are DEV
> ONLY — Gatekeeper warns on launch and FDA churns on every upgrade. **Do not
> distribute an ad-hoc build.**

## One-time prerequisites (Apple Developer Program)

1. Enrol in the Apple Developer Program (required for a Developer ID).
2. In Xcode or the Developer portal, create both a **Developer ID Application**
   certificate and a **Developer ID Installer** certificate for the release team.
3. Export a `.p12` containing **both identities and their private keys**, then note
   its export password. The release workflow requires both identities to be Team
   `9D382WV988`; an Application-only bundle cannot produce the commercial PKG.
4. Create an **app-specific password** for notarization:
   appleid.apple.com → Sign-In & Security → App-Specific Passwords.
5. Note your **Team ID** (10 chars) from the Developer portal membership page.

## GitHub Actions secrets to add

Repo → Settings → Secrets and variables → Actions → New repository secret:

| Secret | Value |
|--------|-------|
| `MACOS_CERTIFICATE` | one `.p12` containing the Team `9D382WV988` Developer ID **Application and Installer** identities, base64-encoded: `base64 -i cert.p12 \| pbcopy` |
| `MACOS_CERTIFICATE_PWD` | the `.p12` export password |
| `KEYCHAIN_PWD` | any throwaway password for the ephemeral CI keychain |
| `DEVELOPER_ID_INSTALLER` | exact identity name: `Developer ID Installer: Sirsi Technologies (9D382WV988)`; it must match the identity imported from `MACOS_CERTIFICATE` |
| `APPLE_ID` | the Apple ID email used for notarization |
| `APPLE_TEAM_ID` | `9D382WV988` |
| `APPLE_APP_PASSWORD` | the app-specific password from step 4 |

That's it — the workflow (`.github/workflows/release.yml`, `menubar` job) imports
both signing identities into an ephemeral keychain, proves their Team identity,
then builds the DMG and PKG, notarizes each, and staples each ticket.

## What the pipeline does (`scripts/build-dmg.sh` and `scripts/build-pkg.sh`)

1. Builds the menu bar app — the **native SwiftUI app** (`macapp/`) when present,
   else the legacy fyne binary — plus the `sirsi` CLI, into `Pantheon.app`.
2. Signs **inside-out** (inner executables, then the bundle) with the Developer ID,
   **hardened runtime** (`--options runtime`) + secure `--timestamp`, and verifies
   with `codesign --verify --deep --strict`.
3. Builds the DMG and Installer-signed PKG, then **notarizes each artifact**
   (`xcrun notarytool submit --wait`) and **staples** each ticket
   (`xcrun stapler staple`).

## Verifying a release locally

```bash
spctl --assess --type open --context context:primary-signature -v SirsiPantheon-*.dmg   # → accepted, source=Notarized Developer ID
xcrun stapler validate SirsiPantheon-*.dmg                                              # → The validate action worked!
codesign -dv --verbose=4 /Volumes/Sirsi\ Pantheon/Pantheon.app                          # → Authority=Developer ID Application: …
pkgutil --check-signature SirsiPantheon-*.pkg                                            # → Developer ID Installer: … (9D382WV988)
xcrun stapler validate SirsiPantheon-*.pkg                                               # → The validate action worked!
```

## The contract this enforces

A stable Developer ID means macOS TCC recognizes every version as the **same app**.
So a user grants Full Disk Access **once**, and `brew upgrade` keeps it — no warning,
no re-grant, no new row in the Full Disk Access list. That is the difference between
"a tool you can hand to people" and the ad-hoc build that clutters their machine.
