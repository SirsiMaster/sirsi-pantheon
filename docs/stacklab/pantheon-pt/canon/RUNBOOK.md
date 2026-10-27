# Production runbook — Pantheon PT

Verify clean commit/tree and source receipts. Run focused normal/race tests, full Go and Swift gates, package inventory, and canonical render/verify. Use `--development` only for clearly named local package work; it cannot satisfy release evidence. Use `--release` only with the full Developer ID/notarization credential set, then staple and validate each commercial DMG/PKG before remote publication. Publish only create-only evidence with hash/readback. Use remote/cask lifecycle only under their own authority. On failure, stop, preserve evidence, and roll back only to the exact known-good revision.

## Corrective certification procedure

1. Record clean source commit/tree/parents/VERSION, original task IDs, component
   reviews and exact execution/cleanup roots. Inspect existing PR/tag/workflow
   custody first; never launch a duplicate release.
2. Read `CERTIFICATION.md`, the component catalog and `DIAGRAM_INDEX.md`.
   Reconcile each applicable requirement, failure and next action against code.
3. Use existing bounded Ma'at CPU scheduling and isolated caches; record actual
   source/environment and durable command result before the next gate. Preserve
   first failures and fix their causes; no branch-protection or check waiver.
4. Qualify the composed Go/Swift source, static/surface contracts, package
   inventory, canonical cask validation and required causal controls. Component
   evidence is not a whole-composition result.
5. SSA selects the existing normal route for the agreed publication AND install
   scope. `scripts/release-train.sh` is not publication-only: after tagging and
   waiting for release CI it runs a local Homebrew cask upgrade/version check,
   then attempts an M5 SSH cask upgrade. Omitting `--deploy-service` suppresses
   service deployment only, not those installations. Run the train once only
   when those actual effects are in the selected runtime scope; otherwise use
   the existing separately scoped publication route under SSA custody. Do not
   add a duplicate tag/publisher or treat the later checks as the first install.
   Use established delegated signing and verify Team/application/installer,
   notary/staples/resources and final payload identity.
6. Re-read both public assets/hashes, exact tag/source/release and canonical tap
   bytes. Ambiguous publication requires reconciliation, not clobber/replay.
7. Runtime custodians exercise supported lifecycle in isolated contexts and
   selected loaded-client/UI adoption. Preserve active/unknown/foreign work.
   Recheck original caller paths without changing its dirty files.
8. Record exact cleanup or retained owned debt. An exited process or absent path
   does not certify unknown old requests as drained.
9. Run `sirsi stacklab doctor --json`; retain exact origin wing, roster and pin
   hashes. Refresh a changed pin through its custodian after origin merge.
10. Update existing Owner Reading Room and native Workspace records, cross-links,
    timestamps and sync status; do not create a duplicate reading authority.
11. Independent review resolves applicable certification rows. Close only
    satisfied originals. Release publication alone does not end the freeze.

Repairs and qualification continue while a proof is pending. This procedure is
not another owner-approval loop or a reason to invent a credential blocker.
