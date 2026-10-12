# Scoped cloud access — integration correction

Classification: platform-foundation. References: ADR-062, ADR-061, owner integrated A2A/M1/M5/Studio/cloud direction.

Ra's `cac71b10` and `cb4d4e4e` are integrated on the signed v0.24.102 source base `0dbbf30c`. The integration additionally closes four boundaries:

- A recognized scoped thread mutation refuses a backend without atomic scoped operations. It cannot fall through to an unscoped write.
- Ownership lookup errors deny. They do not mean the existing row has no owner.
- Thread deletion combines agent scope with status, timestamp and host CAS predicates in the database delete itself.
- A changed-owner atomic refusal is HTTP 403, not a retryable HTTP 500.

Tests exercise foreign thread payload reads, blank/same-host takeover attempts, resume/upsert conflicts, scoped deletion, unsupported backends, batch rollback and a changed-owner interleaving. The interleaving now verifies that the foreign ownership update actually landed before the scoped write: distinct increasing timestamps are essential because the original same-timestamp injected update could be ignored by freshness rules. This corrects the previous test attribution; it is not evidence that the original probe actually changed ownership.

Focused host-surface race command: `go test -race ./internal/routerstore -run 'TestSSA|TestScopedThread|TestLane' -count=1 -timeout=120s`.
Result: PASS, 5.535 seconds. Log: `/Users/thekryptodragon/.sirsi/evidence/router-a2a-scoped-cure-focused-race-host-20261012.log`.
The restricted-surface rerun was denied loopback listener creation; that failure is retained separately and is not classified as a source regression.

This is source/test evidence, not a live service deployment or cloud-lane credential qualification. Full suite, independent integrated review, service rollout, scoped positive/negative live requests, Studio enrollment and app-workload movement remain separate unfinished steps on `router-a2a-cloud-studio-20261012`.

Owner access: canonical repository document is authoritative. Desktop/Workspace mirrors are pending final integrated publication; this document is not represented as fully three-home published.
