- **fix(router):** stopped the session table growing ~86k rows per 48 h. Root cause (measured on the M5): the client's cached session is
  valid only for the exact executable that minted it, and two binaries acting as the same agent (19 wake loops that all fall back to the
  hostname, 8 running an older binary) shared one cache file and invalidated each other on every call. The cache file is now keyed by
  runtime hash (`<agent>.<runtime>.json`), stale cache files are pruned after 7 days, and the service garbage-collects dead sessions
  (unrevoked and unseen for 24 h, or revoked for 24 h; never a session that owns a lease on an open item or unfinished task), amortized
  on mint at most every 5 minutes in batches of 5000. New operator tools: `sirsi router sessions stats` and
  `sirsi router sessions gc [--dry-run]` (service backend, like `token`). Tests with negative controls: the flap (20 sessions -> 2), lease
  protection, freshness protection, batch bound, dry-run equals real GC.
