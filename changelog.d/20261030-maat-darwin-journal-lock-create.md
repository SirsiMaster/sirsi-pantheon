### Fixed — avoid Darwin's concurrent journal-lock first-create failure

- Create the decision-journal mutation lock relative to its retained parent
  descriptor with `O_CREAT|O_EXCL`; only `EEXIST` falls back to opening the
  exact existing leaf without creation. A bounded retry covers only a leaf
  disappearing between those operations. Creation `ENOENT` and other errors
  remain failures; pathname fallback is not used.
- Add per-call injected-open tests for exact flags, create refusal, existing
  lock fallback, bounded leaf disappearance, and symlink refusal. Existing
  nonblocking flock and descriptor ownership remain in place.
- The isolated 100-run race receipt on base `94a87488` recorded 98 failures
  and 2 passes with `open mutation lock: no such file or directory`. The
  matching Darwin `openat(O_CREAT)` without `O_EXCL` first-create failure is
  documented by the [primary Entire CLI source fix](https://github.com/entireio/cli/commit/9c18f31b23a62749b804718125b17c236420a28a).

Refs: `docs/stacklab/maat/canon/DESIGN.md`, `docs/stacklab/maat/canon/SECURITY_THREAT_MODEL.md`, ADR-009, PANTHEON_RULES.md A16; receipt `~/.sirsi/evidence/pantheon-freeze-zero-ledger-20261030/qualification-lock-94a87488-isolated-reproduction.jsonl`. Source only; no tests, build, or push performed for this correction.
