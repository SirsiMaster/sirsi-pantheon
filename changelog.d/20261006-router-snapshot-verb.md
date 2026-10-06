### Added

- **`sirsi router snapshot`: the router snapshot as JSON, from the same producer as the Horus dashboard.** The native menubar shells out to `sirsi ... --json` and never speaks HTTP, so it could not read `/api/router`; this verb prints the identical snapshot so web and native render one set of router decisions instead of recomputing them. The JSON field names are pinned by a contract test and documented in `docs/router-service/SNAPSHOT_CONTRACT.md` (which fields serve which view, the allowlisted copy-only next steps, what is deliberately absent, and failure behavior).
