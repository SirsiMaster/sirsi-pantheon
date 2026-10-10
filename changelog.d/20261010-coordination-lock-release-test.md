### Added
- Independent-review test coverage for `coordinatedHostLoad`'s lock release: a second file descriptor must acquire a non-blocking exclusive flock immediately after the call returns, paired with a negative control that holds the lock open (the exact leaked-lock shape) to prove the check has teeth.

### Changed
- `TestCoordinationFailure`'s lock-open-failure case now puts a directory at the lock file's own path (deterministic `EISDIR` regardless of uid) instead of an unwritable parent directory, which a root-run CI ignores.
