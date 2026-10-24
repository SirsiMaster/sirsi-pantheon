### Fixed — exact selected Horus legacy signing provenance

- Accept the historical `sirsi-horus-ssa-20261012` signing identifier only for
  the selected shipped legacy wrapper at its exact OS account-owner home path.
  Retain strict Apple generic anchor and Team `9D382WV988` verification.
- Keep canonical identifier `sirsi` valid for existing legacy payloads, and
  mandatory for every protected replacement. No dynamic identifier trust,
  artifact/hash/plist/instance-pin change, service action or new authority.
- Add per-instance signature-argument, conversion/readback and refusal fixtures
  for wrong path/identifier/team, unsigned/tampered artifacts and plan drift.
  These are source fixtures; main owns actual admitted qualification.

Refs: existing Horus selected-refresh correction; HORUS_SELECTED_REFRESH.md;
AGENTS.md A35. No installed signature or rollout success claimed by this patch.
