package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func makeTestRecord(t *testing.T, topic, key string) Record {
	t.Helper()
	evt, err := NewEvent(topic, "test-source", "corr-1", map[string]string{"data": "hello"})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, _ := json.Marshal(evt)
	return Record{Topic: topic, Key: key, Value: data}
}

func TestChannelConsumer_BasicConsume(t *testing.T) {
	dlqProducer := NewChannelProducer(DefaultProducerConfig())
	cfg := DefaultConsumerConfig("test-group")
	c := NewChannelConsumer(cfg, dlqProducer)

	src := make(chan Record, 10)
	c.AddSource("test-topic", src)

	var received int32
	c.Subscribe("test-topic", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&received, 1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Send records then close
	for i := 0; i < 5; i++ {
		src <- makeTestRecord(t, "test-topic", fmt.Sprintf("key-%d", i))
	}
	close(src)

	c.Start(ctx)
	cancel()

	if atomic.LoadInt32(&received) != 5 {
		t.Errorf("received = %d, want 5", atomic.LoadInt32(&received))
	}
	if c.ProcessedCount() != 5 {
		t.Errorf("processed count = %d, want 5", c.ProcessedCount())
	}
}

func TestChannelConsumer_Idempotency(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	c := NewChannelConsumer(cfg, nil)

	src := make(chan Record, 10)
	c.AddSource("test-topic", src)

	var count int32
	c.Subscribe("test-topic", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	// Send the same event twice
	evt, _ := NewEvent("test-topic", "src", "corr", nil)
	data, _ := json.Marshal(evt)
	rec := Record{Topic: "test-topic", Key: "k", Value: data}
	src <- rec
	src <- rec
	close(src)

	ctx := context.Background()
	c.Start(ctx)

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("handler called %d times, want 1 (idempotency)", atomic.LoadInt32(&count))
	}
}

func TestChannelConsumer_RetryAndDLQ(t *testing.T) {
	dlqProducer := NewChannelProducer(DefaultProducerConfig())
	dlqCh := dlqProducer.RegisterTopic("ace-commodities.dlq.test-topic", 10)

	cfg := DefaultConsumerConfig("test-group")
	cfg.MaxRetries = 2
	cfg.RetryBackoff = 0
	c := NewChannelConsumer(cfg, dlqProducer)

	src := make(chan Record, 10)
	c.AddSource("ace-commodities.test-topic", src)

	var attempts int32
	c.Subscribe("ace-commodities.test-topic", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&attempts, 1)
		return fmt.Errorf("always fails")
	})

	src <- makeTestRecord(t, "ace-commodities.test-topic", "k1")
	close(src)

	c.Start(context.Background())

	// 1 initial + 2 retries = 3 attempts
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
	}

	// Should be sent to DLQ
	select {
	case <-dlqCh:
		// ok
	default:
		t.Error("expected DLQ record")
	}
}

func TestChannelConsumer_UnmarshalError(t *testing.T) {
	dlqProducer := NewChannelProducer(DefaultProducerConfig())
	dlqProducer.RegisterTopic("ace-commodities.dlq.test-topic", 10)

	cfg := DefaultConsumerConfig("test-group")
	c := NewChannelConsumer(cfg, dlqProducer)

	src := make(chan Record, 10)
	c.AddSource("ace-commodities.test-topic", src)
	c.Subscribe("ace-commodities.test-topic", func(ctx context.Context, event Event) error {
		return nil
	})

	// Send invalid JSON
	src <- Record{Topic: "ace-commodities.test-topic", Key: "k", Value: []byte("not json")}
	close(src)

	c.Start(context.Background())
	// Should not panic, should send to DLQ
}

func TestChannelConsumer_ContextCancellation(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	c := NewChannelConsumer(cfg, nil)

	src := make(chan Record, 10)
	c.AddSource("test-topic", src)
	c.Subscribe("test-topic", func(ctx context.Context, event Event) error {
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Start(ctx)
	}()

	// Cancel after a short delay
	time.Sleep(10 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not stop on context cancellation")
	}
}

func TestChannelConsumer_EvictionOnFull(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	cfg.MaxTrackedIDs = 5
	c := NewChannelConsumer(cfg, nil)

	src := make(chan Record, 20)
	c.AddSource("test-topic", src)
	c.Subscribe("test-topic", func(ctx context.Context, event Event) error {
		return nil
	})

	// Send more events than MaxTrackedIDs
	for i := 0; i < 10; i++ {
		src <- makeTestRecord(t, "test-topic", fmt.Sprintf("key-%d", i))
	}
	close(src)

	c.Start(context.Background())

	// After eviction, processed count should be <= MaxTrackedIDs
	if c.ProcessedCount() > cfg.MaxTrackedIDs {
		t.Errorf("processed count %d exceeds max %d", c.ProcessedCount(), cfg.MaxTrackedIDs)
	}
}

func TestChannelConsumer_Close(t *testing.T) {
	cfg := DefaultConsumerConfig("test-group")
	c := NewChannelConsumer(cfg, nil)
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTopicWithoutPrefix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"ace-commodities.trades.executed", "trades.executed"},
		{"ace-commodities.clearing.novated", "clearing.novated"},
		{"ace-commodities.test-topic", "test-topic"},
		{"no-prefix", "no-prefix"},
		{"ac", "ac"},
	}
	for _, tt := range tests {
		got := topicWithoutPrefix(tt.input)
		if got != tt.want {
			t.Errorf("topicWithoutPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
