### Added

- Added a fixture-only native prefix-cache-pressure renderer and Swift test
  target. It proves the visible owner-confirmation flow and exact receipt
  bindings without contacting a live Pantheon/SNE service or inferring an SNE
  execution result.

### Fixed

- Rejected malformed receipt identities before protected read-only routes and
  rejected malformed fixture-renderer appearance arguments before app startup.

Refs: SIRSI_MANIFESTO.md §8; SNE Pantheon prefix-cache pressure integration contract v1
