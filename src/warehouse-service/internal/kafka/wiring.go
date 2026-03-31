package kafka

import (
	"log"
	"os"
	"strings"
)

// Warehouse-service Kafka wiring:
//   Producer: ace.warehouse.receipt-pledged (partition key: participant_id)
//   Producer: ace.warehouse.delivery-completed (partition key: instrument_id)

const ServiceName = "warehouse-service"

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
