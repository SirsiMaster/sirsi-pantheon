- **fix(router):** `codex-apollo` is now declared as one resident, attended identity (cwd/repo
  `/Users/thekryptodragon/Development/Apollo`, `consumer.mode=resident`, health check
  `sirsi thread attended codex-apollo`, `wake.mechanism=none`). The previous record spawned an
  `--ephemeral` `codex exec` from a contributor checkout that rewrote Apollo's task ledger with
  retired blockers. Aliases `codex-inference` and `codex-sne-runtime` still resolve to
  `codex-apollo`. Refs: codex-apollo item 20261011-125339.
