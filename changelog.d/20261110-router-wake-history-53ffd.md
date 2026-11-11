- Add read-only `sirsi router wake-history <agent>` for one lane's newest
  canonical terminal wake failures (or `--status all`), with bounded JSON,
  recognized diagnostic vocabulary and complete-error byte counts/SHA-256
  fingerprints. Raw errors, credentials, source prose, leases and acknowledgment
  refs are omitted. Reuses the existing authenticated `ListWakeEvents` RPC;
  last/count and byte limits bound client output, not the full per-lane RPC
  acquisition. No wake, retry, reconcile or new authority is introduced.

- Refs: ADR-061, ADR-062, PANTHEON_RULES.md (A11, A16, A35); reviewed source
  `53ffd04d5cc695aa1da40ea8c9e4847275271efd`. Existing standalone evidence does
  not qualify this combined tree.
