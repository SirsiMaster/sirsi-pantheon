### Added
- `sirsi router task-lease-audit [task-id]` — a read-only audit of task-lease
  ownership decisions (`checkTaskOwner` verdicts, `BindTaskSession` outcomes)
  from a new `task_lease_log` table (schema v24, both SQLite and Postgres).
  Closes the gap `audience_log`/`sirsi router audience` left: that log covers
  the thread-registration gate, not task claim/bind ownership — there was no
  durable record of "who held this lease and what did the service decide" for
  a given task id until now.
