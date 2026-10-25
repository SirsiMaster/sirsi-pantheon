# Own blocked-task recovery — released136 corrective plan

Classification: platform-foundation. Baseline: published v0.24.136,
eff2c45d9acd75a19e49cb1c4e68947786b4a4ad. This correction does not change
that release's signed artifacts or certify whole-product completion.

## Cause and goal

The real codex-pantheon/pantheon-maat-operational-integration row has two
attempts, no remaining dependency and no holder. Its obsolete blocked status
prevents ordinary claim; explicit cause-fixed recovery rejects it because it
only accepts exhausted attempts. Allow that original row to consume its next
ordinary attempt without resetting its history, inventing exhaustion or
creating a replacement task. ADR-061 transactional ownership remains binding.

## Design and decisions

Use the existing ResumeTaskClaim transaction and authenticated own-lane actor.
For attempts below the retry ceiling, accept only a blocked row. Pending and
in-progress rows retain ordinary claim semantics. Existing terminal/dependency,
live or malformed custody, expected-row-version CAS, principal revalidation,
audit, replay, lease fencing and attempt-increment checks remain unchanged.
The existing exhausted recovery path is preserved. No schema or credential
change, automatic retry, cross-lane recovery, completion or attempt refund.

Flow: inspect original -> attest corrected cause/evidence -> validate own
principal and row CAS -> validate blocked/inactive eligibility -> atomically
increment attempts, bind lease and record audit -> normal fenced work/closure.
Any failed check leaves task, wake and audit unchanged.

## Implementation and proof

1. In progress: eligibility correction plus isolated-store tests for attempts
   0/1/2, exact replay, preserved history and ordinary fenced completion.
2. Prove pending/in-progress below-ceiling, malformed attempts, dependencies,
   live/partial custody, foreign principals and stale versions remain refused;
   retain existing exhausted-path, audit rollback and contention tests.
3. Run same final positive fixture against released136 (must fail), then focused
   normal/race tests and protected integration; exact-head independent review.
4. Integrate via the next canonical release, package/catalog/tag/publish under
   the existing release train. Use the actual original Main row to verify the
   deployed correction. No fabricated task, retry-count reset or false DONE.

Release, installed readback and three-home publication remain unproven until
their actual receipts exist. Rollback is the retained136 baseline; no schema
migration is introduced. SSA owns implementation; Main verifies its real row.
