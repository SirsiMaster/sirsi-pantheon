# Apollo cluster client status

Apollo cluster chat is under development. The current CLI refuses cluster
commands because enrolled authentication and verified plan integration are not
installed. An HTTPS URL does not authenticate your account. Do not supply router
credentials, tokens or private keys to work around this refusal.

Offline `sirsi apollo catalog`, `plan` and `telemetry` keep their existing roles.
The existing single-host service is separate and is never substituted for a
requested cluster job.

The provisional cluster interface takes pre-tokenized input and manual immutable
model, tokenizer, template, plan and session fields. It does not yet provide the
normal text-to-model/routing-to-streaming conversation workflow. Apollo must
prepare that text through its authoritative tokenizer/template contract.

For qualified future event replay, retain the original admission receipt with
the last accepted sequence in your private conversation storage. The planned
`events --job-id JOB --job-receipt admission.json --after N` interface verifies
every event against the same job, execution, request, plan and epoch before
printing token text. A changed execution is an error, not a resumed conversation.
The receipt file itself is not proof of service authentication.

Signing, installation, authenticated normal-client success, foreign-endpoint and
unauthorized refusal, exact frozen-plan verification and real reply/cancellation
proof remain required before this client can be advertised as available.
