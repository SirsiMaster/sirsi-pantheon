# Pantheon M1 read-only host inventory — 2026-08-24

This is a bounded, non-mutating SSH inventory of the reachable M1 deployment-validation host (`MacBookPro`). It does not authorize installation, service restart, or security changes.

## Observed state

- OS: macOS 26.6.2, arm64.
- Full Xcode: Xcode 26.6 (17F113), selected developer directory `/Applications/Xcode.app/Contents/Developer`.
- Remote transport: TCP 22 and TCP 5900 listening on IPv4 and IPv6.
- FileVault: `FileVault is On.`
- Launch state observed: `ai.sirsi.host-readiness-watch`, `ai.sirsi.pantheon`, `ai.sirsi.sne.supervisor`, and `ai.sirsi.pantheon.dashboard`; Tailscale login-item helper and application entry also present.
- `brew`, `go`, and `node` were not found on the SSH session PATH.

## Disposition

M1 satisfies the current remote-access and full-Xcode prerequisites. Absence of Homebrew/Go/Node is recorded as a deployment-host exception, not silently treated as drift or repaired remotely. If M1 is later admitted as a benchmark peer, install and version receipts for those tools must be obtained through the supported owner-visible path before qualification. No claim is made here about Pantheon package lifecycle, sustained performance, or release readiness.
