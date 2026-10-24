### Fixed

- Discover private release drafts with twenty-record authenticated pages while retaining the one-MiB response and ten-page limits, complete-absence requirement, exact draft identity, publish-last readback and no-overwrite rules. Historical metadata can no longer crowd a hundred-record page unnecessarily; incomplete or oversized discovery still refuses.
- Invoke official package-mode, PackageInfo and publication Python helpers with explicit `-B`, preventing imported helper bytecode from dirtying release source before canonical Go cask checks.
- Add inert heavy-history/discovery refusal coverage and a real copied-helper fixture proving no bytecode side effects without an ambient environment bypass. Source fixtures are not live publication proof; SSA's existing134 recovery remains separate.
