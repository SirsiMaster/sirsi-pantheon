## Ma'at ADR070 source-import contract correction (next increment)

- Separate explicit `maat.adr070.source-screen`, schema v2, from historical v1
  and the free deterministic host/preflight policy. Source imports require a
  separate expected head and exact required JSON keys; duplicate, unknown,
  case-alias, null, malformed and oversized packets reject before journal writes.
- Retain per-finding evidence identity and declared evidence recipe/calibration
  identities. Floor evidence is explicitly declared-only, not executed proof.
  Failed/incomplete floor blocks first; valid unqualified imports escalate.
  Imported qualification claims reject, never silently downgrade or authorize.
- Exact R0–R8 policy fixtures use package-private inert qualified context only.
  No production qualification writer, gate activation, model/service call or
  binding approval is added. Authenticated calibration, independent outcome,
  drift and requalification lifecycle remain OPEN; this is not full ADR070 or
  Ma'at completion. Legacy v1 journal reads and host repair/preflight semantics
  remain unchanged. Source fixtures are authored, not executed at checkpoint.

Refs: docs/ADR-070-MAAT-STRUCTURED-VERDICT.md;
docs/qa/MAAT_SYSTEM_ONE_CATALOG.md; contracts/stacklab/maat-system-one-recipe-v1.json.
