# Ma'at local liveness-watch repair integration (source candidate)

- Bind the existing closed Ma'at repair to the canonical running engine and
  ordinary HOME through the shared local plist, installer and inspector APIs;
  remove its developer-checkout dependency without changing legacy routed run.
- Preserve ordinary owned 0755 `.sirsi`/`maat` ancestors without chmod or
  sentinel changes, while keeping the actual failure-memory root private;
  recheck the complete EDA snapshot under the attempt-admission lock.
- Route repair review through the bounded local inspector, including exact
  legacy/unloaded metadata findings while refusing disabled or unknown state;
  retain historical projection separately from a fresh current observation.
- Make `liveness-watch status` require the exact local configuration and loaded
  state rather than plist-file presence alone. Source-only; qualification and
  installed behavior remain unclaimed, and the other seven Ma'at operational
  rows remain open.
- Keep standalone persistent installation on the same packaged-engine
  resolver/verifier, and accept bounded signed `launchctl list` status values
  while continuing to refuse malformed tables.
- Correct the default-store existence probe to use the actual one-result
  `unix.Fstatat` contract without changing its ENOENT/refusal checks.
- Explicitly set liveness `OpenStore` test roots to 0700; preserve the
  production private-root validator unchanged.
- Assert the native read-only probe sequence including both independent
  `launchctl print-disabled` observations; forbidden operations still fail.
- Validate HOME ownership from a no-follow native directory descriptor rather
  than asserting an `os.Lstat` platform `Sys` value is `unix.Stat_t`.
