# Platform Control Plane (platform-service)
# Tenant registry, lifecycle provisioning, and the platform-admin API for GarudaX.
# Produces a static binary and runs it as a non-root user on a minimal Alpine base.
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Zero external dependencies (no go.sum); copy the module file then the sources.
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o platform-service ./cmd/platform-service

FROM alpine:3.19

# Non-root runtime user. BusyBox wget (bundled in alpine) backs the compose healthcheck.
RUN adduser -D -u 1000 appuser

COPY --from=builder /build/platform-service /usr/local/bin/platform-service

USER appuser

# 8095: platform-admin REST API    9095: health/readiness probes
EXPOSE 8095 9095

ENTRYPOINT ["platform-service"]
