package kafka

import (
	"context"
	"encoding/json"
	"fmt"
)

// Auth-service Kafka wiring:
//   Consumer: ace.compliance.status-changed
//   Producer: ace.auth.user-registered (partition key: participant_id)

const ServiceName = "auth-service"

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
