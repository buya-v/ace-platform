FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o settlement-engine ./cmd/settlement-engine

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/settlement-engine /usr/local/bin/settlement-engine
USER appuser
EXPOSE 50054 8084
ENTRYPOINT ["settlement-engine"]
