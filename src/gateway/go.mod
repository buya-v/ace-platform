module github.com/garudax-platform/gateway

go 1.22.0

require (
	github.com/garudax-platform/decimal v0.0.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/garudax-platform/tenant v0.0.0
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/garudax-platform/decimal v0.0.0 => ../shared/pkg/types/decimal

replace github.com/garudax-platform/tenant v0.0.0 => ../shared/pkg/tenant
