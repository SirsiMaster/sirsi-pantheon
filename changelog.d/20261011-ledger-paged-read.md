- **fix(router):** the full-ledger read no longer grows without bound. `sirsi router dump`, `scripts/bind/sirsi-bind.sh`
  and the Gemma triage script all failed on 2026-10-11 with "service response over 67108864 bytes": the ledger (17.7k
  closed items, ~3.7 KB each) had crossed the spool relay's 64 MiB response ceiling. Added a cursor-paged read
  (`Store.ListPage`, `ListPageMax`=2000) and `ListAllPaged`; `Facade.ListAll` now pages, so every full-history caller is
  fixed in one place. A page whose cursor does not advance is an error, never an endless loop. Tests: page walk equals
  `ListAll` for every page size, a strict-after-cursor bound, the wire path through `RemoteStore`, and a negative control
  (a `>=` cursor fails fast). **Deploy order:** the service must carry `ListPage` before clients that page are released.
