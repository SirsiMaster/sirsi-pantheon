- **feat(router):** `sirsi router export --dir <path>` writes every item as
  `<id>.md` on the caller's own Mac. It reads in bounded pages (at most 2,000
  items per call) through the existing `ListPage` RPC, and reports
  `snapshot: false`, because a page-by-page walk is not a point-in-time copy.
  The contract (caller, limits, server floor, completeness) is in
  `docs/router/BULK_EXPORT_CONTRACT.md`.
