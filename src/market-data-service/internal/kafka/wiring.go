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

// Market-data-service Kafka wiring:
//   Consumer: ace.trades.executed
//   Consumer: ace.settlement.completed
//   Producer: ace.market-data.trade-ingested (internal, partition key: instrument_id)

const ServiceName = "market-data-service"

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
	p.RegisterTopic(TopicMarketDataIngested, 1000)
	return p
}

// NewConsumerFromEnv creates a Consumer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaConsumer using
// kafka-go Reader. Otherwise returns a ChannelConsumer for local/test use.
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

// TradeIngestedPayload is the event payload for ace.market-data.trade-ingested.
type TradeIngestedPayload struct {
	InstrumentID string `json:"instrument_id"`
	TradeID      string `json:"trade_id"`
	Price        string `json:"price"`
	Quantity     uint64 `json:"quantity"`
	Side         string `json:"side"`
	IngestedAt   string `json:"ingested_at"`
}

// TradeExecutedPayload mirrors matching-engine's published payload.
type TradeExecutedPayload struct {
	TradeID             string `json:"trade_id"`
	InstrumentID        string `json:"instrument_id"`
	BuyOrderID          string `json:"buy_order_id"`
	SellOrderID         string `json:"sell_order_id"`
	BuyerParticipantID  string `json:"buyer_participant_id"`
	SellerParticipantID string `json:"seller_participant_id"`
	Price               string `json:"price"`
	Quantity            uint64 `json:"quantity"`
	TradeValue          string `json:"trade_value"`
	AggressorSide       string `json:"aggressor_side"`
	TradeType           string `json:"trade_type"`
	SequenceNumber      uint64 `json:"sequence_number"`
	ExecutedAt          string `json:"executed_at"`
}

// SettlementCompletedPayload mirrors settlement-engine's published payload.
type SettlementCompletedPayload struct {
	CycleID           string                 `json:"cycle_id"`
	SettleDate        string                 `json:"settle_date"`
	Status            string                 `json:"status"`
	SettlementPrices  []SettlementPriceEntry `json:"settlement_prices"`
	TotalPayIn        string                 `json:"total_pay_in"`
	TotalPayOut       string                 `json:"total_pay_out"`
	InstructionsCount int                    `json:"instructions_count"`
}

// SettlementPriceEntry is a single instrument's settlement price.
type SettlementPriceEntry struct {
	InstrumentID    string `json:"instrument_id"`
	SettlementPrice string `json:"settlement_price"`
	PreviousPrice   string `json:"previous_price"`
}

// PublishTradeIngested publishes an internal trade-ingested event.
func PublishTradeIngested(p Producer, payload TradeIngestedPayload, correlationID string) error {
	evt, err := NewEvent(TopicMarketDataIngested, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicMarketDataIngested, payload.InstrumentID, evt)
}

// TradeExecutedHandler returns a handler for ace.trades.executed events.
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

// SettlementCompletedHandler returns a handler for ace.settlement.completed events.
type SettlementCallback func(ctx context.Context, payload SettlementCompletedPayload, correlationID string) error

func SettlementCompletedHandler(cb SettlementCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload SettlementCompletedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode settlement payload: %w", err)
		}
		return cb(ctx, payload, event.CorrelationID)
	}
}
