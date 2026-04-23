package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// Gateway Kafka wiring:
//   Consumer: ace.margin.call-issued (push to WebSocket clients)
//   Consumer: ace.compliance.status-changed (push to WebSocket clients)

const ServiceName = "gateway"

// NewConsumerFromEnv creates a Consumer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaConsumer.
// Otherwise returns a ChannelConsumer for local/test use.
func NewConsumerFromEnv(dlqProducer Producer) Consumer {
	brokers := os.Getenv("KAFKA_BROKERS")
	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = ServiceName
	}
	if brokers != "" && len(strings.TrimSpace(brokers)) > 0 {
		cfg := ConsumerConfigFromEnv(groupID)
		log.Printf("[%s] using real Kafka consumer, brokers=%v, group=%s", ServiceName, cfg.Brokers, cfg.GroupID)
		return NewKafkaConsumer(cfg, dlqProducer)
	}
	log.Printf("[%s] KAFKA_BROKERS not set, using channel-based consumer", ServiceName)
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
