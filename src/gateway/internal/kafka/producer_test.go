package kafka

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
)

func TestChannelProducer_Publish(t *testing.T) {
	cfg := DefaultProducerConfig()
	p := NewChannelProducer(cfg)
	ch := p.RegisterTopic(TopicTradesExecuted, 10)

	evt, err := NewEvent(TopicTradesExecuted, "matching-engine", "corr-1", map[string]string{"trade_id": "T1"})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	if err := p.Publish(TopicTradesExecuted, "WHEAT-2026Q3", evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case rec := <-ch:
		if rec.Topic != TopicTradesExecuted {
			t.Errorf("topic = %q, want %q", rec.Topic, TopicTradesExecuted)
		}
		if rec.Key != "WHEAT-2026Q3" {
			t.Errorf("key = %q, want %q", rec.Key, "WHEAT-2026Q3")
		}
		var decoded Event
		if err := json.Unmarshal(rec.Value, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if decoded.ID != evt.ID {
			t.Errorf("event ID = %q, want %q", decoded.ID, evt.ID)
		}
		if decoded.Type != TopicTradesExecuted {
			t.Errorf("event type = %q, want %q", decoded.Type, TopicTradesExecuted)
		}
	default:
		t.Fatal("no record received")
	}
}

func TestChannelProducer_PublishClosed(t *testing.T) {
	cfg := DefaultProducerConfig()
	p := NewChannelProducer(cfg)
	p.Close()

	evt, _ := NewEvent("test", "test", "corr", nil)
	err := p.Publish("test", "key", evt)
	if err == nil {
		t.Fatal("expected error on closed producer")
	}
}

func TestChannelProducer_RetryOnHookError(t *testing.T) {
	cfg := DefaultProducerConfig()
	cfg.MaxRetries = 2
	cfg.RetryBackoff = 0 // no delay in tests
	p := NewChannelProducer(cfg)
	p.RegisterTopic("test-topic", 10)

	var attempts int32
	p.SetPublishHook(func(topic, key string, event *Event) error {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			return fmt.Errorf("transient error %d", n)
		}
		return nil
	})

	evt, _ := NewEvent("test-topic", "src", "corr", nil)
	if err := p.Publish("test-topic", "k", evt); err != nil {
		t.Fatalf("Publish should succeed after retries: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
	}
}

func TestChannelProducer_RetryExhausted(t *testing.T) {
	cfg := DefaultProducerConfig()
	cfg.MaxRetries = 1
	cfg.RetryBackoff = 0
	p := NewChannelProducer(cfg)

	p.SetPublishHook(func(topic, key string, event *Event) error {
		return fmt.Errorf("permanent error")
	})

	evt, _ := NewEvent("test", "src", "corr", nil)
	err := p.Publish("test", "k", evt)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestChannelProducer_ChannelFull(t *testing.T) {
	cfg := DefaultProducerConfig()
	cfg.MaxRetries = 0
	p := NewChannelProducer(cfg)
	p.RegisterTopic("full-topic", 1)

	evt1, _ := NewEvent("full-topic", "src", "corr", nil)
	evt2, _ := NewEvent("full-topic", "src", "corr", nil)

	if err := p.Publish("full-topic", "k", evt1); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	err := p.Publish("full-topic", "k", evt2)
	if err == nil {
		t.Fatal("expected error on full channel")
	}
}

func TestChannelProducer_Records(t *testing.T) {
	cfg := DefaultProducerConfig()
	p := NewChannelProducer(cfg)
	p.RegisterTopic("recs-topic", 10)

	for i := 0; i < 3; i++ {
		evt, _ := NewEvent("recs-topic", "src", "corr", nil)
		p.Publish("recs-topic", "k", evt)
	}

	recs := p.Records("recs-topic")
	if len(recs) != 3 {
		t.Errorf("records = %d, want 3", len(recs))
	}

	// Records on unknown topic
	recs = p.Records("unknown")
	if len(recs) != 0 {
		t.Errorf("records on unknown = %d, want 0", len(recs))
	}
}
