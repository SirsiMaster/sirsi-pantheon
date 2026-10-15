- **fix(router):** two P1 gaps in the lane-scoped host token API (schema v25, `internal/routerstore/lanescope.go`) found by
  independent SSA review of PR #1079. (1) `ListThreads` returned every thread's full record — including the raw continuation
  payload (`ResumePrompt`, etc.) — to any scoped token; it is now filtered to the token's own agents. (2) `UpsertThreads`,
  `UpsertThreadCAS` and `ResumeThreadCAS` validated only the record's incoming agent, so a scoped token could take over an
  existing thread row owned by another agent by naming its id, when that row had a blank (legacy) or same host — the existing
  host check in `threadAuthority` only protects a row already bound to a *different* host. Both writes now also check the
  existing row's owner via `ThreadBinding`. New negative-control tests: `TestScopedListThreadsHidesForeignPayloads`,
  `TestScopedThreadWriteCannotHijackExistingForeignRow`.
