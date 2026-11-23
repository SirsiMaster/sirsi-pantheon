### Fixed
- The recipient-halt stalled count no longer forgets a started task when
  `task reset-attempts` zeroes `attempts`. New column `tasks.first_claimed_at`
  (SQLite schema v28, Postgres schema version 28) is written once by every task
  claim path (`ClaimTask`, `task resume-claim`) and never cleared or rewritten by
  reset-attempts, release or withdraw. The "was started" predicate is now
  `status='in-progress' OR first_claimed_at<>'' OR attempts>0`; `attempts>0` is
  kept because there is no backfill, so rows claimed before v28 count exactly as
  before. Deploy order: the router service and every `sirsi` binary that opens
  the Postgres ledger require schema version 28 exactly, so apply the schema and
  roll the service and token job to the new image together; a SQLite store
  upgraded by a v28 binary is refused by older binaries.

Refs: PANTHEON_RULES.md A35
