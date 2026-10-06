### Added
- CI + pre-push gate refuse a non-release/* branch that edits `CHANGELOG.md`
  directly; the entry belongs in `changelog.d/` instead
  (`scripts/check-changelog-straggler.sh`). `release/*` branches (the
  release train) remain the only legitimate writer.
