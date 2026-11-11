# Display guard: builder, artifact, installer and rollback custody

Answers SHA 20261103-005303 and SSA 20261107-022411 (task
`ra-display-guard-install-custody-20261109`). On 11-03 Ra reported these as
missing. This record names who does each step. It does not build, sign, install
or roll anything back.

The guard is `sirsi-display-power-guard` (source `cmd/sirsi-pretoolguard`), a
pre-tool hook at three targets. The copies there today (sha256 `e08b331e…`)
come from a dirty build of `0ed15ac9` and have no release provenance. They stay
in place as evidence until a signed replacement is installed.

| Step | Custody |
|---|---|
| **Builder** | `release.yml` → `scripts/build-release-pretoolguard.sh` (plus `build-release-pretoolguard-controller.sh` for the installer), run only by SSA as the single publisher. |
| **Signed artifact** | The pretoolguard DMG in the one owner-approved **v0.24.164** set ([v164-forward-route-decision.md](v164-forward-route-decision.md)). It carries the guard and `sirsi-pretoolguard-install`, both signed under team `9D382WV988`. **Unreleased as of 2026-11-10**: there is no signed guard artifact yet, so nothing is installed before v0.24.164 is published and its artifacts are verified. |
| **Installer, per target** | `sirsi-pretoolguard-install -action install` with an exact selector. It verifies the artifact's SHA-256 and `codesign --strict` against the expected identifier and team, then swaps the file transactionally and keeps the original. |
| — `codex-user`, `claude-user` (M5 `~/.codex/hooks`, `~/.claude/hooks`) | Run as that user, by SSA in the post-publication install hand-off (SSA decision 070443 #7), with Ra's fresh admission. |
| — `codex-system` (`/etc/codex/hooks`, root) | Requires real root; the installer never elevates. Run by the host administrator (the owner) or under an owner-granted root session, in the same hand-off. |
| **Rollback owner** | Whoever ran that target's install owns its rollback: `-action rollback -transaction <id> -expected-current … -expected-backup …`. The original inode, the e08 copy, is kept in the transaction directory, so rollback swaps it back unchanged. After a crash, the same operator runs `-action recover`; the installer never guesses an outcome. |
| **Receipt** | The install's journal record (installed identity plus backup identity), read back. It is attached to the hand-off item as the per-target receipt. |

What stays open, owned by the steps above:
- v0.24.164 publication and artifact verification (SSA).
- The installer's live qualification on the target OS and filesystem: its own
  README says the ACL query, provenance reads and swap durability are untested
  on real targets, so the first install on each target is also its
  qualification run.
- The emitter's input budget. The 64 KiB reader limit is not an approved
  contract (SSA 022411); see `ra-display-emitter-proof-20261031-ix254`.
