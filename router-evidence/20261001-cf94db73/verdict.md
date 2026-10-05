# PR944 cf94db73 independent review

Agent: codex-pantheon. Thread: thr-b9302c79c6136524.
Candidate: cf94db73084fb8cec37bc7ac62448c56e0b4524b.
/plan: read exact source, run focused controls, verify published CI, reconcile durable parents.
/goal: independent source verdict with explicit release qualification boundary.
Estimated duration: 20 minutes. Next check: on returned bind/qualification receipt.

SOURCE REVIEW PASS. The three retained source gaps are corrected. Read the exact git objects and extracted git archive, not the dirty ambient checkout. PostgreSQL is present and the independent-handle tests now select openBackendStore/openSecondHandle without resetting the second handle. The authenticated identity harness covers stolen live tokens and expired/reclaimed tokens through RemoteStore -> Handler -> checkItemOwner. CompleteItem now has owner-recipient, undeclared/nondelegated actor, audited declared delegation, and unknown-item refusal controls. MCP handleClose uses CompleteItem; the store fence remains atomic and the mirror follows successful store completion.

Independent local verification at the extracted exact candidate:
- GOCACHE=/tmp/pantheon-go-cache go test ./internal/routerstore -run 'TestComplete(TwoIndependent|Unavailable|Unknown)|TestVerifyLease' -count=1: PASS.
- Full internal/dispatch and cmd/sirsi-router-mcp: PASS.
- Focused go test -race ./internal/dispatch ./cmd/sirsi-router-mcp -run 'TestCompleteItem|TestClaim|TestRegistration|TestHandleClose' -count=1: PASS.
- Full routerstore run cannot finish here: httptest listen tcp6 [::1]:0 bind operation not permitted. This is a sandbox limitation, not an observed candidate failure.

Published GitHub PR metadata independently confirmed open, mergeable and unchanged exact head. CI run 36882533656 is successful; lint, test and both builds passed. Binding Hold 36882533589 and Secrets Scan 36882533537 successful. Read the Test job 110437632186 log: authenticated identity controls pass in the SQLite/race suite. IMPORTANT: PostgreSQL CI leg was SKIPPED ('initdb not on PATH; Postgres leg not run'). Therefore green CI is not a PostgreSQL execution receipt. The author's successful scratch-PostgreSQL report is attributed author evidence; this session did not independently execute PostgreSQL.

No new source correction requested. Release/bind disposition must preserve the PostgreSQL qualification distinction. Please retain a rerunnable PostgreSQL run receipt (or run the existing CI leg on a qualified runner) and execute normal bind/release; do not represent the skipped CI step as PostgreSQL verification. Installed authenticated operational qualification remains a separate parent obligation.

Managed lane limitations: git fetch fails writing .git/FETCH_HEAD; GitHub connector supplied current PR/job truth. Own normal thread registration fails /bin/ps operation not permitted; inherited thread heartbeat succeeds. Exact review task claim issued lease cbd6e184757bcbdaa2b3b194e43fd1fb, but normal completion refused 'caller\'s session does not own this lease'. Historical claim/reconciliation probes also refused; no lease/store bypass attempted. These ledger obligations remain unresolved, not done.
