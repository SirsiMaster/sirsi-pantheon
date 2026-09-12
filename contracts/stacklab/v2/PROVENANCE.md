# Provenance: `wing.schema.json`

**Schema authority:** SNE (`sirsi-inference`). Pantheon vendors an exact,
hash-pinned consumption copy — this file is a reproducible dependency, **not**
a separately editable contract (per SNE's explicit instruction, router item
20260912-032526).

| Field | Value |
| :--- | :--- |
| Source path (on the authoring host) | `/Users/thekryptodragon/Development/sirsi-inference/contracts/stacklab/v2/wing.schema.json` |
| SHA-256 | `a69e0094b8ec4596c30b316fc8bd6ce5f806f5a801b5c80c10a972e3d86338ad` |
| Byte length | 3281 |
| Encoding | UTF-8, LF, final newline preserved |
| Delivered via | router result `20260912-032526-codex-inference-ra-response-ra-sne-w1-rs-32a-receipt-pr-745-rs-31a-wing-schema-` (base64, decoded and byte-verified against the stated hash before landing here — no normalization applied) |
| Originating request | `20260912-032252-ra-codex-inference-ra-sne-w1-rs-32a-receipt-pr-745-rs-31a-wing-schema-json-byte` |

## Rules for this file (per SNE)

- Do not normalize JSON, line endings, or trailing newline before checking the
  raw-byte hash.
- Fail closed on missing/mismatched bytes — never download or silently
  replace a mismatched contract at runtime. `TestVendoredWingSchemaMatchesPinnedHash`
  (`internal/routerstore/wingschema_test.go`) enforces this at test time.
- A future schema change requires an explicit newly-pinned SNE handoff and
  consumer review — never a local edit to this file.

## What this schema does — and does not — establish

Per SNE (router item 20260912-032526 and 20260912-015930): schema validation
establishes field **shape**. It does **not** establish caller/root authority,
enforce path containment, or authorize runtime admission by its mere
existence. rs-31a's admission/enforcement path (`sirsi router wing register
<file>`) must independently:

- bind the caller to independently-established project/repository authority
  (a self-declared `owner`/`repository_root`/`evidence_root` cannot establish
  it),
- check canonical path containment (unconditional default-deny outside the
  admitted repository/evidence root) and peer policy,
- persist the admitted record and its content/schema digests atomically, and
- reject a conflicting existing identity rather than silently overwriting it.

This commit lands the vendored, hash-verified schema bytes only — the
admission/enforcement verb (`sirsi router wing register`) is separate,
larger implementation work (rs-31a/b/c), not included here.
