# Pantheon worker control from an M1 client

Pantheon keeps one worker/router authority: the authenticated M5 control
endpoint. An M1 client can inspect that live state and submit closed worker
actions to M5; it does not maintain a replica registry or use an unrestricted
SSH shell as its control interface.

## Configure the client

Use the endpoint and bearer token provisioned for the authorized client
session. Keep the token in the protected process environment; do not put it in
a URL, request JSON, command-line argument, shell history, or shared log.
Remote endpoints must use HTTPS. Plain HTTP is accepted only for loopback.

## M5 transport boundary

The canonical `sirsi board-serve` listener binds to `127.0.0.1:8734`; the
dashboard listener is loopback-only as well. An M1 client cannot connect to an
M5 Tailscale address and reach either listener directly. Cross-host operation
requires an independently authorized, tailnet-only HTTPS bridge to the M5
loopback endpoint. The bridge must forward the bearer `Authorization` header
and preserve the backend's authentication. If it targets the dashboard's
`/api/control` endpoint, it must also send an upstream `Host` accepted by the
loopback guard. Do not widen the Pantheon listener to a wildcard address to
bypass this boundary.

Until that bridge is provisioned and its exact endpoint is verified, remote
worker control is unavailable; do not report the client as connected or imply
that a local fleet view is M5 state. Provisioning a bridge or changing
Tailscale access is outside this guide and requires its own authorization.

```sh
export SIRSI_CONTROL_ENDPOINT='https://<authorized-m5-host>:<port>'
# Supply SIRSI_CONTROL_TOKEN through the approved protected environment.
```

Without an endpoint, the command fails closed. It never silently opens a local
router registry. `--client-only` can make the M1 intent explicit; the
`SIRSI_CONTROL_CLIENT_ONLY=true` environment setting is also supported.

## Inspect canonical state

```sh
sirsi router control --client-only
# Or specify the endpoint explicitly:
sirsi router control --client-only --endpoint "$SIRSI_CONTROL_ENDPOINT"
```

The response is one validated worker-control envelope from the canonical
router store. A missing endpoint, token, or valid response is an error; the
client does not silently substitute local state. Completed task result
references remain visible in the canonical evidence projection, so an operator
can verify a committed result after reconnecting.

In the dashboard, both the canonical snapshot and action routes require the
local Pantheon capability. The page receives it only as a session-only,
HttpOnly cookie; the M5 bearer token remains in the server-side client and is
never sent to the browser.

On the host that owns the canonical router state only, local inspection must
be explicitly requested:

```sh
sirsi router control --local-authority
```

## Submit an action

`sirsi router control-action` accepts one JSON object from stdin or a request
file. The client validates the exact field names, verb-specific schema, and
request size before sending; M5 validates again before applying it. Successful
and rejected actions return receipts bound to the exact request bytes.
When M5 rejects a valid action, stdout contains the validated failure receipt
and the command still returns a non-zero result. Transport or response-
validation failures do not carry a canonical M5 receipt.

These examples perform real mutations when submitted. Replace every
placeholder with authorized values and review the request before sending.

Send a message or a review request:

```sh
printf '%s\n' '{"verb":"message","from":"<sender>","to":"<recipient>","title":"<title>","instructions":"<message>"}' \
  | sirsi router control-action --request-file -

printf '%s\n' '{"verb":"review_request","from":"<sender>","to":"<reviewer>","title":"<review title>","instructions":"<review scope>"}' \
  | sirsi router control-action --request-file -
```

Create a task, then claim either that task or the next available task:

```sh
printf '%s\n' '{"verb":"delegate","agent":"<agent>","task_id":"<unique-task-id>","subject":"<task subject>","phase":"queued","responsible_party":"<owner>"}' \
  | sirsi router control-action --request-file -

printf '%s\n' '{"verb":"claim","agent":"<agent>","worker":"<worker-id>","thread_id":"<thread-id>","task_id":"<task-id>","ttl_seconds":600}' \
  | sirsi router control-action --request-file -
```

Omit `task_id` to claim the next eligible task. The response includes the
canonical lease token; keep it protected and use it only for that task’s
completion or handback. The default lease is 10 minutes; the maximum is 24
hours. If a request times out after M5 commits the claim, retry it with the
same agent, worker, thread, and task selection: while that lease is active,
M5 returns the existing lease instead of claiming another task. Keep the
returned lease token; changing worker or thread creates a different claim
identity and may select a different task, so it cannot recover the uncertain
response.

Return a result or release a lease with a reason:

```sh
printf '%s\n' '{"verb":"result_return","agent":"<agent>","task_id":"<task-id>","lease_token":"<lease-token>","result_ref":"<durable-result-reference>"}' \
  | sirsi router control-action --request-file -

printf '%s\n' '{"verb":"cancel_handback","agent":"<agent>","task_id":"<task-id>","lease_token":"<lease-token>","reason":"<why the task is being returned>"}' \
  | sirsi router control-action --request-file -
```

`result_ref` should identify durable evidence without embedding credentials or
private content in the router ledger. A lease token is required for both
result return and handback. The server rejects stale or mismatched leases.

## Read the response

Check `authority`, `verb`, and the returned item/task identifier. For claims,
verify that the lease names the requested agent, worker, thread, and task
before doing task work. For every action, retain the request-bound response
with the resulting work record according to the team’s evidence policy. An
HTTP success alone is not proof that an unrelated or replayed result belongs
to the request.

The client never runs a shell command on M5, follows endpoint redirects, or
creates local worker/task state. Do not use it to bypass normal authorization
or the task owner’s handback requirements.
