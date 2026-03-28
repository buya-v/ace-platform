package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPublishTradeIngested(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicMarketDataIngested, 10)

	payload := TradeIngestedPayload{
		InstrumentID: "WHEAT-2026Q3",
		TradeID:      "TRD-001",
		Price:        "1850.0000",
		Quantity:     100,
		Side:         "BUY",
	}

	if err := PublishTradeIngested(p, payload, "corr-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recs := p.Records(TopicMarketDataIngested)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "WHEAT-2026Q3" {
		t.Errorf("key = %q, want WHEAT-2026Q3", recs[0].Key)
	}
}

func TestTradeExecutedHandler(t *testing.T) {
	var received TradeExecutedPayload
	handler := TradeExecutedHandler(func(ctx context.Context, p TradeExecutedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := TradeExecutedPayload{TradeID: "TRD-001", InstrumentID: "WHEAT"}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.TradeID != "TRD-001" {
		t.Errorf("trade_id = %q, want TRD-001", received.TradeID)
	}
}

func TestSettlementCompletedHandler(t *testing.T) {
	var received SettlementCompletedPayload
	handler := SettlementCompletedHandler(func(ctx context.Context, p SettlementCompletedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := SettlementCompletedPayload{CycleID: "SETTLE-001", Status: "COMPLETED"}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.CycleID != "SETTLE-001" {
		t.Errorf("cycle_id = %q, want SETTLE-001", received.CycleID)
	}
}
