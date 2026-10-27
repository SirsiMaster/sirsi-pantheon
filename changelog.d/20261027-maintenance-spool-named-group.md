Legacy maintenance now resolves the relay service account and its named group independently, verifies explicit membership, and requires both controller spool access and successor startup intent to use that exact group identity. The successor resolves identity only when a startup intent exists, preserving ordinary no-marker startup. Preview continues to refuse lookup, membership, owner, or spool identity mismatches without changing permissions.

Refs: docs/stacklab/pantheon-pt/canon/LEGACY_RELAY_APP_OPERATION_FENCE.md
