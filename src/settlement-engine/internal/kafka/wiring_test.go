package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPublishSettlementCompleted(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicSettlementCompleted, 10)

	payload := SettlementCompletedPayload{
		CycleID:    "SETTLE-20260328",
		SettleDate: "2026-03-28",
		Status:     "COMPLETED",
		SettlementPrices: []SettlementPriceEntry{
			{InstrumentID: "WHEAT-2026Q3", SettlementPrice: "1855.0000", PreviousPrice: "1840.0000"},
		},
		TotalPayIn:        "1250000.0000",
		TotalPayOut:       "1250000.0000",
		InstructionsCount: 42,
	}

	if err := PublishSettlementCompleted(p, payload, "corr-1"); err != nil {
		t.Fatalf("PublishSettlementCompleted: %v", err)
	}

	recs := p.Records(TopicSettlementCompleted)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "WHEAT-2026Q3" {
		t.Errorf("key = %q, want WHEAT-2026Q3", recs[0].Key)
	}
}

func TestPublishSettlementCompleted_NoPrices(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicSettlementCompleted, 10)

	payload := SettlementCompletedPayload{CycleID: "SETTLE-001", Status: "COMPLETED"}
	if err := PublishSettlementCompleted(p, payload, "corr-1"); err != nil {
		t.Fatalf("PublishSettlementCompleted: %v", err)
	}
	recs := p.Records(TopicSettlementCompleted)
	if recs[0].Key != "SETTLE-001" {
		t.Errorf("key = %q, want SETTLE-001 (fallback to cycle_id)", recs[0].Key)
	}
}

func TestClearingNovatedHandler(t *testing.T) {
	var received ClearingNovatedPayload
	handler := ClearingNovatedHandler(func(ctx context.Context, p ClearingNovatedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := ClearingNovatedPayload{ObligationID: "OBL-001", InstrumentID: "WHEAT"}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data, CorrelationID: "corr-1"}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.ObligationID != "OBL-001" {
		t.Errorf("obligation_id = %q, want OBL-001", received.ObligationID)
	}
}
