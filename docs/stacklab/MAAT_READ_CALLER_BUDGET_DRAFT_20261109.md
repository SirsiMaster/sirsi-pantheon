# Initial Ma'at read caller budget — source-only a0a successor

Classification: internal-tool. Status: UNRUN; independent review and separately
admitted qualification required. Base is exact a0a9b47cb3ca80502d9bbb51d4dc21ea92b31593,
tree5de0d836c86e86ed06a3f79fa32043cbb7459ca1. No frozen release, installed
source755, defaultdc041, 9+4 Studio packets, service, consumer or canonical
state changes. No fetch, build, test, CI, signing or release action performed.

## Minimal carry-forward, not a competing repair

Reviewed the complete a0a spool.go and PR1375 read compatibility contract,
read/unavailable tests and relevant caller-timeout/redial/handshake controls.
Reused the optional read_deadline_unix_nano RawMessage design and four authored
test functions from an existing, privately retained UNRUN pre1375 candidate.
That candidate stays untouched. Its forwardNew/forwardRequest split is NOT
ported: a0a's forwardWithQueuePolicy and PR1375 conservative initial-read
classifier remain the existing integration path. Also do not port its blanket
context-error override: an explicit dial failure is still phase evidence,
whereas deadline expiry alone is not.

Only spool.go changes production behavior. New test and this scope note are
the other two source files. No supervisor, RPC/service schema, lease, admission,
HTTP client/TLS/redirect settings, mutation or outbox implementation changes.

## Exact timing contract

For a newly submitted MaatRead, RoundTrip's existing context remains the earlier
of its incoming context and 30-second transport wait. After cooperative spool
admission/slot acquisition, publish an optional absolute Unix-nanosecond cutoff
equal to THAT original deadline minus two seconds. Insufficient remaining time
fails before publication, without a request ID/envelope or new retry.

The relay honors the field ONLY for an initial MaatRead, never a mutation or
queued record. Its one context ends at min(encoded cutoff, handling-now+25s).
Both connection attempts and HTTP response-body lifetime share it; the second
attempt cannot reset it and is not started when that context is already expired.
Absolute cutoff charges admission and lane/discovery delay instead of granting
25 new seconds after a delayed pickup. A malformed, nonpositive or expired
field is refused before the first forward and settled as PR1375's exact503
never-sent/read-not-queued response.

After forwarding begins, existing phase-aware classification alone governs:
every attempted error proved dial/DNS/TLS-before-send may yield the unchanged
503 sentinel. Generic context deadline, response-header/body failure, read/write
phase or mixed/ambiguous errors remain502 UNKNOWN, parked non-retryable; they
are NOT reclassified from timeout alone. Service denials retain precedence.
No initial read is newly placed in the asynchronous outbox by this correction.

The two-second reserve is explicit response publication/polling headroom,
NOT a delivery guarantee. Scheduling/FS failure, paused/dead relay or caller,
wall-clock jumps across the serialized local deadline, and abrupt cancellation
can still yield retained custody or a caller UNKNOWN. It grants no heartbeat,
lease extension, successful revocation check, retry authority or new admission.
Generic transport implementations must honor request contexts; the production
net/http transport does. No claim covers a custom transport ignoring cancellation.

## Compatibility and retained obligations

Old clients omit the field and retain PR1375 behavior, without this shared
timing bound. Older relays ignore the additive field: new client/old relay does
NOT gain the bound and may still queue reads according to its installed version.
RawMessage deliberately avoids rejecting an unrelated mutation because an
otherwise ignored extension has a legal-JSON but nonnumeric value.

Already-owned queued reads ignore the new field and retain their exact legacy
classifier/frontier, including previously documented ambiguous replay risk.
Mutations ignore it, preserving first-attempt unknown custody, existing redial,
outbox ownership/retry and all known unresolved rejection/retirement debts.
Those are separate obligations, not fixes or approvals supplied here.

## Authored causal controls; NONE EXECUTED

The carried-forward four tests cover initial read/legacy/mutation isolation,
unchanged queued-read deadline behavior, too-short caller no-publication, and
legacy envelope decoding. Existing PR1375 tests remain unchanged.

New controls use testing/synctest, supported by the repo's Go1.26.2 baseline,
with real context/timer APIs and actual RoundTrip -> request JSON -> handleOne ->
forward -> response JSON -> RemoteStore.do body consumption. Only the network
edge is in-process; there are no sockets, processes, services, time.Sleep calls
or new database/test frameworks. Virtual time exercises original30s/25s bounds:

- Two dial-phase errors consume one cutoff (24s + remaining1s), return the
  unchanged503 error before caller expiry, and leave no asynchronous read work.
- First dial reaches the cutoff: no second attempt; explicit dial phase remains
  never-sent. Bare deadline and post-send read deadline instead retain UNKNOWN,
  one attempt, failed custody and no outbox; a403 containing sentinel text stays403.
- Queue delay exhausts an earlier10-second caller's forwarding cutoff: zero
  forwards, response consumed before caller expiry. Invalid/expired fields also
  cause zero attempts and no outbox or unknown-envelope discard.

These are authored deterministic private-spool controls, not passing tests,
native-network/fleet timing qualification or live fault recovery. A future
red control must use the SAME fixture with only production budgeting removed;
do not substitute the pre1375 baseline or label a compile failure a red result.
Normal review/qualification, exact-source integration, existing mutation/legacy
holds, signed v164 rollout and loaded-runtime/live-repair evidence remain owed.
Nothing substitutes for v124/152 or establishes a release as shipped.
