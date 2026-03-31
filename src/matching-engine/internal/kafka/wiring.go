package kafka

import (
	"log"
	"os"
	"strings"
)

// Matching-engine Kafka wiring:
//   Producer: ace.trades.executed (partition key: instrument_id)

const (
	ServiceName = "matching-engine"
)

// NewProducerFromEnv creates a Producer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaProducer using
// kafka-go Writer. Otherwise returns a ChannelProducer for local/test use.
func NewProducerFromEnv() Producer {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers != "" && len(strings.TrimSpace(brokers)) > 0 {
		cfg := ConfigFromEnv()
		log.Printf("[%s] using real Kafka producer, brokers=%v", ServiceName, cfg.Brokers)
		return NewKafkaProducer(cfg)
	}
	log.Printf("[%s] KAFKA_BROKERS not set, using channel-based producer", ServiceName)
	return NewTradeProducer(DefaultProducerConfig())
}

// TradeExecutedPayload is the event payload for ace.trades.executed.
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
