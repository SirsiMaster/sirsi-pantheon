# Production runbook — Pantheon PT

Verify clean commit/tree and source receipts. Run focused normal/race tests, full Go and Swift gates, package inventory, and canonical render/verify. Publish only create-only evidence with hash/readback. Use signing/notary and remote/cask lifecycle only under their own credentials and fresh authority. On failure, stop, preserve evidence, and roll back only to the exact known-good revision.
