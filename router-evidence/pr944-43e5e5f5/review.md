# CHANGES REQUIRED — PR944

Reviewer: codex-pantheon, independent from Claude implementation. Exact reviewed head: 43e5e5f5338c05abc7e7a3bab9a740fd5bcb8177; base 5663a2185cd3f3801d92e8aab58dc0b8fc56c4f9. Published GitHub metadata and all six file patches read; source archived from that exact git object to /tmp/pantheon-pr944-review. No shared source changes.

/plan: read and acknowledge complete request, claim review, inspect exact object and canon, exercise normal close path and compare negative controls, return verdict. /goal: independent evidence verdict; bind only a passing current head. Estimated duration 30 minutes. Corrected candidate requested by 2026-10-01T13:45:00Z, or report ETA against existing MCP ownership task.

**P1 — the positive MCP close path cannot close a live claimed item.** cmd/sirsi-router-mcp/mutate.go:344 verifies the token, then line 354 calls f.CloseItem(agent,id,result). ClaimNext transitions open→claimed; internal/routerstore/items.go:298 CloseItem only updates status='open'. Thus the live lease can verify true and its normal completion still refuses. Independent test TestPR944VerifiedLiveClaimCanCloseThroughFacade reproduces the exact producer/verification/consumer chain: Send → ClaimNext → VerifyLease true → Facade.CloseItem. It fails: `dispatch: item ... closed but store mirror failed: routerstore: Close "...": item is "claimed", not open`. Retained positive-path-regression.go and regression.log. In the tested pre-cutover fixture, closeRaw mutates the canonical file before this store refusal, adding a split file/store outcome. Store-only MCP still cannot complete the valid claim.

The defect is directly reproducible despite existing TestVerifyLease* and full cmd/sirsi-router-mcp tests passing. TestVerifyLeaseRacePreventsStaleCloseFromClobberingReclaim does not test the submitted consumer: it uses one store handle and B calls token-fenced Complete, whereas handleClose calls un-tokened CloseItem. There is no handler-level positive completion test and no actual two-independent-instance close race in the committed test.

Correction: route MCP completion through one shared recipient-authorized, token- AND authenticated-session-fenced mutation, retaining owner-recipient safeguards. The token/expiry/status predicate must be checked in the completion transaction; changing CloseItem's status guard to allow claimed is insufficient because VerifyLease and CloseItem are separate calls. Preserve fail-closed behavior for old services and store errors. Add integration controls for a valid live claim completing, stale token after reclaim refusing while the newer result survives, unknown/terminal/wrong token/read failure, and two separate instances over SQLite and PostgreSQL. Test the actual handler/shared mutation, not a substitute branch.

Validation commands in exact-head archive, writable cache and bounded host load:

`GOCACHE=/tmp/pantheon-go-cache GOMAXPROCS=2 go test -p 1 ./internal/routerstore -run '^TestVerifyLease' -count=1` PASS.
`GOCACHE=/tmp/pantheon-go-cache GOMAXPROCS=2 go test -p 1 ./cmd/sirsi-router-mcp -count=1` PASS (linker emits duplicate -lobjc warning).
`GOCACHE=/tmp/pantheon-go-cache GOMAXPROCS=2 go test -p 1 ./internal/dispatch -run '^TestPR944' -count=1` FAIL, above.

Exact-head GitHub checks at review: Lint, two Build jobs, binding-hold and Secrets Scan success; Test still in-progress. CI is not the reason for rejection; the reproduced consumer defect is. No PostgreSQL independent execution or installed qualification is claimed.

Independent race runs of TestVerifyLease* and the full MCP package also PASS (6.233s and 1.396s), using the same writable cache, GOMAXPROCS=2 and -p 1. Retained race.log. These passes do not exercise the missing positive close path above.

The automatic approval gate rejected GitHub REQUEST_CHANGES publication because approval policy is never. No native GitHub verdict was recorded; this addressed router review is the independent CHANGES REQUIRED verdict. A34 forbids binding this head; existing Ra bind-path-router-rejections remains the enforcement dependency. No merge attempted and no approval-by-comment workaround used.

Existing MCP durable-close ownership task remains open for the corrected candidate. Ledger completion/renewal under this session's own claim is separately rejected by installed authenticated caller ownership; Ra sandboxed-consumer-session-binding remains the prerequisite. Returning this review does not qualify installed runtime or close the parent implementation.
