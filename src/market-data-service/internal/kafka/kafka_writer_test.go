package kafka

import (
	"os"
	"testing"
)

func TestNewKafkaProducer(t *testing.T) {
	cfg := DefaultProducerConfig()
	cfg.Brokers = []string{"broker1:9092", "broker2:9092"}
	p := NewKafkaProducer(cfg)
	if p == nil {
		t.Fatal("NewKafkaProducer returned nil")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestKafkaProducerCloseIdempotent(t *testing.T) {
	cfg := DefaultProducerConfig()
	p := NewKafkaProducer(cfg)
	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestKafkaProducerPublishAfterClose(t *testing.T) {
	cfg := DefaultProducerConfig()
	p := NewKafkaProducer(cfg)
	p.Close()

	evt := &Event{ID: "test", Type: "test.event", Source: "test"}
	err := p.Publish("test-topic", "key", evt)
	if err == nil {
		t.Fatal("expected error publishing after close")
	}
}

func TestNewProducerFromEnv_KafkaBrokersSet(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "kafka1:9092,kafka2:9092")
	defer os.Unsetenv("KAFKA_BROKERS")

	p := NewProducerFromEnv()
	if p == nil {
		t.Fatal("NewProducerFromEnv returned nil")
	}
	// Should be a KafkaProducer when KAFKA_BROKERS is set
	if _, ok := p.(*KafkaProducer); !ok {
		t.Fatalf("expected *KafkaProducer, got %T", p)
	}
	p.Close()
}

func TestNewProducerFromEnv_NoBrokers(t *testing.T) {
	os.Unsetenv("KAFKA_BROKERS")

	p := NewProducerFromEnv()
	if p == nil {
		t.Fatal("NewProducerFromEnv returned nil")
	}
	// Should be a ChannelProducer when KAFKA_BROKERS is not set
	if _, ok := p.(*ChannelProducer); !ok {
		t.Fatalf("expected *ChannelProducer, got %T", p)
	}
	p.Close()
}

func TestNewProducerFromEnv_EmptyBrokers(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "")
	defer os.Unsetenv("KAFKA_BROKERS")

	p := NewProducerFromEnv()
	if p == nil {
		t.Fatal("NewProducerFromEnv returned nil")
	}
	// Empty KAFKA_BROKERS should fall back to ChannelProducer
	if _, ok := p.(*ChannelProducer); !ok {
		t.Fatalf("expected *ChannelProducer, got %T", p)
	}
	p.Close()
}

func TestNewConsumerFromEnv_KafkaBrokersSet(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "kafka1:9092,kafka2:9092")
	defer os.Unsetenv("KAFKA_BROKERS")

	c := NewConsumerFromEnv(nil)
	if c == nil {
		t.Fatal("NewConsumerFromEnv returned nil")
	}
	if _, ok := c.(*KafkaConsumer); !ok {
		t.Fatalf("expected *KafkaConsumer, got %T", c)
	}
	c.Close()
}

func TestNewConsumerFromEnv_NoBrokers(t *testing.T) {
	os.Unsetenv("KAFKA_BROKERS")

	c := NewConsumerFromEnv(nil)
	if c == nil {
		t.Fatal("NewConsumerFromEnv returned nil")
	}
	if _, ok := c.(*ChannelConsumer); !ok {
		t.Fatalf("expected *ChannelConsumer, got %T", c)
	}
	c.Close()
}
