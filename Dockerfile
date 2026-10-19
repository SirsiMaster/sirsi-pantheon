# NOT the router service build. `gcloud run deploy --source .` from the repo root lands here and stops at once.
# cmd/sirsi imports the private github.com/SirsiMaster/sirsi-inference module, and Cloud Build holds no
# credential for it (2026-10-18: the first deploy after #1109 failed). Deploy with
# scripts/router-service/stage.sh, which cross-compiles linux/amd64 where the module is reachable and ships a
# COPY-only image (distroless, ENTRYPOINT /sirsi, CMD router serve). The DSN arrives as the
# SIRSI_ROUTER_STORE secret env var (deploy.sh); serve reads it as the --store default.
FROM alpine:3
RUN echo "refusing: build the router service with scripts/router-service/stage.sh (private Go module; see Dockerfile header)" >&2 && exit 1
