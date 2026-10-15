- **fix(router):** lane-scoped tokens now also check the existing owner of a thread before an upsert overwrites it
  (`threadRecordsInScope`). Found by the automated security review of #1079 after it merged: a scoped token could take over
  another agent's thread on the same host by reusing its id with its own agent name. Test
  `TestScopedSessionCannotRewriteAnotherAgentsThread` was red before the fix and is green after. Not exposed: schema v25 and
  scoped tokens are not yet deployed to production.
