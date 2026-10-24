# Migrate one legacy router MCP entry to the managed engine

This corrective source adds a macOS-only, explicitly selected plan/apply route.
It is not installed-client qualification, discovery, a general config editor,
or a replacement for generic `sirsi mcp` setup. No real client/config has been
inspected or migrated by this source author.

Supported selections are Claude Code's owner-local `~/.claude.json` top-level
`mcpServers` or one exact `projects[project].mcpServers` map, and Cursor's
owner-local `~/.cursor/mcp.json` top-level `mcpServers`. These are supported
source adapters, not proof that an installed client consumes the selected file.
Select the actual client/file/server deliberately; there is no discovery scan.
Other Claude paths, JSONC, wrappers and arbitrary configuration paths are not
inferred or rewritten. Missing entries are refusals, not successful installation.

Run from the normally installed signed CLI, for example:

```sh
/usr/local/libexec/sirsi-pantheon/sirsi setup router-mcp plan \
  --client claude-code --config "$HOME/.claude.json" --server router
```

For project scope add `--project /exact/existing/project/key`. For Cursor use
`--client cursor --config "$HOME/.cursor/mcp.json"`, without `--project`.

Only the exact absolute owner-local `~/.local/bin/sirsi-router-mcp` with no
arguments (omitted `args` or an empty array) is eligible. The existing file must
be an owned, single-link, executable, non-symlink Go standalone with the known
`cmd/sirsi-router-mcp` build path. No PATH lookup, shell, wrapper execution or
hardcoded historical binary hash is used. Generic `sirsi ... mcp` entries and
custom variants remain unchanged/refused.
Legacy Go build metadata is a compatibility discriminator, not a cryptographic
provenance assertion about that old owner-managed executable.

The new command is the currently running canonical managed engine plus
`router-mcp`. Qualification reuses the existing accepted package paths and
strict Sirsi Team/identifier signature requirement. A development binary is
not an installation target. Preview reads selected files and signature/build
metadata only; it creates no parents, configs, backups, sessions or databases.
It returns a digest binding selection, raw config, retained identity/mode,
legacy payload and managed engine fingerprints. Environment/cwd values are
not printed. Selected command/args are the only edited byte spans; env, cwd,
other servers and unknown settings retain their original bytes.

Review the preview, then use the same selection and its exact digest:

```sh
/usr/local/libexec/sirsi-pantheon/sirsi setup router-mcp apply \
  --client claude-code --config "$HOME/.claude.json" --server router \
  --confirm REVIEWED_PLAN_DIGEST
```

Apply rechecks the source and both payload identities, creates a private
no-clobber backup and same-directory stage, then uses retained descriptors,
identity checks, swap, fsync and complete byte/EOF readback. The new config and
retained originals are mode 0600. An occupied backup or concurrent edit refuses;
refresh the preview rather than forcing it. The publication helper is shared
with the existing selected Horus retained-file transaction; this route performs
no launchd, service, authorization or credential operation.

This is the ordinary cooperative-owner filesystem scope, not kernel atomic CAS
against an uncooperating same-UID writer in the final check/swap interval.
Post-swap drift is reported as unverified/recovery-required, never as success.
Backups/staging are retained on failures and never automatically deleted or
blindly restored over a later writer. Error/result output identifies recovery
paths, not private config content. Review the selected current file and retained
originals before any separate recovery action; do not publish those secret-bearing
files in public evidence. An already migrated exact entry is a verified no-op.

Successful apply means **configuration migrated; client reconnect pending**.
It neither stops/restarts clients nor deletes the old executable. It does not
transplant old tokens/session caches. Normal signed delivery, actual selected
client consumption/reconnect, exact bare tool/resource surface, and bounded own
claim/completion/refusal evidence remain separate work. No full MCP/task or
all-client rollout completion follows from these source fixtures.
