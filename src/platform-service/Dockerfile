FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o platform-service ./cmd/platform-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/platform-service /usr/local/bin/platform-service
USER appuser
EXPOSE 8090 9090
ENTRYPOINT ["platform-service"]
