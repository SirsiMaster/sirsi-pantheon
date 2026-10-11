- **fix(maat):** `sirsi maat should-defer` help now states its real exit contract (97 = a runner must
  defer, 0 = clear), matching `internal/maat/schedule/README.md` and every other admission command.
  The old text said "Exit 0 if a runner must defer", so a runner that trusted it deferred every job
  (Mercury #575 worked around it). Guard test `TestShouldDeferHelpStatesTheExitContract`. The exit
  code is unchanged, so nothing that already works against it moves. Refs: mercury item 20261011-123115.
