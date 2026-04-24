FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o matching-engine ./cmd/matching-engine

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/matching-engine /usr/local/bin/matching-engine
USER appuser
EXPOSE 50051 8081
ENTRYPOINT ["matching-engine"]
