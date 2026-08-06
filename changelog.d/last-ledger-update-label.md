- fix(board,menubar): rename the "touched" column to "last ledger update". It
  measures task-record mutation, not liveness — a lane doing real work without
  recording it reads as stale, correctly — but the old label invited reading a
  bookkeeping figure as a heartbeat. The owner read claude-deck's accurate
  "touched 1d1h" as a bug because of it.
