FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o compliance-service ./cmd/compliance-service

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /build/compliance-service /usr/local/bin/compliance-service
USER appuser
EXPOSE 50056 8086
ENTRYPOINT ["compliance-service"]
