### Fixed

- Restore the previously qualified Gemma maintenance claim barrier on the current router APIs. Session-bound item claims and task claims share the same lane lock; existing lease renewal and settlement continue during a pause.
- Apply that barrier to fresh task-resume leases while preserving inspection and idempotent replay of an existing lease. Maintenance pauses cannot silently become new recovery claims.

This source change does not activate a live maintenance pause or replace a running worker. Matching controller, service, and client deployment remains an explicit operational transaction.
