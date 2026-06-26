package kafka

// Enabled reports whether cross-service Kafka wiring should be activated for
// this process. It is true exactly when KAFKA_BROKERS is configured. The engine
// composition root (cmd/*/main.go via internal/eventbus) uses this to decide
// whether to wire the real wire-protocol Producer/Consumer: when brokers are
// configured the real adapter is used; when they are not (pure unit/dev runs)
// Kafka wiring is skipped entirely so the engine still serves gRPC. This does
// NOT weaken the R007 fail-fast — NewProducerFromEnv/NewConsumerFromEnv still
// log.Fatal if called without brokers outside tests; eventbus simply does not
// call them when Enabled() is false.
func Enabled() bool { return kafkaBrokersConfigured() }
