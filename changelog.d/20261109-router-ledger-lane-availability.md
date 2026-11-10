- **Lane-ledger availability:** when a successful ledger read has no lane entry,
  distinguish an unavailable lane inventory (503) from a successfully unknown
  agent (404) and a known lane with an empty ledger (200 with empty arrays).
  Preserve generic errors, read-only routing and the existing projection.
