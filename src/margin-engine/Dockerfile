FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o margin-engine ./cmd/margin-engine

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/margin-engine /usr/local/bin/margin-engine
USER appuser
EXPOSE 50053 8083
ENTRYPOINT ["margin-engine"]
