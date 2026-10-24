# Gemma worker recovery

The resident Gemma worker consumes only `gemma-pantheon` router work through
the native `sirsi gemma worker` command. The LaunchAgent shell script is a
compatibility launcher, not a second queue implementation.

## What a healthy worker does

1. Reads the existing lead-owned `worker_answers` grant before provider
   preflight. Both ordinary drafting and Apollo tool-turn workers refuse an
   absent/unreadable grant; neither mode edits the registry or grants itself a role.
2. Checks the selected native SNE endpoint, then registers one consumer thread,
   records `SIRSI_THREAD_ID`, and verifies its answering-role stamp before claiming.
3. Claims work with a bounded router lease, reads it completely, then records
   an acknowledgement.
4. Renews its lease and heartbeats its thread while generation is active.
5. Routes a **nonbinding draft** to the requester, then completes the original
   item only through the token-fenced completion operation.

## Recovery outcomes

| Condition | Worker behavior | Safe next action |
| --- | --- | --- |
| Native SNE unavailable | Exits before claiming work | Restore the approved SNE service, then restart the worker. |
| Lead answering grant absent/unreadable | Exits before provider preflight, registration or claim | Have the lead review its existing declaration; do not auto-grant or borrow a thread. |
| HTTP 503 with `Retry-After` | Waits under the same live, renewed lease; bounded wait exhaustion records fenced retryable failure | Respect backpressure; do not restart or kill a BUSY broker. |
| Unknown HTTP 503 without `Retry-After` | Preserves bounded structured runtime-failure evidence; does not pretend BUSY | Diagnose the recorded runtime error before retrying. |
| Permanent context/capacity or authentication error | Blocks only the claimed item through its exact fence | Correct the selected profile/request/auth readiness; unblock deliberately. Never shorten the system identity to hide overflow. |
| Empty or truncated draft | Reopens the request with retryable evidence | Inspect the retained reason; retry only after the endpoint is healthy. |
| Reply delivery fails | Keeps the original request unresolved and retains the exact draft | Repair router delivery; the next lease holder can retry. |
| Service refuses reply completion for answering authority | Fenced-blocks the same original and preserves the generated draft for lead review, rather than retrying inference | Reconcile any already-sent correlated reply and the preserved draft under the existing original. Do not submit a fresh request or automatically reopen/regrant. |
| Lease lost or superseded | Cancels generation and publishes nothing | Let the current lease holder continue; do not manually close the old item. |
| Duplicate worker | OS singleton refuses the second process | Keep the existing worker; investigate only if its registered thread is stale. |

The worker never starts a model or falls back to a Python/MLX command. It also
never treats a nonbinding draft as a completed or approved decision.

If the authority block itself fails (for example, its lease fence was lost),
the worker reports both errors and the draft; it does not claim that preservation
in the store succeeded. This correction does not reopen previously dead-lettered
requests or introduce an automatic no-inference completion/recovery command.

When observing a worker on another host, `node-status` reports its kernel PID as
`unknown(remote-unknown)`. PID numbers are local to a machine: absence on M1 is
not evidence that an M5 worker died. Stable machine IDs take precedence over
mutable host labels; an id-less legacy row needs an exact observing-host match
before a local OS check. Foreign loop evidence is likewise unverified, not dead.
Use the canonical original/result, holder and heartbeat receipts for delivery
reconciliation; unknown remote OS state does not contradict a completed item.
Retain stale failure history alongside the fresh readback, and never restart a
foreign worker based on the observer's process table.

## Selected broker profile

## Shared producer (explicit host outbound)

From a genuine registered lane/session, using its existing canonical router and
current actor/thread environment:

```sh
sirsi gemma shared --wait 15m "Draft a pure Go Add function with four test cases"
sirsi gemma shared --resume ORIGINAL_ITEM_ID --wait 2m
```

