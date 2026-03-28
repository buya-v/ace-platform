package kafka

import (
	"testing"
)

func TestPublishReceiptPledged(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicWarehouseReceiptPledged, 10)

	payload := ReceiptPledgedPayload{
		ReceiptID:       "WR-001",
		ParticipantID:   "PART-001",
		Commodity:       "WHEAT",
		QuantityMT:      500.0,
		WarehouseID:     "WH-UB-001",
		Grade:           "GRADE_A",
		CollateralValue: "925000.0000",
	}

	if err := PublishReceiptPledged(p, payload, "corr-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recs := p.Records(TopicWarehouseReceiptPledged)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "PART-001" {
		t.Errorf("key = %q, want PART-001", recs[0].Key)
	}
}

func TestPublishDeliveryCompleted(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicWarehouseDeliveryCompleted, 10)

	payload := DeliveryCompletedPayload{
		DeliveryID:          "DEL-001",
		ReceiptID:           "WR-001",
		InstrumentID:        "WHEAT-2026Q3",
		BuyerParticipantID:  "PART-001",
		SellerParticipantID: "PART-002",
		QuantityMT:          500.0,
		WarehouseID:         "WH-UB-001",
	}

	if err := PublishDeliveryCompleted(p, payload, "corr-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recs := p.Records(TopicWarehouseDeliveryCompleted)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "WHEAT-2026Q3" {
		t.Errorf("key = %q, want WHEAT-2026Q3", recs[0].Key)
	}
}
