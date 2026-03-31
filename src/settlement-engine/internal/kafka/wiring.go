package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// Settlement-engine Kafka wiring:
//   Consumer: ace.clearing.novated
//   Producer: ace.settlement.completed (partition key: instrument_id)

const ServiceName = "settlement-engine"

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
	p.RegisterTopic(TopicSettlementCompleted, 1000)
	return p
}

// SettlementCompletedPayload is the event payload for ace.settlement.completed.
type SettlementCompletedPayload struct {
	CycleID           string                    `json:"cycle_id"`
	SettleDate        string                    `json:"settle_date"`
	Status            string                    `json:"status"`
	SettlementPrices  []SettlementPriceEntry    `json:"settlement_prices"`
	TotalPayIn        string                    `json:"total_pay_in"`
	TotalPayOut       string                    `json:"total_pay_out"`
	InstructionsCount int                       `json:"instructions_count"`
	StartedAt         string                    `json:"started_at"`
	CompletedAt       string                    `json:"completed_at"`
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
