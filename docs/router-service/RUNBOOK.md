# Router service runbook

Scope: operating the shared router service. Each step says whether it has been **rehearsed** (run end to end with
recorded evidence) or **documented only**.

## Run the service

```bash
SIRSI_ROUTER_SERVE_TOKEN=<token> sirsi router serve --store postgres://user@host:5432/db --listen :8080
sirsi router serve --store /path/router.db        # single-host SQLite, for small installs and tests
```

Self-hosted TLS: add `--tls-cert` and `--tls-key`. On Cloud Run, TLS is terminated by the platform and `$PORT` overrides `--listen`.

## Tokens (mint, list, revoke, rotate)

Run on the service host against the service's own backend. These verbs are never served over the wire.

```bash
sirsi router token mint <host> --label "<machine name>" --store <dsn-or-path>   # plaintext printed once
sirsi router token list --store <dsn-or-path>                                  # ids, hosts, labels, state
sirsi router token revoke <token-id> --store <dsn-or-path>                     # takes effect on that host's next request
```

- **Revoke**: kills the host's token and every session minted under that host. Rehearsed (ADR-062; unit test `TestHostTokenRevocationKillsItsSessionsOnNextRequest`).
- **Rotate** (documented only): mint a new token for the host, set `SIRSI_ROUTER_TOKEN` in the relay's environment, restart the relay, confirm `sirsi router status` works on that host, then revoke the old token id.

## Deploy (Cloud Run)

```bash
gcloud run deploy sirsi-router --source . --project <project> --region <region>
```

Deploy the service before any client that uses a new Store method. `scripts/release-train.sh <version> --deploy-service` does this in order and checks that the new revision holds 100% of traffic.

## Roll back

```bash
gcloud run revisions list --service sirsi-router --region <region> --project <project>
gcloud run services update-traffic sirsi-router --to-revisions <previous-revision>=100 --region <region> --project <project>
```

Documented only for Cloud Run. The node-side rollback (unset `SIRSI_ROUTER_URL`) is rehearsed and timed in `docs/evidence/ADR-062-RS20-CUTOVER-EVIDENCE-20260910.md`.

## Stand the fabric down and back up

```bash
sirsi router quarantine        # durable off switch: no dispatcher revives a label or spawns a consumer
sirsi router unquarantine
```

It does not stop running sessions; pair it with `sirsi router quarantine-worker` for that. It survives a supervisor restart.

## Danger

`sirsi router migrate-store` mirrors the source into the destination and **deletes** what the destination holds. Never point it at a live service.
