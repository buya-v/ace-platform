package kafka

import (
	"context"
	"encoding/json"
	"fmt"
)

// Market-data-service Kafka wiring:
//   Consumer: ace.trades.executed
//   Consumer: ace.settlement.completed
//   Producer: ace.market-data.trade-ingested (internal, partition key: instrument_id)

const ServiceName = "market-data-service"

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

// SettlementCompletedPayload mirrors settlement-engine's published payload.
type SettlementCompletedPayload struct {
	CycleID           string                  `json:"cycle_id"`
	SettleDate        string                  `json:"settle_date"`
	Status            string                  `json:"status"`
	SettlementPrices  []SettlementPriceEntry  `json:"settlement_prices"`
	TotalPayIn        string                  `json:"total_pay_in"`
	TotalPayOut       string                  `json:"total_pay_out"`
	InstructionsCount int                     `json:"instructions_count"`
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
