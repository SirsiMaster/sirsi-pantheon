Add an explicit `stage.sh --stage-only` handoff for an existing router service.
It preserves the verified no-traffic revision and tag after health/readback checks;
promotion, rollback, traffic readback, and tag cleanup remain explicit operator actions.
First-service use and caller-supplied traffic, tag, or revision overrides are refused before building or deploying. The default deploy path is unchanged.

Refs: `scripts/router-service/stage.sh`, `scripts/router-service/staged-tag.test.sh`, `docs/router-service/RUNBOOK.md`, `docs/ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md`
