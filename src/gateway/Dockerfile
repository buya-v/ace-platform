FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway ./cmd/gateway

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/gateway /usr/local/bin/gateway
USER appuser
EXPOSE 8080
ENTRYPOINT ["gateway"]
