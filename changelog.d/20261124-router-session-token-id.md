### Fixed
- A failed publication of a freshly minted host token no longer signs out every session on that host.
  `sirsi router token mint --secret-resource` cleaned up with `RevokeHostToken`, which revokes every active
  session on the token's host because sessions did not record the token that issued them. Sessions now carry
  `token_id` (SQLite schema v29, Postgres schema version 29), stamped by the service at mint from the bearer the
  request authenticated with (never from a client argument; a bootstrap-token, in-process or pre-v29 session is
  `''`). The publish-failure cleanup is the new server-side `DiscardMintedToken(id)`: one transaction marks that
  token revoked and revokes only the sessions whose `token_id` equals it. It refuses an empty id, is idempotent,
  keeps the token and scope rows (marked revoked, not deleted), and is not served over the wire or forwarded by the
  relay. A cleanup that itself fails is still reported as `CLEANUP_DEBT`. The operator command
  `sirsi router token revoke` (`RevokeHostToken`) is unchanged: it still revokes the host's every session.
  Deploy order: the router service and every `sirsi` binary that opens the Postgres ledger require schema version 29
  exactly, so apply the schema and roll the service and token job to the new image together; a SQLite store
  upgraded by a v29 binary is refused by older binaries. Sessions minted before v29 have no `token_id` and are
  never matched by a discard (a later rotation that must clear them uses the explicit revoke).

Refs: PANTHEON_RULES.md A35
