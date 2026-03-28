package kafka

import (
	"context"
	"encoding/json"
	"fmt"
)

// Margin-engine Kafka wiring:
//   Consumer: ace.clearing.novated
//   Consumer: ace.warehouse.receipt-pledged
//   Producer: ace.margin.call-issued (partition key: participant_id)

const ServiceName = "margin-engine"

// MarginCallIssuedPayload is the event payload for ace.margin.call-issued.
type MarginCallIssuedPayload struct {
	MarginCallID      string `json:"margin_call_id"`
	ParticipantID     string `json:"participant_id"`
	InstrumentID      string `json:"instrument_id"`
	CallType          string `json:"call_type"`
	RequiredAmount    string `json:"required_amount"`
	CurrentMargin     string `json:"current_margin"`
	MaintenanceMargin string `json:"maintenance_margin"`
	Deficit           string `json:"deficit"`
	Status            string `json:"status"`
	Deadline          string `json:"deadline"`
	IssuedAt          string `json:"issued_at"`
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

// WarehouseReceiptPledgedPayload mirrors warehouse-service's published payload.
type WarehouseReceiptPledgedPayload struct {
	ReceiptID       string  `json:"receipt_id"`
	ParticipantID   string  `json:"participant_id"`
	Commodity       string  `json:"commodity"`
	QuantityMT      float64 `json:"quantity_mt"`
	WarehouseID     string  `json:"warehouse_id"`
	Grade           string  `json:"grade"`
	CollateralValue string  `json:"collateral_value"`
	PledgedAt       string  `json:"pledged_at"`
}

// PublishMarginCallIssued publishes a margin call event.
func PublishMarginCallIssued(p Producer, payload MarginCallIssuedPayload, correlationID string) error {
	evt, err := NewEvent(TopicMarginCallIssued, ServiceName, correlationID, payload)
	if err != nil {
		return err
	}
	return p.Publish(TopicMarginCallIssued, payload.ParticipantID, evt)
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

// ReceiptPledgedHandler returns a handler for ace.warehouse.receipt-pledged events.
type ReceiptPledgedCallback func(ctx context.Context, payload WarehouseReceiptPledgedPayload, correlationID string) error

func ReceiptPledgedHandler(cb ReceiptPledgedCallback) Handler {
	return func(ctx context.Context, event Event) error {
		var payload WarehouseReceiptPledgedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode receipt-pledged payload: %w", err)
		}
		return cb(ctx, payload, event.CorrelationID)
	}
}
