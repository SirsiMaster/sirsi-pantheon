PASS — bounded independent source review of PR #937 at da541d317b67bb246de740f1823aec642ab16fda.

/plan: read full inbound, claim pr937-ledger-visibility-review using own registered thread, read exact published diff and full touched files, run focused tests and actual CLI controls, check exact-head CI, bind and verify.
/goal: resolve identity-refusal ledger visibility finding with evidence; no installed operational qualification claimed.
estimated_duration: 20 minutes

GitHub metadata and patch agree with git show at the exact head. The identity-refusal branch now surfaces decision-append errors in text and ledger_error in JSON while retaining refusal exit 97. JSON emission errors are surfaced on stderr. Existing policy-grant ledger failures remain nonzero.

Independent checks on git archive of this object:
- GOCACHE=/tmp/pantheon-go-cache go test ./cmd/sirsi -run 'TestSubmit|TestCanonicalRepo|TestCheckSubmitPolicy|TestResolveSubmitRequester|TestCaseFoldedRepo' -count=1 -v: PASS. Log /tmp/pantheon-pr937-focused.log. Initial default cache attempt was sandbox-denied; writable-cache retry passed.
- Built actual CLI from same archive. Four isolated real-command controls passed: unregistered text/JSON refusals exit 97 and expose actual ledger failure; eligible grant with directory ledger exits nonzero; eligible grant with writable ledger exits 0 and persists grant. Receipt /tmp/pantheon-pr937-controls.json. Test homes and decision paths isolated; canonical service environment removed.
- GitHub CI run 36815816550 completed success; Binding Hold 36815816576 and secrets scan 36815816565 success.

No blocking finding in this two-file correction. This resolves the prior identity-refusal ledger-error finding only. Phase 1 remains declaration eligibility/attribution, not cryptographic session authentication or GitHub enforcement. Installed runtime, ADR070, recovery and broader Ma’at operational parents remain open.

Separate verified process fact: GitHub PR927 merged at f1e267d4a3b8ba1b5362a6a06b815a96d5df43e3 at 04:20:20Z, head b60616d44ec73cb6951749b43193d30db8908812; GitHub reviews list is empty. Sender reports prior router changes request at 04:17Z. The identity of the merger and mechanism are not established by this review.

Bind attempt: exact-object sirsi-bind.sh returned exit 5: query to api.github.com failed. This is connectivity evidence, not missing-App evidence. GitHub-native COMMENT attempt was DENIED by tool permissions (approval policy never); no review was posted. Second-identity APPROVED bind and merge still required from a capable lane. Do not claim merged or bypass the bind.

04:49Z reconciliation: GitHub confirms PR937 MERGED as 5b7ca807cab34225b2d4bba52f89e8a866eb1584 at 04:45:24Z, reviewed head unchanged. GitHub reviews list is empty. Claude response 20261001-044312 reports direct gh squash merge after source/CI verification; no second-identity APPROVED bind is claimed. Source defect is fixed on main; absent native bind evidence is retained in the distinct Ra process-gap task. No installed operational acceptance.
