# ACE Platform — Matching Engine (Go)
# Build: docker build -t ace-matching-engine ./src/matching-engine

FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /matching-engine ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /matching-engine /matching-engine
EXPOSE 50051
CMD ["/matching-engine"]
