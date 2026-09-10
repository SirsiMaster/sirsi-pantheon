# ADR-062 step 20a.1b — can a no-network codex lane WRITE the spool directory? (2026-09-10T03:26Z, M5)

Prompt to codex (verbatim): *Run exactly these shell commands and paste raw stdout+stderr verbatim, nothing else:* `sh -c 'echo hi > $HOME/.sirsi/relay/probe/from-$$.json && echo wrote=… || echo write-failed=$?; ls $HOME/.sirsi/relay/probe; echo cwd=$(pwd)'`. All runs `--sandbox workspace-write`, no `network_access`.

| Run | Lane cwd | Extra config | Raw result |
|---|---|---|---|
| A | `~/Development/sirsi-pantheon` (repo-rooted lane) | none | `sh: /Users/thekryptodragon/.sirsi/relay/probe/from-72507.json: Operation not permitted` / `write-failed=1` |
| B | same | `-c sandbox_workspace_write.writable_roots=["$HOME/.sirsi/relay"]` | `wrote=/Users/thekryptodragon/.sirsi/relay/probe/from-74291.json` |
| C | `~` (HOME-rooted lane, the SSA shape) | none | `wrote=/Users/thekryptodragon/.sirsi/relay/probe/from-75606.json` |

Host view afterwards: `from-74291.json`, `from-75606.json` present (3 bytes each). codex-cli 0.153.4, macOS 25.6.0, host `Mac`.

**Result:** workspace-write covers the lane's own root; `~/.sirsi/relay` is writable only when it is the cwd's ancestor (C) or is declared as a writable root (B). Therefore every Codex lane's consumer command declares the spool as a writable root — a file-scope grant with no network — and the spool lives at a fixed per-host path, not inside any repo.
