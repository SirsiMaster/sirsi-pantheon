- Retain the exact GitHub draft ID returned by one authenticated creation,
  then verify that selected draft directly instead of rediscovering it through
  changing release-list pages. Preserve full bounded absence proof before
  creation, recheck the remote tag, require HTTP201 and exact private ID/tag
  fields, and retain unknown creation outcomes without another create/upload.
  Keep private paired-byte readback, Apple verification, publish-last ordering,
  public-byte readback and no-overwrite behavior. Inert regressions cover the
  observed post-create ambiguity class and malformed/unknown/drifting identity;
  actual service acceptance remains proof for the next normal release.

Refs: docs/stacklab/pantheon-pt/canon/PUBLISHER_CREATED_ID_CORRECTION_20261024.md;
docs/ADR-023-BINARY-VERSION-CONTRACT.md; docs/user-guides/release-publication.md.
