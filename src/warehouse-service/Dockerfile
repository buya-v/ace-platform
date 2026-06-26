# Build context is ./src (NOT ./src/warehouse-service) so the shared modules referenced by
# the relative `replace github.com/garudax-platform/... => ../shared/...`
# directives in go.mod are present in the build context. See docker-compose.yml
# (context: ./src, dockerfile: warehouse-service/Dockerfile). Building with a narrow
# ./src/warehouse-service context fails with "replacement directory ../shared/... does not
# exist" (R026).
FROM golang:1.25-alpine AS builder
# Modules declare `go 1.25.x`; allow the toolchain to be fetched if the base
# image's patch release is older than required.
ENV GOTOOLCHAIN=auto
WORKDIR /build

# Copy the shared zero-dep modules first (decimal, tenant) so the relative
# ../shared replace targets resolve, then the service module itself.
COPY shared ./shared
COPY warehouse-service ./warehouse-service

WORKDIR /build/warehouse-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/bin/warehouse-service ./cmd/warehouse-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/bin/warehouse-service /usr/local/bin/warehouse-service
USER appuser
EXPOSE 50058 8088
ENTRYPOINT ["warehouse-service"]
