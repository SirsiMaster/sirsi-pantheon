### Fixed

- Serialize native managed app update/rollback with the existing legacy Installer
  and root-maintenance owner. Consume review before suspension, refuse stale
  consent after another operation, and retain custody until canceled work joins.
  Preserve separate memory recovery and receipts. Remove obsolete rollback view
  cleanup references; add inert bidirectional overlap regressions.
