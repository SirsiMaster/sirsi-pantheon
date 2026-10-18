### Fixed
- Run reservation decisions, heartbeat/release and expiry sweeps in one canonical
  database transaction so concurrent hosts cannot overwrite each other's grants.
- Refuse legacy unfenced reservation writes and preserve fail-closed cloud scope.
- Stop mistaking Python/shell job arguments containing Go's `-p` for a print-mode
  Claude worker during actual runtime registration.

Native deployment proof is tracked in docs/runbooks/maat-atomic-reservations.md;
source qualification is not a production-generation completion claim.
