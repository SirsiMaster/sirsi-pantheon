# Router bulk export contract

Owner: Ra. Answers SSA 20261106-155725 (task `ra-router-bulk-export-contract-20261109`).

## Decisions

| Question | Answer |
|---|---|
| Caller | Any already-authorized token for which the service serves `ListPage`: today, an unscoped fleet host token. `ListPage` is not in the lane API or in observer reads, so lane-scoped and board tokens stay refused. No permission is widened. |
| Where files are written | On the caller's own Mac, by `sirsi router export --dir <path>` (`routerstore.ExportMarkdownPaged`). The service never receives a path. The legacy `ExportMarkdown` RPC stays refused (`serve.go`). |
| Size limits | One call reads at most `ListPageMax` = 2,000 items (about 7 MB at the measured 3.7 KB per item), inside every transport limit. Memory stays one page wide. Each call also has the client's per-call budget and the service's request timeout. |
| Server floor | A service that serves `ListPage`: PR #1075 (b8d7e453, 2026-10-11) onward. Every revision deployed since then qualifies. |
| Completeness | **An explicitly incomplete, best-effort keyset scan.** Every item that existed when the walk started, and was not removed, is written exactly once. An item created behind the cursor during the walk is missed. Each page shows its items as they were when that page was read. The report always says `"snapshot": false`. |
| Point-in-time copy | Not an export's job. A consistent point-in-time copy comes from Cloud SQL point-in-time recovery into an inactive clone (`docs/router/ROUTER_RECOVERY_PLAN.md`). |
| Partial errors | The export stops at the first refused page or failed write. Files already written stay, and the report counts exactly those. A page that does not advance the cursor is an error, never a retry. |

## Output

Each item is written as `<id>.md` in the file router's canonical format, the
same bytes `ExportItem` and the file router produce. stdout gets one JSON
report: `{"items":N,"pages":P,"last_id":"...","snapshot":false}`.
