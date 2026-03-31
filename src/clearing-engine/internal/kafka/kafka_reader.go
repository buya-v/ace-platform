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

// KafkaConsumer implements the Consumer interface using real Kafka brokers
// via github.com/segmentio/kafka-go Reader (consumer group mode).
type KafkaConsumer struct {
	mu           sync.Mutex
	config       ConsumerConfig
	handlers     map[string]Handler
	readers      []*kafkago.Reader
	processedIDs map[string]bool
	dlqProducer  Producer
	closed       bool
}

// NewKafkaConsumer creates a wire-protocol Kafka consumer.
func NewKafkaConsumer(cfg ConsumerConfig, dlqProducer Producer) *KafkaConsumer {
	return &KafkaConsumer{
		config:       cfg,
		handlers:     make(map[string]Handler),
		processedIDs: make(map[string]bool),
		dlqProducer:  dlqProducer,
	}
}

// Subscribe registers a handler for a topic.
func (c *KafkaConsumer) Subscribe(topic string, handler Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[topic] = handler
}

// Start begins consuming from all subscribed topics. Blocks until ctx is cancelled.
func (c *KafkaConsumer) Start(ctx context.Context) error {
	c.mu.Lock()
	handlers := make(map[string]Handler)
	for t, h := range c.handlers {
		handlers[t] = h
	}
	c.mu.Unlock()

	if len(handlers) == 0 {
		<-ctx.Done()
		return nil
	}

	var wg sync.WaitGroup
	for topic, handler := range handlers {
		reader := kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        c.config.Brokers,
			GroupID:        c.config.GroupID,
			Topic:          topic,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
			Logger:         kafkago.LoggerFunc(func(msg string, args ...interface{}) {}),
			ErrorLogger:    kafkago.LoggerFunc(log.Printf),
		})
		c.mu.Lock()
		c.readers = append(c.readers, reader)
		c.mu.Unlock()

		wg.Add(1)
		go func(t string, r *kafkago.Reader, h Handler) {
			defer wg.Done()
			c.consumeReader(ctx, t, r, h)
		}(topic, reader, handler)
	}
	wg.Wait()
	return nil
}

func (c *KafkaConsumer) consumeReader(ctx context.Context, topic string, reader *kafkago.Reader, handler Handler) {
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("kafka consumer [%s]: fetch error: %v", topic, err)
			continue
		}

		rec := Record{
			Topic:     msg.Topic,
			Key:       string(msg.Key),
			Value:     msg.Value,
			Offset:    msg.Offset,
			Partition: int32(msg.Partition),
		}
		c.processRecord(ctx, topic, rec, handler)

		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("kafka consumer [%s]: commit error: %v", topic, err)
		}
	}
}

func (c *KafkaConsumer) processRecord(ctx context.Context, topic string, rec Record, handler Handler) {
	var event Event
	if err := json.Unmarshal(rec.Value, &event); err != nil {
		log.Printf("kafka consumer [%s]: unmarshal error: %v", topic, err)
		c.sendToDLQ(topic, rec, fmt.Errorf("unmarshal: %w", err))
		return
	}

	// Idempotency check
	c.mu.Lock()
	if c.processedIDs[event.ID] {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(c.config.RetryBackoff * time.Duration(1<<(attempt-1)))
		}
		if err := handler(ctx, event); err != nil {
			lastErr = err
			continue
		}
		// Success — mark as processed
		c.mu.Lock()
		c.processedIDs[event.ID] = true
		if len(c.processedIDs) > c.config.MaxTrackedIDs {
			c.evictOldest()
		}
		c.mu.Unlock()
		return
	}

	log.Printf("kafka consumer [%s]: event %s failed after %d retries: %v", topic, event.ID, c.config.MaxRetries, lastErr)
	c.sendToDLQ(topic, rec, lastErr)
}

func (c *KafkaConsumer) sendToDLQ(topic string, rec Record, err error) {
	if c.dlqProducer == nil {
		return
	}
	dlqTopic := "ace.dlq." + topicWithoutPrefix(topic)
	dlqErr := c.dlqProducer.Publish(dlqTopic, rec.Key, &Event{
		ID:        fmt.Sprintf("dlq-%s", rec.Key),
		Type:      "dlq.failure",
		Timestamp: time.Now().UTC(),
		Source:    c.config.GroupID,
		Payload:   rec.Value,
	})
	if dlqErr != nil {
		log.Printf("kafka consumer [%s]: DLQ publish failed: %v (original error: %v)", topic, dlqErr, err)
	}
}

func (c *KafkaConsumer) evictOldest() {
	count := 0
	half := len(c.processedIDs) / 2
	for k := range c.processedIDs {
		if count >= half {
			break
		}
		delete(c.processedIDs, k)
		count++
	}
}

// Close shuts down all kafka-go readers.
func (c *KafkaConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var firstErr error
	for _, r := range c.readers {
		if err := r.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
