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

// Compliance-service Kafka wiring:
//   Consumer: ace.margin.call-issued
//   Consumer: ace.auth.user-registered
//   Producer: ace.compliance.status-changed (partition key: participant_id)

const ServiceName = "compliance-service"

// NewProducerFromEnv creates a Producer based on environment configuration.
func NewProducerFromEnv() Producer {
	if kafkaBrokersConfigured() {
		cfg := ConfigFromEnv()
		log.Printf("[%s] using real Kafka producer, brokers=%v", ServiceName, cfg.Brokers)
		return NewKafkaProducer(cfg)
	}
	if !testing.Testing() {
		log.Fatalf("[%s] KAFKA_BROKERS is required but not set; refusing to fall back to the in-process channel producer in a multi-process deployment — cross-service events would be silently dropped. Set KAFKA_BROKERS, or build the in-process adapter explicitly via newInProcessProducer in unit tests.", ServiceName)
	}
	log.Printf("[%s] KAFKA_BROKERS not set; using in-process channel producer (TEST MODE ONLY)", ServiceName)
	return newInProcessProducer()
}

// kafkaBrokersConfigured reports whether the KAFKA_BROKERS environment variable
// is set to a non-empty (non-whitespace) value.
func kafkaBrokersConfigured() bool {
	return strings.TrimSpace(os.Getenv("KAFKA_BROKERS")) != ""
}

// newInProcessProducer builds the in-process channel-based producer used ONLY by
// unit tests. Production code reaches it only via NewProducerFromEnv under
// testing.Testing(); the in-process adapter must never be used in a
// multi-process deployment, where Go channels do not cross process boundaries.
func newInProcessProducer() *ChannelProducer {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicComplianceChanged, 1000)
	return p
}

// ComplianceStatusChangedPayload is the event payload for ace.compliance.status-changed.
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

// UserRegisteredPayload mirrors auth-service's published payload.
type UserRegisteredPayload struct {
	UserID        string   `json:"user_id"`
	ParticipantID string   `json:"participant_id"`
	Email         string   `json:"email"`
	Roles         []string `json:"roles"`
	RegisteredAt  string   `json:"registered_at"`
}

// PublishComplianceStatusChanged publishes a compliance status change event.
func PublishComplianceStatusChanged(p Producer, payload ComplianceStatusChangedPayload, correlationID string) error {
	evt, err := NewEvent(TopicComplianceChanged, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicComplianceChanged, payload.ParticipantID, evt)
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

// UserRegisteredHandler returns a handler for ace.auth.user-registered events.
type UserRegisteredCallback func(ctx context.Context, payload UserRegisteredPayload, correlationID string) error

func UserRegisteredHandler(cb UserRegisteredCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload UserRegisteredPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode user-registered payload: %w", err)
		}
		return cb(ctx, payload, event.CorrelationID)
	}
}

// NewConsumerFromEnv creates a Consumer based on environment configuration.
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
