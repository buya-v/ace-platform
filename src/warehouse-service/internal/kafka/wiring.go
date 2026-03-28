package kafka

// Warehouse-service Kafka wiring:
//   Producer: ace.warehouse.receipt-pledged (partition key: participant_id)
//   Producer: ace.warehouse.delivery-completed (partition key: instrument_id)

const ServiceName = "warehouse-service"

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
