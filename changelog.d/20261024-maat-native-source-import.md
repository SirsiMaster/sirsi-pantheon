## Ma'at native v2 source-import workflow correction (next increment)

- Migrate the existing native file picker to explicit ADR070 v2: editable
  independent expected head, copyable typed format/example, read-only Go
  validation preview, then frozen file/head/raw-digest confirmation. The Go
  importer rechecks the same bounded bytes before writing; drift requires review.
- Use existing bounded capture, check completion/exit plus exact returned
  contract/version/head and unqualified state (never provider-name trust), and
  consume confirmation once before await. No decode/error/readback refresh can
  replay an import. Keep selected inputs editable after failure and retain the
  original operation receipt if the separate Casebook refresh fails.
- CLI and native are the only existing public source-import writers. MCP,
  terminal and dashboard remain read-only projections; deterministic host and
  release/credential preflight writers and historical v1 reads remain unchanged.
  No v2 downgrade, native exemption, model/service operation or gate activation.
  Authenticated calibration lifecycle remains OPEN. Fixtures are source-only,
  unrun; no installed or visual qualification claim.

Refs: docs/ADR-070-MAAT-STRUCTURED-VERDICT.md section 2.5;
docs/qa/MAAT_SYSTEM_ONE_CATALOG.md; contracts/stacklab/maat-system-one-recipe-v1.json.
