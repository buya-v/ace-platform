# Build context is ./src (NOT ./src/securities-service) so the shared modules referenced by
# the relative `replace github.com/garudax-platform/... => ../shared/...`
# directives in go.mod are present in the build context. See docker-compose.yml
# (context: ./src, dockerfile: securities-service/Dockerfile). Building with a narrow
# ./src/securities-service context fails with "replacement directory ../shared/... does not
# exist" (R026).
FROM golang:1.25-alpine AS builder
# Modules declare `go 1.25.x`; allow the toolchain to be fetched if the base
# image's patch release is older than required.
ENV GOTOOLCHAIN=auto
WORKDIR /build

# Copy the shared zero-dep modules first (decimal, tenant) so the relative
# ../shared replace targets resolve, then the service module itself.
COPY shared ./shared
COPY securities-service ./securities-service

WORKDIR /build/securities-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/bin/securities-service ./cmd/securities-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/bin/securities-service /usr/local/bin/securities-service
USER appuser
EXPOSE 8089 9089
ENTRYPOINT ["securities-service"]
