package kafka

import (
	"log"
	"os"
	"strings"
	"testing"
)

// Matching-engine Kafka wiring:
//   Producer: ace-commodities.trades.executed (partition key: instrument_id)

const (
	ServiceName = "matching-engine"
)

// kafkaBrokersConfigured reports whether the KAFKA_BROKERS environment variable
// is set to a non-empty (non-whitespace) value.
func kafkaBrokersConfigured() bool {
	return strings.TrimSpace(os.Getenv("KAFKA_BROKERS")) != ""
}

// NewProducerFromEnv creates a Producer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaProducer using
// kafka-go Writer. If KAFKA_BROKERS is unset, it FAILS FAST (log.Fatal) outside
// of unit tests: each service runs as a separate process and the in-process
// channel adapter does not cross process boundaries, so silently falling back to
// it would drop matching->clearing->margin->settlement events. The in-process
// adapter is reachable only under testing.Testing() via the explicit test
// constructor NewTradeProducer.
func NewProducerFromEnv() Producer {
	if kafkaBrokersConfigured() {
		cfg := ConfigFromEnv()
		log.Printf("[%s] using real Kafka producer, brokers=%v", ServiceName, cfg.Brokers)
		return NewKafkaProducer(cfg)
	}
	if !testing.Testing() {
		log.Fatalf("[%s] KAFKA_BROKERS is required but not set; refusing to fall back to the in-process channel producer in a multi-process deployment — cross-service events would be silently dropped. Set KAFKA_BROKERS, or build the in-process adapter explicitly via NewTradeProducer in unit tests.", ServiceName)
	}
	log.Printf("[%s] KAFKA_BROKERS not set; using in-process channel producer (TEST MODE ONLY)", ServiceName)
	return NewTradeProducer(DefaultProducerConfig())
}

// TradeExecutedPayload is the event payload for ace-commodities.trades.executed.
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

// NewTradeProducer creates a producer configured for trade execution events.
func NewTradeProducer(cfg ProducerConfig) *ChannelProducer {
	p := NewChannelProducer(cfg)
	p.RegisterTopic(TopicTradesExecuted, 1000)
	return p
}

// PublishTradeExecuted publishes a trade execution event.
func PublishTradeExecuted(p Producer, payload TradeExecutedPayload, correlationID string) error {
	evt, err := NewEvent(TopicTradesExecuted, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicTradesExecuted, payload.InstrumentID, evt)
}
