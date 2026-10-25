### Fixed

- Own-lane cause-fixed recovery accepts an inactive blocked original with
  ordinary attempts remaining. It consumes the next attempt with the existing
  authenticated transaction, evidence audit and row-version fence; it never
  resets history or marks the task complete. Pending/in-progress below-ceiling
  tasks retain ordinary claim semantics. Focused qualification passed; protected
  integration, independent review, release and real-row deployment remain open.

Refs: docs/stacklab/maat/canon/OWN_BLOCKED_TASK_RECOVERY_PLAN.md;
docs/ADR-061-DURABLE-CONTINUOUS-EXECUTION-ENFORCEMENT.md.
