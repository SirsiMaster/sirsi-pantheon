Require exact-ID CLI support for owned Ma’at reservation readback

The admitted-run supervisor now requests `maat status --id <full-id> --json`
for its own reservation and refuses malformed, missing, duplicated, or extra
rows. It does not fall back to a resource history query. Install the normal
signed CLI rollout with `status --id` support before using this recovery path.

Refs: `scripts/maat/README.md`, `docs/ADR-006-SELF-AWARE-RESOURCE-GOVERNANCE.md`
