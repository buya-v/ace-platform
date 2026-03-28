package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPublishComplianceStatusChanged(t *testing.T) {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicComplianceChanged, 10)

	payload := ComplianceStatusChangedPayload{
		ParticipantID:  "PART-001",
		PreviousStatus: "PENDING_REVIEW",
		NewStatus:      "APPROVED",
		KYCLevel:       "ENHANCED",
		AMLCheckPassed: true,
	}

	if err := PublishComplianceStatusChanged(p, payload, "corr-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recs := p.Records(TopicComplianceChanged)
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].Key != "PART-001" {
		t.Errorf("key = %q, want PART-001", recs[0].Key)
	}
}

func TestMarginCallHandler(t *testing.T) {
	var received MarginCallIssuedPayload
	handler := MarginCallHandler(func(ctx context.Context, p MarginCallIssuedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := MarginCallIssuedPayload{MarginCallID: "MC-001", ParticipantID: "PART-001"}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.MarginCallID != "MC-001" {
		t.Errorf("margin_call_id = %q, want MC-001", received.MarginCallID)
	}
}

func TestUserRegisteredHandler(t *testing.T) {
	var received UserRegisteredPayload
	handler := UserRegisteredHandler(func(ctx context.Context, p UserRegisteredPayload, corrID string) error {
		received = p
		return nil
	})

	payload := UserRegisteredPayload{UserID: "USR-001", ParticipantID: "PART-001", Roles: []string{"TRADER"}}
	data, _ := json.Marshal(payload)
	evt := Event{ID: "evt-1", Payload: data}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if received.UserID != "USR-001" {
		t.Errorf("user_id = %q, want USR-001", received.UserID)
	}
}
