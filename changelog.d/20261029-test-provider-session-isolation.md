### Fixed
- Keep synthetic CLI subprocess registrations independent of inherited Codex/Claude session aliases and explicit thread environment; test-only private canaries verify that registration does not rewrite unrelated fixture markers. Preserve explicit fixture inputs and existing production identity policy.
