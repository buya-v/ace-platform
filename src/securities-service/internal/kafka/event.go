// Package kafka provides event envelope types and producer/consumer interfaces
// for the securities-service event pipeline.
package kafka

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// Event is the standard GarudaX event envelope. Every message published to any
// GarudaX Kafka topic uses this structure.
type Event struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Timestamp     time.Time       `json:"timestamp"`
	Source        string          `json:"source"`
	CorrelationID string          `json:"correlation_id"`
	SchemaVersion int             `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

// NewEvent creates a new event envelope with a generated UUID.
func NewEvent(eventType, source, correlationID string, payload interface{}) (*Event, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("kafka: marshal payload: %w", err)
	}
	id, err := generateUUID()
	if err != nil {
		return nil, fmt.Errorf("kafka: generate event id: %w", err)
	}
	return &Event{
		ID:            id,
		Type:          eventType,
		Timestamp:     time.Now().UTC(),
		Source:        source,
		CorrelationID: correlationID,
		SchemaVersion: 1,
		Payload:       data,
	}, nil
}

// Record represents a Kafka record with key, value, topic, and metadata.
type Record struct {
	Topic     string
	Key       string
	Value     []byte
	Offset    int64
	Partition int32
}

// generateUUID produces a UUID v4 using crypto/rand.
func generateUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
