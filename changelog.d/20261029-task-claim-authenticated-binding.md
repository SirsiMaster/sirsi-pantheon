### Fixed

- Reject new authenticated task claims with empty or mismatched caller thread
  before consuming an attempt, and commit the session owner atomically with the
  executable lease. Preserve existing release ownership and same-worker remint
  fences; do not transfer old threadless leases to a newly registered caller.
