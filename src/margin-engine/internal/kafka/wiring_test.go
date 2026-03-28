package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPublishMarginCallIssued(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicMarginCallIssued, 10)

	payload := MarginCallIssuedPayload{
		MarginCallID:  "MC-001",
		ParticipantID: "PART-001",
		InstrumentID:  "WHEAT-2026Q3",
		CallType:      "VARIATION",
		Status:        "ISSUED",
	}

	if err := PublishMarginCallIssued(p, payload, "corr-1"); err != nil {
		t.Fatalf("PublishMarginCallIssued: %v", err)
	}

	recs := p.Records(TopicMarginCallIssued)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "PART-001" {
		t.Errorf("key = %q, want PART-001", recs[0].Key)
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

func TestReceiptPledgedHandler(t *testing.T) {
	var received WarehouseReceiptPledgedPayload
	handler := ReceiptPledgedHandler(func(ctx context.Context, p WarehouseReceiptPledgedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := WarehouseReceiptPledgedPayload{ReceiptID: "WR-001", ParticipantID: "PART-001"}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data, CorrelationID: "corr-1"}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.ReceiptID != "WR-001" {
		t.Errorf("receipt_id = %q, want WR-001", received.ReceiptID)
	}
}
