# Owner router console actions

Classification: internal-tool. User/operational owner: Cylton and SSA/Ra. Pain: the owner can inspect messages but cannot respond or route them without a shell. Workflow: select an item, compose a reply, reassign with an explanation, or record a closing disposition. Value: accountable operations and reduced handoff loss; not a new priced tier or release qualification.

## Trust and design

The loopback browser console acts as `owner`, never a lane selected by browser input. Same-origin loopback Host/Origin/remote-address checks and a random process-local token protect writes against cross-site calls and DNS rebinding. The separate read-only CORS allowlist for Nexus cannot obtain this token. Existing local account processes remain trusted; this is not a remote/multi-user authentication scheme.

Reply creates an owner response addressed to the original requester and leaves the source open. Assign moves the same request with a note, preserving identity and history; the store's recipient/status/lease guard rejects competing assignment or claim. Close records a required owner disposition; it does not fabricate product completion, a review verdict or a release bind. The atomic status guard refuses claimed work. Item revisions reject a selection already changed when the server reads it. Revision validation is not a lock spanning that read and the subsequent store operation; the native store guards remain authoritative.

No browser shell commands, arbitrary file paths, alternate stores, caller-supplied actors or automatic retries. JSON input is limited to64KiB, message to32KiB, unknown fields and extra JSON objects rejected. Failed operations preserve the draft; timeout explicitly says the outcome is unconfirmed and requires readback before retry. Replies use the router's existing deduplication/quota policy, not a new delivery channel. Results are read back from the canonical store.

## Implementation and integration

- `dispatch.ActAsOwner`: narrow human action entry point shared by future CLI/app adapters; normal agent authorization is unchanged.
- `GET /api/router/v1/owner-session`: same-origin owner token; never returned to the read-only integration origin.
- `POST /api/router/v1/action`: closed typed request and fixed owner identity.
- `RouterActionFn`: dependency-injected operation, wired by the dashboard CLI to canonical service only.
- Queue message inspector: accessible action selector, assignment destination, composer, explicit submit result, draft retention when inspector closes, pending double-submit lock.

Ra was sent the owner's incorporation request as canonical item20261011-010432-sirsi-software-admin-ra-owner-requests-actionable-router-frontend-and-ra-incorporati. SSA implements and verifies the owner workflow; Ra owns the upstream integration/visual improvement handoff. This is not a request for another invented implementation approval.

## Verification and operation

Isolated facade tests cover reply persistence and routing, original stays open, assignment same identity/history, stale close refusal, closure audit, invalid and claimed item refusal. HTTP tests cover token, same-origin, read/write separation, rebinding, nonloopback, actor injection, oversized/extra JSON and media-type rejection. Existing queue tests cover totals/filtering and markup escaping. Live validation uses only explicitly marked temporary test items, retired with owner dispositions after readback; existing work is not acted on.

Deployment updates only the user-owned `ai.sirsi.router.surface` frontend process. The installed app, relay/service, authentication material, caches and Horus autonomy remain unchanged. Rollback: restore the preceding preserved frontend LaunchAgent path and reload that user agent; never open a retired SQLite router store.
