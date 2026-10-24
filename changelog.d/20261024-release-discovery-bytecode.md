### Fixed

- Discover private release drafts with an exact id/tag_name/draft projection on authenticated100-record list pages, preserving the ten-page/1000-record coverage and one-MiB projected-response limits. Selected-ID/tag metadata and assets remain full; complete absence, exact draft identity, publish-last readback and no-overwrite rules stay unchanged. Incomplete or oversized discovery still refuses.
- Invoke official package-mode, PackageInfo and publication Python helpers with explicit `-B`, preventing imported helper bytecode from dirtying release source before canonical Go cask checks.
- Add inert heavy-history/discovery refusal coverage and a real copied-helper fixture proving no bytecode side effects without an ambient environment bypass. Source fixtures are not live publication proof; SSA's existing134 recovery remains separate.
