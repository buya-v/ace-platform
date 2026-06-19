# GarudaX Platform Control Plane (platform-control)
# Tenant registry + platform-admin lifecycle API (create/suspend/activate/modify/decommission).
# Produces a static binary and runs it as a non-root user on a minimal Alpine base.
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Zero external dependencies (no go.sum); the whole package lives at the module root.
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o platform-control .

FROM alpine:3.19

# Non-root runtime user. BusyBox wget (bundled in alpine) backs the compose healthcheck.
RUN adduser -D -u 1000 appuser

COPY --from=builder /build/platform-control /usr/local/bin/platform-control

USER appuser

# 8096: platform-admin REST API    9096: health/readiness probes
EXPOSE 8096 9096

ENTRYPOINT ["platform-control"]
