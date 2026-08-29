# SNE Signed Catalog Update Feed Gate

**Date:** 2026-08-18  
**Scope:** Pantheon catalog update acquisition  
**Performance claim:** None  
**Release state:** Staged, not activated

**Workspace mirror:** https://docs.google.com/document/d/1CWZCKNXmikgmbUzu63QzZcOYHvXDkAMSJB43jUMBOa8

## Implemented chain

Pantheon now supports a governed signed update feed:

1. Distribution configuration supplies fixed HTTPS feed and detached-signature URLs.
2. Pantheon downloads both with strict size limits and rejects insecure redirects.
3. Exact feed bytes are authenticated with the pinned Ed25519 trust root before parsing.
4. The signed feed binds an exact catalog SHA-256 and catalog/signature HTTPS URLs.
5. Pantheon downloads the selected version, checks its digest, and verifies its detached signature.
6. The immutable store installs it atomically while retaining the previous version for rollback.
7. A mutation lease prevents model start from racing update, rollback, or removal.

The operator UI can check a configured feed, show full-digest versions, and
install a selected version only while SNE is stopped. Feed and asset URLs are
never accepted from browser requests.

## Fail-closed coverage

- mutated/unsigned feed or wrong trust root;
- malformed, duplicate, or unknown version;
- non-HTTPS URL, insecure redirect, or oversized response;
- catalog digest or signature mismatch;
- concurrent catalog mutation or model start during mutation;
- update while SNE is starting, ready, or stopping.

## Evidence and boundary

Focused feed/signature/store tests and the complete Pantheon dashboard/SNE
suites passed. The real installed signed-catalog gate passed.

The staged feed is:

- `configs/sne/releases/sne-runtime-catalog-feed-staging.json`
- `configs/sne/releases/sne-runtime-catalog-feed-staging.json.sig`

Its real-file gate passed as `feed_id=sirsi-sne-staging`, `entries=1`, binding
catalog `bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d`.
Pantheon does not activate it by default because the referenced GitHub release
assets and clean-host retrieval are not yet proven. Staging is not availability.
