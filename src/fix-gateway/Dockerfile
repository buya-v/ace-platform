FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o fix-gateway ./cmd/fix-gateway

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/fix-gateway /usr/local/bin/fix-gateway
USER appuser
EXPOSE 8091 9091 9878
ENTRYPOINT ["fix-gateway"]
