package kafka

import (
	"encoding/json"
	"testing"
)

func TestPublishTradeExecuted(t *testing.T) {
	cfg := DefaultProducerConfig()
	p := NewChannelProducer(cfg)
	p.RegisterTopic(TopicTradesExecuted, 10)

	payload := TradeExecutedPayload{
		TradeID:              "TRD-001",
		InstrumentID:         "WHEAT-2026Q3",
		BuyOrderID:           "ORD-B-001",
		SellOrderID:          "ORD-S-001",
		BuyerParticipantID:   "PART-001",
		SellerParticipantID:  "PART-002",
		Price:                "1850.0000",
		Quantity:             100,
		TradeValue:           "185000.0000",
		AggressorSide:        "BUY",
		TradeType:            "REGULAR",
		SequenceNumber:       42,
		ExecutedAt:           "2026-03-28T09:15:00.123Z",
	}

	if err := PublishTradeExecuted(p, payload, "corr-1"); err != nil {
		t.Fatalf("PublishTradeExecuted: %v", err)
	}

	recs := p.Records(TopicTradesExecuted)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "WHEAT-2026Q3" {
		t.Errorf("key = %q, want WHEAT-2026Q3", recs[0].Key)
	}

	var evt Event
	json.Unmarshal(recs[0].Value, &evt)
	if evt.Source != ServiceName {
		t.Errorf("source = %q, want %q", evt.Source, ServiceName)
	}
}

func TestNewTradeProducer(t *testing.T) {
	p := NewTradeProducer(DefaultProducerConfig())
	if p == nil {
		t.Fatal("NewTradeProducer returned nil")
	}
}
