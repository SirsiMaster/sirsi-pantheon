- **feat(router):** a lane can withdraw its own task with
  `sirsi router task withdraw <agent> <task-id> --superseded-by <agent>/<task-id> --reason "..."`.
  `withdrawn` is a terminal status that never counts as done. It must name a
  live task that carries the work. The task's timeline records who withdrew it
  (the authenticated session) and why. The ledger board no longer counts
  canceled or withdrawn tasks as active work.
