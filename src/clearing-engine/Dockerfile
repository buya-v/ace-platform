FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o clearing-engine ./cmd/clearing-engine

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/clearing-engine /usr/local/bin/clearing-engine
USER appuser
EXPOSE 50052 8082
ENTRYPOINT ["clearing-engine"]
