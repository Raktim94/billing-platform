# billing-server — the HTTP API composition root (apps/server).
#
# No frontend embed yet: apps/web doesn't exist as of this Dockerfile
# (docs/TODO.md Stage 10). When it does, add a Node build stage here that
# builds the SPA and COPYs its output into the final image next to the Go
# binary, and have apps/server serve it as a static fallback route. Do not
# fake that step now.

FROM golang:1.27.1-bookworm AS build
WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0: pgx's default driver is pure Go, no libpq/cgo needed, so a
# fully static binary works on Alpine's musl runtime with zero native deps.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./apps/server

# Alpine, not distroless: HEALTHCHECK below needs *something* to make an
# HTTP request, and distroless images ship no shell/wget/curl at all —
# adding an HTTP-client dependency to the Go binary just to self-probe
# would be a bigger footprint than the ~1MB wget already in Alpine's
# busybox. ca-certificates is required — pgx dials Postgres over TLS in
# most managed/cloud deployments.
FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S billing && adduser -S billing -G billing
WORKDIR /app
COPY --from=build /out/server /app/server

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O- --no-check-certificate http://localhost:8080/health/live || exit 1

USER billing:billing
EXPOSE 8080

# Exec form (no shell) — required for SIGTERM to reach the Go process
# directly, matching apps/server's signal.NotifyContext(SIGINT, SIGTERM)
# graceful-shutdown handling instead of being swallowed by a shell wrapper.
ENTRYPOINT ["/app/server"]
