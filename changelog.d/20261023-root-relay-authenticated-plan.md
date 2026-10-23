- Require the existing canonical actor/thread and exact-service grant check before
  inspecting the root router relay's read-only lifecycle plan. Keep apply
  authorization in the existing service controller and preserve read-only behavior
  for the five ordinary maintenance profiles. Command-local backend/authorizer
  injection covers allowed, denied and canceled preflight without live launchd.

Refs: docs/stacklab/pantheon-pt/canon/ROOT_RELAY_REFRESH.md;
PANTHEON_RULES.md A35; docs/QA_PLAN.md.
