### Fixed

- **A lane can no longer disarm another lane's wake loop.** `sirsi thread watch --uninstall` accepted `--agent <any lane>`; on 2026-10-05 four M5 wake loops were removed in one second that way. It now acts only on the session's own resolved identity and refuses anything else, pointing at `sirsi router quarantine` as the owner's durable off switch. The fabric-on rule (never park, bootout or reap a wake lane) is now enforced at the one code path that deletes a wake plist.
