FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o warehouse-service ./cmd/warehouse-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/warehouse-service /usr/local/bin/warehouse-service
USER appuser
EXPOSE 50058 8088
ENTRYPOINT ["warehouse-service"]
