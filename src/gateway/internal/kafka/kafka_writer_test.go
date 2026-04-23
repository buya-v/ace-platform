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

func TestNewConsumerFromEnv_CustomGroupID(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "kafka1:9092")
	os.Setenv("KAFKA_GROUP_ID", "custom-group")
	defer os.Unsetenv("KAFKA_BROKERS")
	defer os.Unsetenv("KAFKA_GROUP_ID")

	c := NewConsumerFromEnv(nil)
	if c == nil {
		t.Fatal("NewConsumerFromEnv returned nil")
	}
	kc, ok := c.(*KafkaConsumer)
	if !ok {
		t.Fatalf("expected *KafkaConsumer, got %T", c)
	}
	if kc.config.GroupID != "custom-group" {
		t.Errorf("group ID = %q, want custom-group", kc.config.GroupID)
	}
	c.Close()
}
