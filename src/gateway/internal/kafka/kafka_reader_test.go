package kafka

import (
	"context"
	"testing"
)

func TestNewKafkaConsumer(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	cfg.Brokers = []string{"broker1:9092", "broker2:9092"}
	c := NewKafkaConsumer(cfg, nil)
	if c == nil {
		t.Fatal("NewKafkaConsumer returned nil")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestKafkaConsumerCloseIdempotent(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	c := NewKafkaConsumer(cfg, nil)
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestKafkaConsumerSubscribe(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	c := NewKafkaConsumer(cfg, nil)
	defer c.Close()

	called := false
	c.Subscribe("test-topic", func(ctx context.Context, event Event) error {
		called = true
		return nil
	})

	c.mu.Lock()
	if _, ok := c.handlers["test-topic"]; !ok {
		t.Error("handler not registered for test-topic")
	}
	c.mu.Unlock()

	// called is expected to be false since we haven't started
	if called {
		t.Error("handler should not have been called yet")
	}
}

func TestKafkaConsumerStartNoHandlers(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	c := NewKafkaConsumer(cfg, nil)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := c.Start(ctx)
	if err != nil {
		t.Fatalf("Start with no handlers and cancelled ctx: %v", err)
	}
}

// Consumer factory tests are in kafka_writer_test.go for this service
