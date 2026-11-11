### Changed
- Ma'at reservations: the hot `maat:reservations` document now keeps only
  non-terminal rows and rows that ended within the last 2 hours (it was every
  row that STARTED within 48 hours, ~4 MB / ~8,000 rows, rewritten whole by every
  reserve, heartbeat and release). Older finished rows move to the existing
  per-day archive rows; nothing is deleted. Lookups by id (`maat:<id>` completion
  receipts, `sirsi maat status --id`, replay/upsert idempotency, mutate-by-id,
  caller-supplied duplicate ids) fall back to the archive and behave exactly as
  on a hot row. Invalidated rows stay hot 25 h past their end so `maat plan`'s
  24 h conflicts-caught count is unchanged. `sirsi maat status` (no id) lists
  the last ~2 hours of finished rows instead of 48.
- New additive reservation field `ended_at`, stamped on release, invalidation
  and lease expiry. Older rows lack it and fall back to their own timestamps and
  the previous 48 h-from-start rule. No schema change.

Refs: PANTHEON_RULES.md A35, maat-hot-row-shrink, maat-reserve-latency-dashboard-start
