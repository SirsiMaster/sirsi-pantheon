- feat(routerstore): migration gate — a build with uncommitted changes may not
  apply a schema migration. Source that exists only in a working tree cannot be
  rebuilt by any peer, so the migration it performs is unrecoverable by
  construction. Every migration now records its provenance in the existing
  `state` table, and the "schema newer than this binary" refusal names the build
  that did it instead of posing a forensic puzzle. Override:
  `SIRSI_ALLOW_DIRTY_MIGRATION=1`, loud and explicit.
