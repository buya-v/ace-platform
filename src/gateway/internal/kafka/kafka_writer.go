package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// KafkaProducer implements the Producer interface using a real Kafka broker
// via github.com/segmentio/kafka-go Writer.
type KafkaProducer struct {
	mu      sync.Mutex
	writer  *kafkago.Writer
	closed  bool
	config  ProducerConfig
}

// NewKafkaProducer creates a wire-protocol Kafka producer. The Writer is
// configured with automatic topic creation, snappy compression, and the
// retry/timeout settings from ProducerConfig.
func NewKafkaProducer(cfg ProducerConfig) *KafkaProducer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Balancer:     &kafkago.Hash{},
		MaxAttempts:  cfg.MaxRetries + 1,
		WriteTimeout: cfg.WriteTimeout,
		RequiredAcks: kafkago.RequireOne,
		Async:        false,
		Logger:       kafkago.LoggerFunc(func(msg string, args ...interface{}) {}),
		ErrorLogger:  kafkago.LoggerFunc(log.Printf),
	}
	return &KafkaProducer{
		writer: w,
		config: cfg,
	}
}

// Publish serializes the event envelope and sends it to the Kafka topic
// with the specified partition key.
func (p *KafkaProducer) Publish(topic, key string, event *Event) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("kafka producer: closed")
	}
	w := p.writer
	p.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("kafka producer: marshal event: %w", err)
	}

	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
		Time:  time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.config.WriteTimeout)
	defer cancel()

	if err := w.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka producer: publish to %s: %w", topic, err)
	}
	return nil
}

// Close flushes pending writes and closes the underlying Writer.
func (p *KafkaProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return p.writer.Close()
}
