# Sirsi Pantheon v0.24.12 — Stack Lab release receipt

- Tag: `v0.24.12`
- Source commit: `53ca7cfe974026b059fe629d0f2fdb9759723787` (PR #791 merge)
- Predecessor: [`v0.24.11`](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.24.11)
- Scope: Ma'at scheduler classification of idle GitHub Actions runner
  listeners versus active `runner.worker` jobs.
- Stack Lab wing: `maat` / Pantheon scheduling and governance.

## Verification

- PR #791 passed binding hold, lint, tests, secrets scan, and ARM64 build.
- The release retains Pantheon’s platform-foundation boundary. Signing,
  notarization, cask publication, and installed-host lifecycle evidence remain
  separate receipts.
