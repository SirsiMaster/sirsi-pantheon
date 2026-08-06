### Fixed

- Made CTR thread registration, heartbeat/current-item, close, suspend, and resume SQLite-authoritative during STORE-ONLY cutover, so sandboxed agent sessions no longer require writes to the repository-owned `threads.json` mirror.
