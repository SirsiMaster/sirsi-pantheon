- **feat(router):** `sirsi router token mint <host> --host-wide --secret-resource projects/<p>/secrets/<s>` publishes an
  UNSCOPED host token (a Mac's) to Secret Manager the same secure way scoped tokens already are: never printed or logged,
  revoked if delivery fails. Opt-in only: `--secret-resource` alone still requires `--agents`, `--host-wide` cannot be combined
  with `--agents`, and the host-wide entry point refuses a scoped token (and the default one refuses an unscoped token), each
  revoking the fresh token. Needed to rotate a Mac token whose plaintext had reached Cloud Logging via the job's stdout.
