### Fixed
- Preserve exact sleep admission nonce, time window and custody generation through journal recovery and cleanup; reject substituted authorization fields.
- Persist native invocation/return separately and retain the recovery wake when an invoked operation has an ambiguous error. Reject duplicate/late callbacks and a custodian returning success without invocation.
- Add fake-adapter recovery, authorization-tampering and persistence-failure regressions. No native sleep authority, adapter, deployment or live qualification is added.
