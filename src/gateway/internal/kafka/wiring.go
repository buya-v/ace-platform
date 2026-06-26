package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
)

// Gateway Kafka wiring:
//   Consumer: ace.margin.call-issued (push to WebSocket clients)
//   Consumer: ace.compliance.status-changed (push to WebSocket clients)

const ServiceName = "gateway"

// kafkaBrokersConfigured reports whether the KAFKA_BROKERS environment variable
// is set to a non-empty (non-whitespace) value.
func kafkaBrokersConfigured() bool {
	return strings.TrimSpace(os.Getenv("KAFKA_BROKERS")) != ""
}

// NewConsumerFromEnv creates a Consumer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaConsumer. If it is
// unset, it FAILS FAST (log.Fatal) outside of unit tests rather than silently
// returning the in-process channel adapter, which would never receive
// cross-process events in a multi-process deployment. The in-process adapter is
// reachable only under testing.Testing() via newInProcessConsumer.
func NewConsumerFromEnv(dlqProducer Producer) Consumer {
	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = ServiceName
	}
	if kafkaBrokersConfigured() {
		cfg := ConsumerConfigFromEnv(groupID)
		log.Printf("[%s] using real Kafka consumer, brokers=%v, group=%s", ServiceName, cfg.Brokers, cfg.GroupID)
		return NewKafkaConsumer(cfg, dlqProducer)
	}
	if !testing.Testing() {
		log.Fatalf("[%s] KAFKA_BROKERS is required but not set; refusing to fall back to the in-process channel consumer in a multi-process deployment — cross-service events would never be received. Set KAFKA_BROKERS, or build the in-process adapter explicitly via newInProcessConsumer in unit tests.", ServiceName)
	}
	log.Printf("[%s] KAFKA_BROKERS not set; using in-process channel consumer (TEST MODE ONLY)", ServiceName)
	return newInProcessConsumer(groupID, dlqProducer)
}

// newInProcessConsumer builds the in-process channel-based consumer used ONLY by
// unit tests. Production code reaches it only via NewConsumerFromEnv under
// testing.Testing(); the in-process adapter must never be used in a
// multi-process deployment, where Go channels do not cross process boundaries.
func newInProcessConsumer(groupID string, dlqProducer Producer) *ChannelConsumer {
	return NewChannelConsumer(DefaultConsumerConfig(groupID), dlqProducer)
}

// MarginCallIssuedPayload mirrors margin-engine's published payload.
type MarginCallIssuedPayload struct {
	MarginCallID      string `json:"margin_call_id"`
	ParticipantID     string `json:"participant_id"`
	InstrumentID      string `json:"instrument_id"`
	CallType          string `json:"call_type"`
	RequiredAmount    string `json:"required_amount"`
	CurrentMargin     string `json:"current_margin"`
	MaintenanceMargin string `json:"maintenance_margin"`
	Deficit           string `json:"deficit"`
	Status            string `json:"status"`
	Deadline          string `json:"deadline"`
	IssuedAt          string `json:"issued_at"`
}

// ComplianceStatusChangedPayload mirrors compliance-service's published payload.
type ComplianceStatusChangedPayload struct {
	ParticipantID  string   `json:"participant_id"`
	PreviousStatus string   `json:"previous_status"`
	NewStatus      string   `json:"new_status"`
	KYCLevel       string   `json:"kyc_level"`
	AMLCheckPassed bool     `json:"aml_check_passed"`
	Restrictions   []string `json:"restrictions"`
	Reason         string   `json:"reason"`
	ChangedAt      string   `json:"changed_at"`
}

// MarginCallHandler returns a handler for ace.margin.call-issued events.
type MarginCallCallback func(ctx context.Context, payload MarginCallIssuedPayload, correlationID string) error

func MarginCallHandler(cb MarginCallCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload MarginCallIssuedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode margin-call payload: %w", err)
		}
		return cb(ctx, payload, event.CorrelationID)
	}
}

// ComplianceStatusHandler returns a handler for ace.compliance.status-changed events.
type ComplianceCallback func(ctx context.Context, payload ComplianceStatusChangedPayload, correlationID string) error

func ComplianceStatusHandler(cb ComplianceCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload ComplianceStatusChangedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode compliance payload: %w", err)
		}
		return cb(ctx, payload, event.CorrelationID)
	}
}
