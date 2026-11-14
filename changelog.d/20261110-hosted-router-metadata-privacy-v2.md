- Restrict both hosted router snapshot endpoints to an explicit public metadata
  allowlist and fixed producer-failure text. Local operator payloads stay intact;
  request/blocker bodies, worker reports, attention commands/details and local
  paths do not cross this boundary. Preserve canonical Fabric public redaction.
  Keep provider-quota counts, label withheld request/release content explicitly,
  and expose blocker presence without inferring dependency resolution.

- Refs: ADR-062; Rule A11; hosted-board privacy correction, 2026-11-10;
  Singer v2 patch `f81c7a22654b1a1428a73a0de01e6e0019854c24014e3107f48ed28acefe41dd`,
  parent and Plato SOURCE_ACCEPT; normal source `c9a01de7c27bd2886c9e3e1422aea0177200e250`
  changes alignment spaces only in two Go files. Source integration only; Go and explicit UI
  qualification of this composition remain unrun, with hosted deployment held.
