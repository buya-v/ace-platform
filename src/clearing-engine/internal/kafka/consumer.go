package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"fmt"
	"log"
	"sync"
	"time"
)

// Handler processes a single event. Return an error to trigger retry/DLQ.
type Handler func(ctx context.Context, event Event) error

// Consumer reads records from Kafka topics and dispatches to handlers.
type Consumer interface {
	// Subscribe registers a handler for a topic.
	Subscribe(topic string, handler Handler)
	// Start begins consuming. Blocks until ctx is cancelled.
	Start(ctx context.Context) error
	// Close shuts down the consumer.
	Close() error
}

// ConsumerConfig holds consumer settings.
type ConsumerConfig struct {
	Brokers      []string
	GroupID      string
	MaxRetries   int
	RetryBackoff time.Duration
	MaxTrackedIDs int
}

// DefaultConsumerConfig returns sensible defaults.
func DefaultConsumerConfig(groupID string) ConsumerConfig {
	return ConsumerConfig{
		Brokers:       []string{"localhost:9092"},
		GroupID:       groupID,
		MaxRetries:    3,
		RetryBackoff:  100 * time.Millisecond,
		MaxTrackedIDs: 100000,
	}
}

// ChannelConsumer is an in-process Consumer backed by Go channels.
type ChannelConsumer struct {
	mu           sync.Mutex
	config       ConsumerConfig
	handlers     map[string]Handler
	sources      map[string]<-chan Record
	processedIDs map[string]bool
	dlqProducer  Producer
	closed       bool
}

// NewChannelConsumer creates a channel-based consumer.
func NewChannelConsumer(cfg ConsumerConfig, dlqProducer Producer) *ChannelConsumer {
	return &ChannelConsumer{
		config:       cfg,
		handlers:     make(map[string]Handler),
		sources:      make(map[string]<-chan Record),
		processedIDs: make(map[string]bool),
		dlqProducer:  dlqProducer,
	}
}

// Subscribe registers a handler for a topic.
func (c *ChannelConsumer) Subscribe(topic string, handler Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[topic] = handler
}

// AddSource sets the channel to read from for a given topic.
func (c *ChannelConsumer) AddSource(topic string, ch <-chan Record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources[topic] = ch
}

// Start begins consuming from all subscribed topics. Blocks until ctx is done.
func (c *ChannelConsumer) Start(ctx context.Context) error {
	c.mu.Lock()
	topics := make(map[string]<-chan Record)
	handlers := make(map[string]Handler)
	for t, ch := range c.sources {
		if h, ok := c.handlers[t]; ok {
			topics[t] = ch
			handlers[t] = h
		}
	}
	c.mu.Unlock()

	var wg sync.WaitGroup
	for topic, ch := range topics {
		wg.Add(1)
		go func(t string, src <-chan Record, h Handler) {
			defer wg.Done()
			c.consumeTopic(ctx, t, src, h)
		}(topic, ch, handlers[topic])
	}
	wg.Wait()
	return nil
}

func (c *ChannelConsumer) consumeTopic(ctx context.Context, topic string, src <-chan Record, handler Handler) {
	for {
		select {
		case <-ctx.Done():
			return
		case rec, ok := <-src:
			if !ok {
				return
			}
			c.processRecord(ctx, topic, rec, handler)
		}
	}
}

func (c *ChannelConsumer) processRecord(ctx context.Context, topic string, rec Record, handler Handler) {
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

func (c *ChannelConsumer) sendToDLQ(topic string, rec Record, err error) {
	if c.dlqProducer == nil {
		return
	}
	dlqTopic := TenantID + ".dlq." + topicWithoutPrefix(topic)
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

func (c *ChannelConsumer) evictOldest() {
	// Simple eviction: clear half the map when full.
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

// Close marks the consumer as closed.
func (c *ChannelConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// ProcessedCount returns the number of tracked event IDs (for testing).
func (c *ChannelConsumer) ProcessedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.processedIDs)
}

// topicWithoutPrefix strips the tenant prefix from a topic name for DLQ routing.
func topicWithoutPrefix(topic string) string {
	if idx := strings.Index(topic, "."); idx >= 0 {
		return topic[idx+1:]
	}
	return topic
}
