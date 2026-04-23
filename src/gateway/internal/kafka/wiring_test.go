package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

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

func TestComplianceStatusHandler(t *testing.T) {
	var received ComplianceStatusChangedPayload
	handler := ComplianceStatusHandler(func(ctx context.Context, p ComplianceStatusChangedPayload, corrID string) error {
		received = p
		return nil
	})

	payload := ComplianceStatusChangedPayload{
		ParticipantID: "PART-001",
		NewStatus:     "APPROVED",
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
