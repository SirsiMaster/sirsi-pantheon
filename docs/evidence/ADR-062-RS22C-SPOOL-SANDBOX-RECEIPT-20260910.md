# ADR-062 step 20a.3 — `sirsi router status` from INSIDE a no-network codex sandbox through the spool (2026-09-10T03:45Z, M5)

Setup on the M5 (`Mac`): `~/.sirsi/build/sirsi-relay` built from `relay-spool` at 156f24f5 (same code as the reviewed head after the rebase); relay started by hand from a shell holding the host's `SIRSI_ROUTER_URL`/`SIRSI_ROUTER_TOKEN`: `sirsi-relay router relay serve --spool ~/.sirsi/relay`.

Lane: `codex exec -C ~/Development/sirsi-pantheon --sandbox workspace-write -c 'sandbox_workspace_write.writable_roots=["/Users/thekryptodragon/.sirsi/relay"]' --ephemeral --skip-git-repo-check` (no `network_access`). Prompt, verbatim: *Run exactly this shell command and paste raw stdout+stderr verbatim, nothing else:* `env -u SIRSI_ROUTER_TOKEN SIRSI_ROUTER_URL=spool:///Users/thekryptodragon/.sirsi/relay /Users/thekryptodragon/.sirsi/build/sirsi-relay router status; echo exit=$?`

Raw codex output (tail): the full `status` listing of the live ledger, ending
```
    • 20260909-020438-claude-io-codex-inference-fyi-two-cable-striping-landed-5-95-ms-thunderbolt-idle-ramp-  age=1d1h  → codex-inference
exit=0
```
Relay log for the run (codex executed the command three times; each run minted a session and listed):
```
relay: forwarded agent=Mac method=MintSession id=1789011952352-1-d744c7b0 status=200
relay: forwarded agent=Mac method=ListAll     id=1789011952504-2-d7d5fa41 status=200
relay: forwarded agent=Mac method=MintSession id=1789011959505-1-41c52823 status=200
relay: forwarded agent=Mac method=ListAll     id=1789011959657-2-41e263ea status=200
relay: forwarded agent=Mac method=MintSession id=1789011967593-1-9312b6ec status=200
relay: forwarded agent=Mac method=ListAll     id=1789011967746-2-01d841de status=200
```
`grep -c "$SIRSI_ROUTER_TOKEN" relay.log` = 0. Spool files left afterwards: 0.

What this proves: a codex lane with **no network at all** reads the router service through the spool with **no token in its environment**; the host token lives only in the relay process. What it does not prove: a claim/close mutation through the spool (that is the 20a.5 receipt, on the SSA lane, after the relay LaunchAgent is installed), and same-uid isolation (none, as the charter states). Session caching inside the sandbox is unavailable (`~/.sirsi/sessions` is not a writable root), so each run mints a session — acceptable for lanes; a lane that wants the cache declares that directory too.

## Full raw receipt (second run, exact head 1a054d20, 2026-09-10T03:55Z)
Raw files, unedited except the host token replaced by `<redacted>` wherever it might appear (count: 0 occurrences found), in
`docs/evidence/ADR-062-RS22C-SPOOL-SANDBOX-RECEIPT-20260910/`:
- `env.txt` — head, host, time, `codex-cli 0.153.4`, macOS 26.6.2, sha256 prefix of the built binary, token-in-log=0, spool-leftovers=0
- `invocation.txt` / `prompt.txt` — the exact `codex exec` command line and prompt
- `codex-full.txt` — the COMPLETE codex transcript; its header is codex's own statement of the effective sandbox:
  `sandbox: workspace-write [workdir, /tmp, $TMPDIR, /Users/thekryptodragon/.sirsi/relay]` (no network_access line, none configured)
- `relay-log.txt` — the relay's log for the run (MintSession/ListAll × 3, each with agent, method, id, status)
This is still author-reported: an independent verifier reruns `invocation.txt` on the M5 with the relay started as described and compares.
