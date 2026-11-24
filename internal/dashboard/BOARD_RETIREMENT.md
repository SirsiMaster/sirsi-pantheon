# Reviewed owner retirement

`board-retire.js` supplies input validation and the sequential workflow used by
the IAP-authenticated control in `board-actions.js`. No new backend capability,
read endpoint, token, IAM policy or task enumeration is introduced. The shared
hosted board continues to withhold private task rows.

For each reviewed ID, the control calls existing owner `cancel-task` once.
The final store UPDATE requires a nonterminal status and empty lease token,
expiry, worker and thread fields. The predicate is evaluated atomically with
the terminal transition; even expired or partial custody is refused until
the original lane reconciles it. Records are never reopened or relabeled as
preparation. A successful cancellation creates no completion receipt. The
batch is sequential, not an all-or-nothing transaction.

Each request is checked by the existing IAP/origin/header guard. Any non-success,
including generic 422, stops as unknown: backend errors may follow a commit.
The control does not automatically replay. Stopping preserves acknowledged
outcomes; reconcile all unknown and unprocessed IDs against the
canonical registry. Results are page-local, not durable receipts. Closing or
reloading the page loses that display but not the canonical changes.

Run `node --test internal/dashboard/board_retire_ui_test.cjs`. Its six synthetic
tests cover validation, exact cancellation identities without reopening,
unconfirmed/ambiguous responses, and stopping. They do not replace authenticated
UI qualification, store transaction tests, or post-action canonical readback.
Deploy the guarded store before exposing this control: old servers do not
establish the new custody protection. Mixed or old revisions must be drained.
