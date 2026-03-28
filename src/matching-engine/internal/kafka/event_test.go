package kafka

import (
	"encoding/json"
	"testing"
)

func TestNewEvent(t *testing.T) {
	payload := map[string]string{"trade_id": "T1"}
	evt, err := NewEvent(TopicTradesExecuted, "matching-engine", "corr-1", payload)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if evt.ID == "" {
		t.Error("event ID is empty")
	}
	if evt.Type != TopicTradesExecuted {
		t.Errorf("type = %q, want %q", evt.Type, TopicTradesExecuted)
	}
	if evt.Source != "matching-engine" {
		t.Errorf("source = %q, want %q", evt.Source, "matching-engine")
	}
	if evt.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", evt.SchemaVersion)
	}

	var p map[string]string
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p["trade_id"] != "T1" {
		t.Errorf("payload trade_id = %q, want T1", p["trade_id"])
	}
}

func TestNewEvent_NilPayload(t *testing.T) {
	evt, err := NewEvent("test", "src", "corr", nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if string(evt.Payload) != "null" {
		t.Errorf("payload = %s, want null", evt.Payload)
	}
}

func TestGenerateUUID(t *testing.T) {
	id1, err := generateUUID()
	if err != nil {
		t.Fatalf("generateUUID: %v", err)
	}
	id2, err := generateUUID()
	if err != nil {
		t.Fatalf("generateUUID: %v", err)
	}
	if id1 == id2 {
		t.Error("UUIDs should be unique")
	}
	if len(id1) != 36 {
		t.Errorf("UUID length = %d, want 36", len(id1))
	}
}
