FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o securities-service ./cmd/securities-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/securities-service /usr/local/bin/securities-service
USER appuser
EXPOSE 8089 9089
ENTRYPOINT ["securities-service"]
