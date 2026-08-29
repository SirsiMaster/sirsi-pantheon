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
4. Prefer an **App Store Connect API key** for notarization. An Apple-ID
   app-specific password remains supported as a fallback.
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

Preferred API-key alternative to the final three Apple-ID fields:

| Secret | Value |
|--------|-------|
| `ASC_KEY_PATH` | absolute path to `AuthKey_<KEYID>.p8` on the macOS release runner |
| `ASC_KEY_ID` | App Store Connect API key ID |
| `ASC_ISSUER_ID` | App Store Connect issuer UUID |

That's it — the workflow (`.github/workflows/release.yml`, `menubar` job) imports the
cert and runs `scripts/build-dmg.sh`, which signs, notarizes, and staples.

## What the pipeline does (`scripts/build-dmg.sh`)

1. Builds the canonical Go control engine (`cmd/sirsi-menubar`) plus the `sirsi`
   CLI into `Pantheon.app`. The SwiftUI package under `macapp/` is an additive
   native product surface and design/prototyping target; it may not silently
   replace the canonical engine in a release package.
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
## App Store Connect API-key notarization

The preferred notarization credential is an App Store Connect API key. It is
non-interactive, auditable, and does not require storing an Apple ID password:

```bash
export ASC_KEY_PATH=/secure/path/AuthKey_KEYID.p8
export ASC_KEY_ID=KEYID
export ASC_ISSUER_ID=ISSUER_UUID
```

`scripts/build-dmg.sh` submits with these three values when all are present.
The Apple ID/app-specific-password variables remain a compatibility fallback.
Pin both `VERSION` and numeric `BUILD_NUMBER` for a release; the script embeds
them into `Pantheon.app` before signing so the DMG, bundle, signature,
notarization receipt, and update metadata identify the same artifact.