`SIRSI_AGENT_ID` (or the current session actor) and `SIRSI_THREAD_ID` must match
an active thread on this machine whose anchor is in the current process ancestry.
There is no `--from` override. Prompts are UTF-8, at most 8192 bytes; oversize
input is refused rather than trimmed. The explicit shared command sends the full
prompt through the configured canonical HTTPS router or its existing relay and
may execute on M5. It does not mislabel this as local inference or open an SSH
forward, broker capability, new DB, model or consumer.

`--wait 0` submits and immediately returns Pending plus the original ID.
Wait is bounded to 0–15 minutes; cancellation preserves the original and prints
its resume ID. Resume never resubmits. Blocked/retryable outcomes retain their
canonical evidence. Only an exact completed original plus verified
`nonbinding_draft` reply reference is emitted as a completed draft. Replies
already closed by another legitimate reader remain readable by exact ID.
Legacy completion without a reference returns pending/manual retrieval.
No Inbox/ListAll scan, acknowledgement, claim or closure is performed by the
producer. The worker still sends before fenced completion; deduped publication
uses the returned reply ID and its actual stored one-way lease correlation.
Each real builder must submit under its own genuine identity; source fixtures
do not qualify every lane.

The selected JSON profile binds one existing loopback broker, its full model
identity and declared capabilities. It neither loads a model nor starts a worker:

```json
{
  "endpoint": "http://127.0.0.1:8477/v1",
  "model": "gemma-4-e2b-it-nvfp4-sne-v0",
  "context_tokens": 1024,
  "output_tokens": 256,
  "streaming": false,
  "access_token_file": "/Users/sirsimasterdev/.sirsi/apollo-m1-profile-20261013/capability"
}
```

This M1 example is a **candidate**, not an installed/qualified profile. Its pilot
failed memory admission before model load. M5's reported existing selection is
`mlx-community/gemma-4-12B-it-8bit`, context 4096, output 1024, nonstreaming, with
an empty `access_token_file`. Use measured values for the actually selected host;
do not infer capabilities from a model name or copy M5 defaults onto M1.

Select the JSON using `local_profile=/absolute/profile.json` in the existing
`~/.sirsi/orchestrator.conf`, preserving its other fields, or set
`SIRSI_LOCAL_PROFILE` to that path. The environment reference takes precedence.
An explicit worker `--inference-profile` overrides that shared selection for that
worker only; prefer the shared reference so consumers agree.

| Field/flag | Behavior |
| --- | --- |
| `endpoint`, `model` | Credential-free loopback URL and exact full model identity; no automatic alternate-model fallback. |
| `context_tokens`, `output_tokens`, `streaming` | Explicit declared values required for a selected profile. Missing/invalid values fail; unprofiled capability metadata remains unknown. Input/system identity is not trimmed; backend overflow is surfaced. |
| `access_token_file` | Optional existing regular 0600 file; required by the M1 worker profile. The client builds the Bearer header in memory. Missing, empty, public-mode or invalid configured files fail closed. Never put a capability value in JSON, shell arguments or logs. |
| `request_timeout_seconds` | Optional integer 1–600. Applies to the actual worker and provider HTTP requests; zero/negative/oversized values fail. Missing values preserve existing defaults: worker completion five minutes, provider client 180 seconds (or its injected client). The worker also caps its request to the remaining generation-context deadline. |
| `gemma worker --profile m1` | Requires selected 1024-context metadata and capability reference; output capped at 256. |
| `gemma worker --profile m5` | Output capped at 1024; preserves existing unauthenticated loopback when selected. |
| Worker `--max-tokens N` | Worker-local override, bounded by the worker profile and selected JSON output cap; the parent `gemma --max-tokens` flag is not inherited. |
| Worker `--access-token-file PATH` | Existing private capability reference; a conflicting selected-profile path is refused. |

Provider-backed Ask, local routing, engine connectors and the native worker use
these shared fields. Engine fixture receipts are not standalone MCP/native
deployment proof; separately qualify each installed consumer before release.

