- Bind the native release job to the existing `m1` runner label and fail before checkout unless runner context, native Darwin/arm64, and `hw.model` match the qualified `MacBookPro18,1`. The hosted source fixture binds these checks to `jobs.menubar` and runs before tag release.
- Run keychain cleanup only after accepted host binding and an explicit marker for keychain mutation; retain cleanup after later signing failures while leaving rejected or pre-keychain failures untouched.

Refs: docs/ADR-042-SELF-HOSTED-CI-ON-SIRSI-SILICON.md, scripts/verify-menubar-release-contract.sh, scripts/verify-menubar-release-contract.test.sh
