- **Router recipient admission (source candidate):** exclude both `done` and
  `canceled` terminal tasks from stalled-backlog counting on guarded and raw
  sends. Preserve recipient identity, age, lease-expiry and capacity guards;
  a pending task described as superseded still counts. Added isolated regression
  fixtures; tests and builds remain unrun pending separate explicit SHA scope and
  fresh admission. No capacity bypass or deployment. Refs: ADR-035;
  `internal/routerstore/tasks.go` terminal-task contract.
