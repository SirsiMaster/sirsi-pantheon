### Fixed

- Native Ma’at supervision retains only already-proven lease time during transient heartbeat transport failures, reads back the exact reservation before renewal, and stops/reaps at its conservative monotonic deadline. Explicit denial and changed scope/executor still stop immediately. Deterministic recovery, uncertainty, expiry and cleanup fixtures added. Refs: ADR-062, ADR-009.

- Bound healthy reservation inventory reads/renewal to a30-second cadence shortened by remaining TTL; keep two-second local child monitoring, fast bounded uncertainty readback and exact terminal acceptance. Deterministic95-second fixture proves six reads total and three renewals.
