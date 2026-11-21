- **fix(router):** `FindRepoRoot` no longer hangs a daemon or hook at startup.
  Its `git rev-parse --git-common-dir` probe is bounded to 5 s (child killed,
  `WaitDelay` set); a timeout falls through to the existing cwd walk-up and
  marker fallbacks exactly like a git failure. Evidence: `sirsi maat
  authority-serve` stack dump in CI run 42958098771.
