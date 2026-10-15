- **fix(router):** brought main to parity with what production already runs (revision 00036, SSA build `ssa-e4fab409`).
  Round 3 of SSA's independent review of lane-scoped tokens: scoped thread writes never fall back to an unscoped write
  (malformed or unsupported arguments fail closed), `DeleteThreadCAS` gets the same atomic scoped write, lane-scope
  refusals map to HTTP 403, and an unknown-thread lookup failure refuses instead of allowing. Adds
  `sirsi router token mint --secret-resource projects/<p>/secrets/<s>`: the minted scoped token is published straight to
  Google Secret Manager and **never printed or logged** (the plaintext no longer reaches Cloud Logging); if delivery
  fails the fresh token is revoked. Requires `--agents`. Code authored by SSA (cac71b10/cb4d4e4e lineage plus the deployed
  round-3 tree); ported verbatim with its tests, plus the earlier thread-takeover test kept as an extra guard.
