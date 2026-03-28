package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPublishUserRegistered(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicAuthUserRegistered, 10)

	payload := UserRegisteredPayload{
		UserID:        "USR-001",
		ParticipantID: "PART-001",
		Email:         "trader@example.mn",
		Roles:         []string{"TRADER"},
	}

	if err := PublishUserRegistered(p, payload, "corr-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recs := p.Records(TopicAuthUserRegistered)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "PART-001" {
		t.Errorf("key = %q, want PART-001", recs[0].Key)
	}
}

func TestComplianceStatusHandler(t *testing.T) {
	var received ComplianceStatusChangedPayload
	handler := ComplianceStatusHandler(func(ctx context.Context, p ComplianceStatusChangedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := ComplianceStatusChangedPayload{
		ParticipantID:  "PART-001",
		PreviousStatus: "PENDING_REVIEW",
		NewStatus:      "APPROVED",
	}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.NewStatus != "APPROVED" {
		t.Errorf("new_status = %q, want APPROVED", received.NewStatus)
	}
}
