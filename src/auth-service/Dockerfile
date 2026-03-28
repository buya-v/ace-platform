FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o auth-service ./cmd/auth-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/auth-service /usr/local/bin/auth-service
USER appuser
EXPOSE 50055 8085
ENTRYPOINT ["auth-service"]
