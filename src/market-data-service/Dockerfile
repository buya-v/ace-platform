FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o market-data-service ./cmd/market-data-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/market-data-service /usr/local/bin/market-data-service
USER appuser
EXPOSE 50057 8087
ENTRYPOINT ["market-data-service"]
