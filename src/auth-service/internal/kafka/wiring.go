package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// Auth-service Kafka wiring:
//   Consumer: ace.compliance.status-changed
//   Producer: ace.auth.user-registered (partition key: participant_id)

const ServiceName = "auth-service"

// NewProducerFromEnv creates a Producer based on environment configuration.
func NewProducerFromEnv() Producer {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers != "" && len(strings.TrimSpace(brokers)) > 0 {
		cfg := ConfigFromEnv()
		log.Printf("[%s] using real Kafka producer, brokers=%v", ServiceName, cfg.Brokers)
		return NewKafkaProducer(cfg)
	}
	log.Printf("[%s] KAFKA_BROKERS not set, using channel-based producer", ServiceName)
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicAuthUserRegistered, 1000)
	return p
}

// UserRegisteredPayload is the event payload for ace.auth.user-registered.
type UserRegisteredPayload struct {
	UserID        string   `json:"user_id"`
	ParticipantID string   `json:"participant_id"`
	Email         string   `json:"email"`
	Roles         []string `json:"roles"`
	RegisteredAt  string   `json:"registered_at"`
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

// PublishUserRegistered publishes a user registration event.
func PublishUserRegistered(p Producer, payload UserRegisteredPayload, correlationID string) error {
	evt, err := NewEvent(TopicAuthUserRegistered, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicAuthUserRegistered, payload.ParticipantID, evt)
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

// NewConsumerFromEnv creates a Consumer based on environment configuration.
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
