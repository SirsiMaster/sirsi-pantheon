# Release Signing & Notarization

How a tagged release becomes a **Developer-ID-signed, notarized, stapled** DMG —
so `brew install` / `brew upgrade` is Gatekeeper-clean and the user's **Full Disk
Access grant survives upgrades** (one stable signing identity = one stable TCC
identity = no re-grant, no duplicate FDA rows).

> Until these secrets are set, the pipeline falls back to **ad-hoc** signing.
> Ad-hoc builds are DEV ONLY — Gatekeeper warns on launch and FDA churns on every
> upgrade. **Do not distribute an ad-hoc build.**

## One-time prerequisites (Apple Developer Program)

1. Enrol in the Apple Developer Program (required for a Developer ID).
2. In Xcode or the Developer portal, create a **Developer ID Application** cert.
3. Export it (with its private key) as a `.p12` and note the export password.
4. Create an **app-specific password** for notarization:
   appleid.apple.com → Sign-In & Security → App-Specific Passwords.
5. Note your **Team ID** (10 chars) from the Developer portal membership page.

## GitHub Actions secrets to add

Repo → Settings → Secrets and variables → Actions → New repository secret:

| Secret | Value |
|--------|-------|
| `MACOS_CERTIFICATE` | the Developer ID `.p12`, base64-encoded: `base64 -i cert.p12 \| pbcopy` |
| `MACOS_CERTIFICATE_PWD` | the `.p12` export password |
| `KEYCHAIN_PWD` | any throwaway password for the ephemeral CI keychain |
| `DEVELOPER_ID_APPLICATION` | the identity name, e.g. `Developer ID Application: Sirsi Technologies (TEAMID)` (from `security find-identity -v -p codesigning`) |
| `APPLE_ID` | the Apple ID email used for notarization |
| `APPLE_TEAM_ID` | the 10-char Team ID |
| `APPLE_APP_PASSWORD` | the app-specific password from step 4 |

That's it — the workflow (`.github/workflows/release.yml`, `menubar` job) imports the
cert and runs `scripts/build-dmg.sh`, which signs, notarizes, and staples.

## What the pipeline does (`scripts/build-dmg.sh`)

1. Builds the menu bar app — the **native SwiftUI app** (`macapp/`) when present,
   else the legacy fyne binary — plus the `sirsi` CLI, into `Pantheon.app`.
2. Signs **inside-out** (inner executables, then the bundle) with the Developer ID,
   **hardened runtime** (`--options runtime`) + secure `--timestamp`, and verifies
   with `codesign --verify --deep --strict`.
3. Builds the DMG, signs it, then **notarizes the DMG** (`xcrun notarytool submit
   --wait`) and **staples** the ticket (`xcrun stapler staple`).

## Verifying a release locally

```bash
spctl --assess --type open --context context:primary-signature -v SirsiPantheon-*.dmg   # → accepted, source=Notarized Developer ID
xcrun stapler validate SirsiPantheon-*.dmg                                              # → The validate action worked!
codesign -dv --verbose=4 /Volumes/Sirsi\ Pantheon/Pantheon.app                          # → Authority=Developer ID Application: …
```

## The contract this enforces

A stable Developer ID means macOS TCC recognizes every version as the **same app**.
So a user grants Full Disk Access **once**, and `brew upgrade` keeps it — no warning,
no re-grant, no new row in the Full Disk Access list. That is the difference between
"a tool you can hand to people" and the ad-hoc build that clutters their machine.
