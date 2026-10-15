# Apollo cluster client status

Apollo cluster chat is under development. Remote commands currently refuse
before network access because enrolled authentication and allowed endpoint/session
binding are not installed. An HTTPS URL does not authenticate your account or
make an arbitrary destination safe for private conversation text.

Offline `sirsi apollo catalog`, `plan` and `telemetry` retain their existing roles.
The single-host service is separate and never substitutes for a cluster request.

The intended chat workflow sends `--prompt` or `--stdin` text to Apollo-owned
preparation, then streams that prepared job. The client does not select its own
tokenizer/template or accept manually supplied token IDs. Plain and Flash remain
distinct; the current service advertises Plain only. This source flow has fixture
coverage and remains unavailable until actual authenticated installation proof.

Future status, replay and cancellation commands require the original
`--job-receipt admission.json` and the same `--job-id`. Replay also takes `--after`
with the last accepted sequence. A changed execution is an error. An interrupted
stream reports its last sequence; partial output is never reported as completed.
An expired original receipt can still be used to reconcile status or cancellation;
it does not renew permission to compute. A receipt file alone is not authentication.

Do not supply router credentials, private keys or workaround tokens. Signing,
installation, authenticated normal-client success, unauthorized/foreign-endpoint
refusal, safe cancellation, private receipt/cursor persistence and actual useful
streamed conversation remain prerequisites to availability.