For example, adding `"request_timeout_seconds": 600` to a qualified selected
profile permits a longer client wait without exceeding the worker's current
generation envelope (ten minutes under its fifteen-minute lease). This is a
configuration example, not a native rollout or a causal fix for a slow broker.
Broker write timeouts, generation budgets and buffered cancellation are separate
Apollo-owned constraints; do not attribute a header timeout solely to output
budget. The complete system identity and user input remain unchanged.

On request timeout or context cancellation the worker records fenced retryable
failure for the **same original item and lease**; it publishes no partial answer
and does not permanently block a temporary timeout. Existing durable retry work
must continue under its supported lease lifecycle, not a new throwaway proof.
The worker renews while the request is alive. Missing the configured deadline
does not authorize a native restart, a queue reset or a second consumer.

## Selection, handover and rollback

### Shared request recovery

`sirsi gemma shared --request-key <32-lowercase-hex-characters> "prompt"`
prints the request key, exact search title and local receipt location before it
sends. Omitting the key generates one. Receipts under
`~/.sirsi/gemma-shared-attempts/` contain only key, actor, prompt digest, title,
timestamp, state and any returned original ID, not prompt text; they are created
mode0600 in a mode0700 directory. The configured relay still stages the request
payload locally when applicable; the canonical router receives it and the draft
may execute on M5 or another host. This is not an on-device-only workflow.

A key is single-use locally. Reusing it never sends again, even if the previous
attempt timed out, was held by the relay, was canceled, or happened in another
hour. A changed prompt or actor with the same key is refused. If an original ID
is known, use `sirsi gemma shared --resume <original-id>` to read only that owned
original and its exact correlated reply. Otherwise preserve the receipt and
manually reconcile the printed exact title with the canonical router. No inbox
scan, automatic resend, permanent server deduplication or exactly-once guarantee
is claimed. Do not create a fresh key merely to retry an uncertain request.

This receipt fence is per user home and host, not globally idempotent across
different Macs or receipt directories. Keep an uncertain attempt on its original
host for reconciliation; do not use another host as a way to resend it.

Caller validation uses the existing registered runtime, exact actor/thread and
machine identity, and kernel process start signature. Codex Desktop uses the
same unique code-mode-host resolver as thread registration; a shared broker alone
does not establish a task host. Missing identity or ambiguous hosts refuse rather
than falling back to an arbitrary supplied PID or a fabricated session marker.

Keep receipts while an outcome is uncertain. Corrupt, substituted, symlinked or
wrong-mode receipts refuse submission; no automatic cleanup deletes them.

A completed legacy original with no valid exact reply reference returns
`correlation_unavailable` immediately, even when `--wait` is nonzero. It is not a
successful draft readback and does not warrant another submission.

1. Preserve the prior configuration, profile, signed client and broker receipt.
   Stage the new JSON separately; verify exact source/build and fresh host
   admission through the existing native custodian. Profile fixtures do not prove
   GPU capacity or a successful model load.
2. Keep one canonical `gemma-pantheon` consumer. A requester on M1 may send an
   ordinary canonical router item to the existing M5 lane with its real identity;
   do not create an SSH forward, a second queue or simultaneous global consumers.
3. Change queue placement only through an explicit fenced single-owner handover
   after provider/client/requester qualification. A running old broker remains
   authoritative until the supported native replacement succeeds.
4. On failed admission/readiness, restore the saved selection and qualified
   broker/client through the existing custodian, without overlapping processes.
   Preserve failed-request bodies and lease history. Do not retry a consumed
   admission window, widen credentials, force memory release or disable security.
5. BUSY alone never authorizes a broker restart. The worker's bounded BUSY timeout
   uses retryable failure, not permanent blocking. Only repair-backed native
   custody may unblock a permanent error; no stale lease reset is implied.

This follows [ADR-062 router concurrency/fencing](../ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md),
[ADR-040 running-host safety](../ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md) and
the [engine-selection guide](PANTHEON-ENGINE-SELECTION.md). A nonbinding draft,
profile handoff or routing subtask must not close an unfinished implementation or
offline-window obligation.
