package kafka

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Producer sends records to Kafka topics.
type Producer interface {
	// Publish sends an event to the given topic with the specified partition key.
	Publish(topic, key string, event *Event) error
	// Close shuts down the producer.
	Close() error
}

// ProducerConfig holds producer settings.
type ProducerConfig struct {
	Brokers       []string
	MaxRetries    int
	RetryBackoff  time.Duration
	WriteTimeout  time.Duration
}

// DefaultProducerConfig returns sensible defaults.
func DefaultProducerConfig() ProducerConfig {
	return ProducerConfig{
		Brokers:      []string{"localhost:9092"},
		MaxRetries:   3,
		RetryBackoff: 100 * time.Millisecond,
		WriteTimeout: 10 * time.Second,
	}
}

// ChannelProducer is an in-process Producer backed by Go channels.
// It is used for testing and local development.
type ChannelProducer struct {
	mu       sync.Mutex
	topics   map[string]chan Record
	closed   bool
	config   ProducerConfig
	onPublish func(topic, key string, event *Event) error
}

// NewChannelProducer creates a channel-based producer. Provide topic channels
// via RegisterTopic, or use SetPublishHook to intercept all publishes.
func NewChannelProducer(cfg ProducerConfig) *ChannelProducer {
	return &ChannelProducer{
		topics: make(map[string]chan Record),
		config: cfg,
	}
}

// RegisterTopic adds a buffered channel for the given topic.
func (p *ChannelProducer) RegisterTopic(topic string, bufSize int) chan Record {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan Record, bufSize)
	p.topics[topic] = ch
	return ch
}

// SetPublishHook sets a callback invoked on every Publish. If the hook returns
// an error, Publish retries according to config.
func (p *ChannelProducer) SetPublishHook(fn func(topic, key string, event *Event) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPublish = fn
}

// Publish serializes the event and sends it to the topic channel.
func (p *ChannelProducer) Publish(topic, key string, event *Event) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("kafka producer: closed")
	}
	hook := p.onPublish
	ch := p.topics[topic]
	cfg := p.config
	p.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(cfg.RetryBackoff * time.Duration(1<<(attempt-1)))
		}

		if hook != nil {
			if err := hook(topic, key, event); err != nil {
				lastErr = err
				continue
			}
		}

		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("kafka producer: marshal event: %w", err)
		}

		rec := Record{
			Topic: topic,
			Key:   key,
			Value: data,
		}

		if ch != nil {
			select {
			case ch <- rec:
			default:
				lastErr = fmt.Errorf("kafka producer: topic %s channel full", topic)
				continue
			}
		}
		return nil
	}
	return fmt.Errorf("kafka producer: publish to %s failed after %d retries: %w", topic, cfg.MaxRetries, lastErr)
}

// Close marks the producer as closed.
func (p *ChannelProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, ch := range p.topics {
		close(ch)
	}
	return nil
}

// Records returns all records currently buffered on a topic channel.
func (p *ChannelProducer) Records(topic string) []Record {
	p.mu.Lock()
	ch := p.topics[topic]
	p.mu.Unlock()
	if ch == nil {
		return nil
	}
	var recs []Record
	for {
		select {
		case r, ok := <-ch:
			if !ok {
				return recs
			}
			recs = append(recs, r)
		default:
			return recs
		}
	}
}
