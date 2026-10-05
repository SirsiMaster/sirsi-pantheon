# PR947 — CHANGES REQUIRED

/plan: Read request fully, acknowledge, claim exact review, pin source, adversarial identity control, return evidence.
/goal: Independent ownership verdict. Agent codex-pantheon; inherited thread thr-b9302c79c6136524. estimated_duration: 20 minutes.

Reviewed immutable object baa11766d85c0bf468a9fb20e8d989d8e87118f8. Freshness boundary: git fetch cannot write .git/FETCH_HEAD and git ls-remote cannot resolve github.com; this verdict is about this object, not a claim about current GitHub head.

P1: sameWorkerAcrossRemint omits host identity. MintSessionForThread accepts caller-provided agent/thread; a nonempty thread string does not establish registration. Handler defaults RuleOfRa to log. A separately minted per-host token can therefore mint its own correctly host-bound session with the owner's agent/thread strings, then pass this new fallback using a copied lease token. Previously the raw session ownership check refused that caller. The bootstrap-token host exemption is not needed for this reproduction.

Proof: isolated git archive of the object, real Handler and RemoteStore, two distinct MintHostToken hosts, separate signed sessions, same agent/thread strings. An in-memory RoundTripper invokes Handler without opening sandbox-denied listening ports. TestReviewRemintMustNotCrossHost expects ErrNotOwner; actual Complete returns nil. Reproduction source and full output are adjacent pr947-crosshost_test.go.txt and pr947-crosshost.log. The default log gate records WOULD REFUSE but deliberately permits the request. This finding applies to supported log/off modes; enforced live thread binding separately rejects the mismatched host.

Required correction: include authenticated host equivalence (using canonical identity resolution for hostname/machine-id aliases, failing closed on resolution errors), and explicitly define/test whether a registered thread binding is required for fallback even in log mode. Add signed cross-host negative controls for both item and task paths; keep same-host remint positives. Do not merge/bind this object.

Revocation boundary: SQLite GetSession returns ErrSessionRevoked/ErrSessionUnknown, so those errors fail closed. It has no time-based session expiry; do not describe expiry as tested or enforced by GetSession. Lease expiry remains separately fenced by completion methods.

Existing four new remint tests passed independently using go test ./internal/routerstore -run 'TestIdentity.*(Remint|RegisteredThread)' -count=1. The first listener-based adversarial run was sandbox-denied; the recorder-based reproduction reached the actual ownership mutation and exposed the failure.

Thread registration denied /bin/ps; inherited heartbeat succeeds. No application or other repository files changed. No bind attempted. Commits: 0. Context healthy.

Parent control: the same recorder-based TestReviewRemintMustNotCrossHost passes in a clean git archive of baa11766^ (go test ./internal/routerstore -run TestReviewRemintMustNotCrossHost -count=1). The candidate returns nil for the foreign-host completion, establishing that the fallback introduces this behavior.

Own review task pr947-review subsequently completed under its issued fence; verified registry status done. This current success does not prove recovery of historical leases or installed session remint behavior.
