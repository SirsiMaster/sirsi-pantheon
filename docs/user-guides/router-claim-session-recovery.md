# Authenticated inbox claim ownership and recovery

An authenticated router claim becomes successful only when both its item lease
and the calling session's ownership are durably committed. A binding database
error returns failure without a new live claim or claim-counter increment.
After a definitive rollback, explicitly inspect the same item and retry its exact
claim when the database is healthy. A transport timeout is not proof of rollback:
preserve its original item/request identity rather than blindly claiming next work.

Keep the lease private to its actual registered worker. Another thread cannot
complete the claim merely by copying its token. A normal session refresh/remint
for the same authenticated host, agent and nonempty thread remains supported.
A new MCP instance has a different thread and does not inherit prior ownership.

Local legacy claims and the separately declared unclaimed CLI close route are
unchanged. MCP close remains lease-fenced; do not switch it to unclaimed close
to work around an ownership refusal. No new login, credential, database or router
is needed. This correction requires normal backend deployment; a source-level
test or client-only update is not installed canonical close proof.
