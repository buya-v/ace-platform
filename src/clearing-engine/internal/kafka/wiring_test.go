package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPublishClearingNovated(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicClearingNovated, 10)

	payload := ClearingNovatedPayload{
		ObligationID:        "OBL-001",
		TradeID:             "TRD-001",
		InstrumentID:        "WHEAT-2026Q3",
		BuyerParticipantID:  "PART-001",
		SellerParticipantID: "PART-002",
		CCPID:               "GarudaX-CCP",
		Price:               "1850.0000",
		Quantity:            100,
		Status:              "NOVATED",
		NovatedAt:           "2026-03-28T09:15:00.456Z",
	}

	if err := PublishClearingNovated(p, payload, "corr-1"); err != nil {
		t.Fatalf("PublishClearingNovated: %v", err)
	}

	recs := p.Records(TopicClearingNovated)
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
	evt := Event{ID: "evt-1", Type: TopicTradesExecuted, Payload: data, CorrelationID: "corr-1"}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.TradeID != "TRD-001" {
		t.Errorf("trade_id = %q, want TRD-001", received.TradeID)
	}
}

func TestSetupConsumer(t *testing.T) {
	cfg := DefaultConsumerConfig("clearing-engine-trades")
	c := SetupConsumer(cfg, nil)
	if c == nil {
		t.Fatal("SetupConsumer returned nil")
	}
}
