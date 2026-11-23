### Fixed
- `FindCallerRepoRoot` now bounds its `git rev-parse --show-toplevel` probe at 5s (same bound as `FindRepoRoot`, #1401) and still fails closed on timeout, so a hung git can no longer hang a completion-proof caller.

Refs: `internal/router/router.go`, `internal/router/router_findreporoot_test.go`
