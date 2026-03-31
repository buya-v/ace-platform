package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// Clearing-engine Kafka wiring:
//   Consumer: ace.trades.executed
//   Consumer: ace.settlement.completed
//   Consumer: ace.warehouse.delivery-completed
//   Producer: ace.clearing.novated (partition key: instrument_id)

const ServiceName = "clearing-engine"

// NewProducerFromEnv creates a Producer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaProducer.
// Otherwise returns a ChannelProducer for local/test use.
func NewProducerFromEnv() Producer {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers != "" && len(strings.TrimSpace(brokers)) > 0 {
		cfg := ConfigFromEnv()
		log.Printf("[%s] using real Kafka producer, brokers=%v", ServiceName, cfg.Brokers)
		return NewKafkaProducer(cfg)
	}
	log.Printf("[%s] KAFKA_BROKERS not set, using channel-based producer", ServiceName)
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicClearingNovated, 1000)
	return p
}

// ClearingNovatedPayload is the event payload for ace.clearing.novated.
type ClearingNovatedPayload struct {
	ObligationID        string          `json:"obligation_id"`
	TradeID             string          `json:"trade_id"`
	InstrumentID        string          `json:"instrument_id"`
	BuyerParticipantID  string          `json:"buyer_participant_id"`
	SellerParticipantID string          `json:"seller_participant_id"`
	CCPID               string          `json:"ccp_id"`
	Price               string          `json:"price"`
	Quantity            uint64          `json:"quantity"`
	Status              string          `json:"status"`
	BuyerPosition       json.RawMessage `json:"buyer_position"`
	SellerPosition      json.RawMessage `json:"seller_position"`
	NovatedAt           string          `json:"novated_at"`
}

// TradeExecutedPayload mirrors the matching-engine's published payload.
type TradeExecutedPayload struct {
	TradeID              string `json:"trade_id"`
	InstrumentID         string `json:"instrument_id"`
	BuyOrderID           string `json:"buy_order_id"`
	SellOrderID          string `json:"sell_order_id"`
	BuyerParticipantID   string `json:"buyer_participant_id"`
	SellerParticipantID  string `json:"seller_participant_id"`
	Price                string `json:"price"`
	Quantity             uint64 `json:"quantity"`
	TradeValue           string `json:"trade_value"`
	AggressorSide        string `json:"aggressor_side"`
	TradeType            string `json:"trade_type"`
	SequenceNumber       uint64 `json:"sequence_number"`
	ExecutedAt           string `json:"executed_at"`
}

// PublishClearingNovated publishes a clearing novation event.
func PublishClearingNovated(p Producer, payload ClearingNovatedPayload, correlationID string) error {
	evt, err := NewEvent(TopicClearingNovated, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicClearingNovated, payload.InstrumentID, evt)
}

// TradeExecutedHandler returns a handler that processes trade execution events.
// The callback receives the decoded payload for integration with the clearing engine.
type TradeCallback func(ctx context.Context, payload TradeExecutedPayload, correlationID string) error

func TradeExecutedHandler(cb TradeCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload TradeExecutedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode trade payload: %w", err)
		}
		return cb(ctx, payload, event.CorrelationID)
	}
}

// SetupConsumer configures the clearing-engine consumer with all subscriptions.
func SetupConsumer(cfg ConsumerConfig, dlqProducer Producer) *ChannelConsumer {
	c := NewChannelConsumer(cfg, dlqProducer)
	return c
}

// NewConsumerFromEnv creates a Consumer based on environment configuration.
// If KAFKA_BROKERS is set, returns a real KafkaConsumer. Otherwise ChannelConsumer.
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
