# Build context is ./src (NOT ./src/margin-engine) so the shared modules referenced by
# the relative `replace github.com/garudax-platform/... => ../shared/...`
# directives in go.mod are present in the build context. See docker-compose.yml
# (context: ./src, dockerfile: margin-engine/Dockerfile). Building with a narrow
# ./src/margin-engine context fails with "replacement directory ../shared/... does not
# exist" (R026).
FROM golang:1.25-alpine AS builder
# Modules declare `go 1.25.x`; allow the toolchain to be fetched if the base
# image's patch release is older than required.
ENV GOTOOLCHAIN=auto
WORKDIR /build

# Copy the shared zero-dep modules first (decimal, tenant) so the relative
# ../shared replace targets resolve, then the service module itself.
COPY shared ./shared
COPY margin-engine ./margin-engine

WORKDIR /build/margin-engine
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/bin/margin-engine ./cmd/margin-engine

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/bin/margin-engine /usr/local/bin/margin-engine
USER appuser
EXPOSE 50053 8083
ENTRYPOINT ["margin-engine"]
