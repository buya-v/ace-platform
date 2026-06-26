package kafka

import (
	"os"
	"testing"
)

// TestKafkaBrokersConfigured verifies the broker-presence predicate that gates
// the fail-fast behaviour of NewProducerFromEnv. Outside of tests, an unset/empty
// value triggers log.Fatal rather than a silent in-process fallback (R007).
func TestKafkaBrokersConfigured(t *testing.T) {
	cases := []struct {
		name  string
		set   bool
		value string
		want  bool
	}{
		{"unset", false, "", false},
		{"empty", true, "", false},
		{"whitespace", true, "   ", false},
		{"single", true, "kafka:9092", true},
		{"multiple", true, "k1:9092,k2:9092", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("KAFKA_BROKERS")
			if tc.set {
				os.Setenv("KAFKA_BROKERS", tc.value)
				defer os.Unsetenv("KAFKA_BROKERS")
			}
			if got := kafkaBrokersConfigured(); got != tc.want {
				t.Fatalf("kafkaBrokersConfigured()=%v, want %v", got, tc.want)
			}
		})
	}
}

// TestNewProducerFromEnv_FailFastFallbackUnderTest documents the R007 contract:
// with no brokers configured, NewProducerFromEnv would log.Fatal in a real
// (non-test) deployment, but under testing.Testing() it falls back to the
// in-process adapter (NewTradeProducer) so the test binary is not terminated.
func TestNewProducerFromEnv_FailFastFallbackUnderTest(t *testing.T) {
	if !testing.Testing() {
		t.Fatal("expected testing.Testing() to be true inside a test")
	}
	os.Unsetenv("KAFKA_BROKERS")
	p := NewProducerFromEnv()
	if _, ok := p.(*ChannelProducer); !ok {
		t.Fatalf("expected in-process *ChannelProducer under test, got %T", p)
	}
	p.Close()
}
