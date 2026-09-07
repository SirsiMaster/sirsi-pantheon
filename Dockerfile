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
# Cloud Run expands $(VAR) in --args at deploy time; the DSN never appears in the image.
ENTRYPOINT ["/sirsi"]
CMD ["router","serve","--store","$(SIRSI_ROUTER_STORE)"]
