# sirsi router serve on Cloud Run (ADR-062 rs-15/16). Pure-Go (modernc sqlite, pgx): no cgo.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=${VERSION}" -o /sirsi ./cmd/sirsi

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /sirsi /sirsi
# The DSN arrives as the SIRSI_ROUTER_STORE secret env var (deploy.sh); serve reads it as the --store default.
# Cloud Run does NOT expand $(VAR) in args when VAR is secret-backed (first deploy 2026-09-07 ran with the literal).
ENTRYPOINT ["/sirsi"]
CMD ["router","serve"]
