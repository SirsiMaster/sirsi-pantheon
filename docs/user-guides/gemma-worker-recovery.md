# Gemma worker recovery

The resident Gemma worker consumes only `gemma-pantheon` router work through
the native `sirsi gemma worker` command. The LaunchAgent shell script is a
compatibility launcher, not a second queue implementation.

## What a healthy worker does

1. Checks the selected native SNE endpoint before claiming work.
2. Registers one consumer thread and records `SIRSI_THREAD_ID`.
3. Claims work with a bounded router lease, reads it completely, then records
   an acknowledgement.
4. Renews its lease and heartbeats its thread while generation is active.
5. Routes a **nonbinding draft** to the requester, then completes the original
   item only through the token-fenced completion operation.

## Recovery outcomes

| Condition | Worker behavior | Safe next action |
| --- | --- | --- |
| Native SNE unavailable | Exits before claiming work | Restore the approved SNE service, then restart the worker. |
| Empty or truncated draft | Reopens the request with retryable evidence | Inspect the retained reason; retry only after the endpoint is healthy. |
| Reply delivery fails | Keeps the original request unresolved and retains the exact draft | Repair router delivery; the next lease holder can retry. |
| Lease lost or superseded | Cancels generation and publishes nothing | Let the current lease holder continue; do not manually close the old item. |
| Duplicate worker | OS singleton refuses the second process | Keep the existing worker; investigate only if its registered thread is stale. |

The worker never starts a model or falls back to a Python/MLX command. It also
never treats a nonbinding draft as a completed or approved decision.
