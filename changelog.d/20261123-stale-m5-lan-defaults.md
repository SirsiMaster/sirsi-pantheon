### Fixed
- `scripts/router-service/cutover-m5.sh` now requires an explicit `M5=<user>@<host>` instead of defaulting to a stale LAN address. `scripts/release-train.sh` drops its hardcoded LAN fallback; `M5_FALLBACK` is honored only when set explicitly.

Refs: `scripts/router-service/cutover-m5.sh`, `scripts/release-train.sh`, `scripts/maat/fabric-endpoints.py`
