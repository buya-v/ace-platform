package kafka

import (
	"log"
	"os"
	"strings"
	"testing"
)

// Warehouse-service Kafka wiring:
//   Producer: ace.warehouse.receipt-pledged (partition key: participant_id)
//   Producer: ace.warehouse.delivery-completed (partition key: instrument_id)

const ServiceName = "warehouse-service"

// kafkaBrokersConfigured reports whether the KAFKA_BROKERS environment variable
// is set to a non-empty (non-whitespace) value.
func kafkaBrokersConfigured() bool {
	return strings.TrimSpace(os.Getenv("KAFKA_BROKERS")) != ""
}

// NewProducerFromEnv creates a Producer based on environment configuration.
// If KAFKA_BROKERS is set and non-empty, returns a real KafkaProducer. If it is
// unset, it FAILS FAST (log.Fatal) outside of unit tests rather than silently
// returning the in-process channel adapter, which would drop cross-service
// events in a multi-process deployment. The in-process adapter is reachable only
// under testing.Testing() via the explicit test constructor newInProcessProducer.
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

// newInProcessProducer builds the in-process channel-based producer used ONLY by
// unit tests. Production code reaches it only via NewProducerFromEnv under
// testing.Testing(); the in-process adapter must never be used in a
// multi-process deployment, where Go channels do not cross process boundaries.
func newInProcessProducer() *ChannelProducer {
	p := NewChannelProducer(DefaultProducerConfig())
	p.RegisterTopic(TopicWarehouseReceiptPledged, 1000)
	p.RegisterTopic(TopicWarehouseDeliveryCompleted, 1000)
	return p
}

// ReceiptPledgedPayload is the event payload for ace.warehouse.receipt-pledged.
type ReceiptPledgedPayload struct {
	ReceiptID       string  `json:"receipt_id"`
	ParticipantID   string  `json:"participant_id"`
	Commodity       string  `json:"commodity"`
	QuantityMT      float64 `json:"quantity_mt"`
	WarehouseID     string  `json:"warehouse_id"`
	Grade           string  `json:"grade"`
	CollateralValue string  `json:"collateral_value"`
	PledgedAt       string  `json:"pledged_at"`
}

// DeliveryCompletedPayload is the event payload for ace.warehouse.delivery-completed.
type DeliveryCompletedPayload struct {
	DeliveryID          string  `json:"delivery_id"`
	ReceiptID           string  `json:"receipt_id"`
	InstrumentID        string  `json:"instrument_id"`
	BuyerParticipantID  string  `json:"buyer_participant_id"`
	SellerParticipantID string  `json:"seller_participant_id"`
	QuantityMT          float64 `json:"quantity_mt"`
	WarehouseID         string  `json:"warehouse_id"`
	CompletedAt         string  `json:"completed_at"`
}

// PublishReceiptPledged publishes a warehouse receipt pledged event.
func PublishReceiptPledged(p Producer, payload ReceiptPledgedPayload, correlationID string) error {
	evt, err := NewEvent(TopicWarehouseReceiptPledged, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicWarehouseReceiptPledged, payload.ParticipantID, evt)
}

// PublishDeliveryCompleted publishes a delivery completed event.
func PublishDeliveryCompleted(p Producer, payload DeliveryCompletedPayload, correlationID string) error {
	evt, err := NewEvent(TopicWarehouseDeliveryCompleted, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicWarehouseDeliveryCompleted, payload.InstrumentID, evt)
}
