- **fix(router):** close a TOCTOU race in the lane-scoped host token API (schema v25). The
  existing-owner scope check for `UpsertThreads`/`UpsertThreadCAS`/`ResumeThreadCAS` was a
  separate read (`ThreadBinding`) before the write, so a legitimate concurrent write to the
  same row between the read and the write could let a scoped token's write land anyway
  (independent SSA adversarial review, round 2, `TestSSAConcurrentExistingAgentChange`). The
  check is now part of the write's own SQL predicate (`internal/routerstore/threads.go`:
  `UpsertThreadsScoped`, `UpsertThreadCASScoped`, `ResumeThreadCASScoped`), evaluated
  atomically with the write by the database itself — the same pattern this code already uses
  for host ownership. New stress test `TestScopedThreadWriteCannotHijackExistingForeignRowConcurrently`
  (50 concurrent hijack attempts racing 50 legitimate heartbeats on the same row, `-race`,
  5x) never lets the out-of-scope agent win.
