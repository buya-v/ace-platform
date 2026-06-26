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

// Settlement-engine Kafka wiring:
//   Consumer: ace.clearing.novated
//   Producer: ace.settlement.completed (partition key: instrument_id)

const ServiceName = "settlement-engine"

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
	p.RegisterTopic(TopicSettlementCompleted, 1000)
	return p
}

// SettlementCompletedPayload is the event payload for ace.settlement.completed.
type SettlementCompletedPayload struct {
	CycleID           string                 `json:"cycle_id"`
	SettleDate        string                 `json:"settle_date"`
	Status            string                 `json:"status"`
	SettlementPrices  []SettlementPriceEntry `json:"settlement_prices"`
	TotalPayIn        string                 `json:"total_pay_in"`
	TotalPayOut       string                 `json:"total_pay_out"`
	InstructionsCount int                    `json:"instructions_count"`
	StartedAt         string                 `json:"started_at"`
	CompletedAt       string                 `json:"completed_at"`
}

// SettlementPriceEntry is a single instrument's settlement price.
type SettlementPriceEntry struct {
	InstrumentID    string `json:"instrument_id"`
	SettlementPrice string `json:"settlement_price"`
	PreviousPrice   string `json:"previous_price"`
}

// ClearingNovatedPayload mirrors the clearing-engine's published payload.
type ClearingNovatedPayload struct {
	ObligationID        string          `json:"obligation_id"`
	TradeID             string          `json:"trade_id"`
	InstrumentID        string          `json:"instrument_id"`
	BuyerParticipantID  string          `json:"buyer_participant_id"`
	SellerParticipantID string          `json:"seller_participant_id"`
	Price               string          `json:"price"`
	Quantity            uint64          `json:"quantity"`
	Status              string          `json:"status"`
	BuyerPosition       json.RawMessage `json:"buyer_position"`
	SellerPosition      json.RawMessage `json:"seller_position"`
	NovatedAt           string          `json:"novated_at"`
}

// PublishSettlementCompleted publishes a settlement completion event.
func PublishSettlementCompleted(p Producer, payload SettlementCompletedPayload, correlationID string) error {
	evt, err := NewEvent(TopicSettlementCompleted, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	key := payload.CycleID
	if len(payload.SettlementPrices) > 0 {
		key = payload.SettlementPrices[0].InstrumentID
	}
	return p.Publish(TopicSettlementCompleted, key, evt)
}

// ClearingNovatedHandler returns a handler for ace.clearing.novated events.
type NovatedCallback func(ctx context.Context, payload ClearingNovatedPayload, correlationID string) error

func ClearingNovatedHandler(cb NovatedCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload ClearingNovatedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode novated payload: %w", err)
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
