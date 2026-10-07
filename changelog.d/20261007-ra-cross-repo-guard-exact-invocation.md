### Fixed
- `knownfail.ciRunsExactPath` now parses the CI workflow as real YAML and
  requires the guard path to be in shell command position (the first word of
  a `run:` statement, or the second word after a known interpreter wrapper
  like `bash`/`sh`/`source`), instead of accepting any whitespace token that
  matched the path anywhere on a line. A path merely named in a step's
  `name:` field, or passed as an argument to `echo`/`cat`, no longer counts
  as an executed regression guard.

Refs: PANTHEON_RULES.md, internal/maat/knownfail/registrar.go, PR #1031
