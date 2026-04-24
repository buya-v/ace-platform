package kafka_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/kafka"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ---- helpers ----------------------------------------------------------------

func defaultCfg() kafka.ProducerConfig {
	return kafka.ProducerConfig{
		Brokers:      []string{"localhost:9092"},
		MaxRetries:   0, // no retries in tests to keep them fast
		RetryBackoff: time.Millisecond,
		WriteTimeout: time.Second,
	}
}

func newEvent(t *testing.T, eventType string) *kafka.Event {
	t.Helper()
	ev, err := kafka.NewEvent(eventType, "test-source", "corr-1", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return ev
}

// ---- ChannelProducer tests --------------------------------------------------

func TestChannelProducer_Publish(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	p.RegisterTopic("test.topic", 8)

	ev := newEvent(t, "test.event")
	if err := p.Publish("test.topic", "key-1", ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	recs := p.Records("test.topic")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Topic != "test.topic" {
		t.Errorf("Topic: want test.topic, got %s", r.Topic)
	}
	if r.Key != "key-1" {
		t.Errorf("Key: want key-1, got %s", r.Key)
	}
	if len(r.Value) == 0 {
		t.Error("Value is empty")
	}

	// Verify Value is valid JSON that round-trips to an Event.
	var decoded kafka.Event
	if err := json.Unmarshal(r.Value, &decoded); err != nil {
		t.Fatalf("Unmarshal value: %v", err)
	}
	if decoded.Type != "test.event" {
		t.Errorf("decoded Type: want test.event, got %s", decoded.Type)
	}
	if decoded.Source != "test-source" {
		t.Errorf("decoded Source: want test-source, got %s", decoded.Source)
	}
}

func TestChannelProducer_Publish_MultipleRecords(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	p.RegisterTopic("t", 16)

	for i := 0; i < 5; i++ {
		ev := newEvent(t, "evt")
		if err := p.Publish("t", "k", ev); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	recs := p.Records("t")
	if len(recs) != 5 {
		t.Errorf("expected 5 records, got %d", len(recs))
	}
}

func TestChannelProducer_Records_DrainIdempotent(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	p.RegisterTopic("t", 8)
	p.Publish("t", "k", newEvent(t, "e"))

	first := p.Records("t")
	second := p.Records("t")
	if len(first) != 1 {
		t.Errorf("first drain: expected 1, got %d", len(first))
	}
	if len(second) != 0 {
		t.Errorf("second drain: expected 0 (already drained), got %d", len(second))
	}
}

func TestChannelProducer_Records_UnregisteredTopic(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	recs := p.Records("nonexistent.topic")
	if recs != nil {
		t.Errorf("expected nil for unregistered topic, got %v", recs)
	}
}

func TestChannelProducer_Publish_AfterClose(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	p.RegisterTopic("t", 4)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ev := newEvent(t, "e")
	if err := p.Publish("t", "k", ev); err == nil {
		t.Error("expected error publishing to closed producer, got nil")
	}
}

func TestChannelProducer_PublishHook_Called(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	p.RegisterTopic("t", 4)

	called := false
	p.SetPublishHook(func(topic, key string, event *kafka.Event) error {
		called = true
		return nil
	})

	if err := p.Publish("t", "k", newEvent(t, "e")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !called {
		t.Error("publish hook was not called")
	}
}

// testTenantID is the tenant used in all wiring tests.
const testTenantID = "ace-commodities"

// ---- PublishTradeExecuted tests ---------------------------------------------

func TestPublishTradeExecuted(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	topic := kafka.TopicTradeExecuted(testTenantID)
	p.RegisterTopic(topic, 8)

	trade := &types.SecurityTrade{
		ID:             "trade-abc",
		BuyOrderID:     "buy-1",
		SellOrderID:    "sell-1",
		InstrumentID:   "INST-1",
		Price:          55.50,
		Quantity:       100,
		TradeDate:      "2026-01-01",
		SettlementDate: "2026-01-03",
		Status:         types.TradeStatusPending,
		CreatedAt:      "2026-01-01T00:00:00Z",
	}

	if err := kafka.PublishTradeExecuted(p, testTenantID, trade); err != nil {
		t.Fatalf("PublishTradeExecuted: %v", err)
	}

	recs := p.Records(topic)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Topic != topic {
		t.Errorf("Topic: want %s, got %s", topic, r.Topic)
	}
	if r.Key != "trade-abc" {
		t.Errorf("Key: want trade-abc, got %s", r.Key)
	}

	// Decode and verify event envelope fields.
	var ev kafka.Event
	if err := json.Unmarshal(r.Value, &ev); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if ev.Type != "trade.executed" {
		t.Errorf("event Type: want trade.executed, got %s", ev.Type)
	}
	if ev.Source != "securities-service" {
		t.Errorf("event Source: want securities-service, got %s", ev.Source)
	}
	if ev.CorrelationID != "trade-abc" {
		t.Errorf("CorrelationID: want trade-abc, got %s", ev.CorrelationID)
	}
	if ev.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: want 1, got %d", ev.SchemaVersion)
	}

	// Decode payload and verify trade fields survive round-trip.
	var payload types.SecurityTrade
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if payload.ID != "trade-abc" {
		t.Errorf("payload ID: want trade-abc, got %s", payload.ID)
	}
	if payload.Price != 55.50 {
		t.Errorf("payload Price: want 55.50, got %v", payload.Price)
	}
	if payload.Quantity != 100 {
		t.Errorf("payload Quantity: want 100, got %d", payload.Quantity)
	}
}

func TestPublishOrderCreated(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	topic := kafka.TopicOrderCreated(testTenantID)
	p.RegisterTopic(topic, 8)

	order := &types.SecurityOrder{
		ID:           "order-xyz",
		InstrumentID: "INST-1",
		Side:         types.OrderSideBuy,
		OrderType:    types.OrderTypeLimit,
		Quantity:     50,
		Price:        100.00,
		Status:       types.OrderStatusPending,
	}

	if err := kafka.PublishOrderCreated(p, testTenantID, order); err != nil {
		t.Fatalf("PublishOrderCreated: %v", err)
	}

	recs := p.Records(topic)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Key != "order-xyz" {
		t.Errorf("Key: want order-xyz, got %s", recs[0].Key)
	}
	var ev kafka.Event
	json.Unmarshal(recs[0].Value, &ev)
	if ev.Type != "order.created" {
		t.Errorf("event Type: want order.created, got %s", ev.Type)
	}
}

func TestPublishOrderCancelled(t *testing.T) {
	p := kafka.NewChannelProducer(defaultCfg())
	topic := kafka.TopicOrderCancelled(testTenantID)
	p.RegisterTopic(topic, 8)

	order := &types.SecurityOrder{
		ID:     "order-canc",
		Status: types.OrderStatusCancelled,
	}

	if err := kafka.PublishOrderCancelled(p, testTenantID, order); err != nil {
		t.Fatalf("PublishOrderCancelled: %v", err)
	}

	recs := p.Records(topic)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	var ev kafka.Event
	json.Unmarshal(recs[0].Value, &ev)
	if ev.Type != "order.cancelled" {
		t.Errorf("event Type: want order.cancelled, got %s", ev.Type)
	}
}

// ---- Nil producer tests (wiring nil-safety) ---------------------------------

func TestNilProducer_PublishTradeExecuted(t *testing.T) {
	trade := &types.SecurityTrade{ID: "t1", InstrumentID: "i1"}
	if err := kafka.PublishTradeExecuted(nil, testTenantID, trade); err != nil {
		t.Errorf("PublishTradeExecuted(nil) should be a no-op, got: %v", err)
	}
}

func TestNilProducer_PublishOrderCreated(t *testing.T) {
	order := &types.SecurityOrder{ID: "o1"}
	if err := kafka.PublishOrderCreated(nil, testTenantID, order); err != nil {
		t.Errorf("PublishOrderCreated(nil) should be a no-op, got: %v", err)
	}
}

func TestNilProducer_PublishOrderCancelled(t *testing.T) {
	order := &types.SecurityOrder{ID: "o1"}
	if err := kafka.PublishOrderCancelled(nil, testTenantID, order); err != nil {
		t.Errorf("PublishOrderCancelled(nil) should be a no-op, got: %v", err)
	}
}
