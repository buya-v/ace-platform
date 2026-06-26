package kafka

import (
	"os"
	"testing"
)

// TestKafkaBrokersConfigured verifies the broker-presence predicate that gates
// the fail-fast behaviour of NewConsumerFromEnv. Outside of tests, an unset/empty
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

// TestNewInProcessConsumer verifies the explicit test-only constructor returns a
// usable in-process channel consumer. This adapter is reachable from production
// code only via NewConsumerFromEnv under testing.Testing().
func TestNewInProcessConsumer(t *testing.T) {
	c := newInProcessConsumer("test-group", nil)
	if c == nil {
		t.Fatal("newInProcessConsumer returned nil")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestNewConsumerFromEnv_FailFastFallbackUnderTest documents the R007 contract:
// with no brokers configured, NewConsumerFromEnv would log.Fatal in a real
// (non-test) deployment, but under testing.Testing() it falls back to the
// in-process adapter so the test binary is not terminated.
func TestNewConsumerFromEnv_FailFastFallbackUnderTest(t *testing.T) {
	if !testing.Testing() {
		t.Fatal("expected testing.Testing() to be true inside a test")
	}
	os.Unsetenv("KAFKA_BROKERS")
	c := NewConsumerFromEnv(nil)
	if _, ok := c.(*ChannelConsumer); !ok {
		t.Fatalf("expected in-process *ChannelConsumer under test, got %T", c)
	}
	c.Close()
}
