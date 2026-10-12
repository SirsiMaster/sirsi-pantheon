- **feat(router):** lane-scoped host tokens (schema v25, new table `host_token_scopes`). `sirsi router token mint <host> --agents a,b`
  mints a token confined to the listed agents: it may mint sessions for and act as only those agents through a deny-by-default
  lane API (`internal/routerstore/lanescope.go`). It can send to anyone but cannot send as another agent, read the whole ledger,
  read another lane's inbox or items, or register threads for other agents; every refusal is audited (`router audience`).
  Unscoped host tokens (every existing Mac token) behave exactly as before. This is the foundation for cloud-based lanes that
  message Mac lanes and each other through the one cloud ledger. Closes the gap where a host token placed in a cloud sandbox
  could act as any lane (including `ra`) and read every item. Tests with negative controls; verified end to end with the real
  CLI against a local service. **Production rollout order matters** (the Postgres open check requires an exact schema version):
  see `docs/router-service/LANE_SCOPED_TOKENS.md`. Pins updated: schema version 25, 18 tables in `scripts/check-pg-schema.sh`
  and `scripts/router-service/apply-schema-job.sh`.
