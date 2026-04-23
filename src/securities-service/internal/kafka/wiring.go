package kafka

import (
	"fmt"

	"github.com/garudax-platform/securities-service/internal/types"
)

const source = "securities-service"

// PublishTradeExecuted wraps a SecurityTrade in a GarudaX event envelope and
// publishes it to TopicTradeExecuted. The trade ID is used as the partition key.
// If producer is nil the call is a no-op and returns nil.
func PublishTradeExecuted(p Producer, trade *types.SecurityTrade) error {
	if p == nil {
		return nil
	}
	event, err := NewEvent("trade.executed", source, trade.ID, trade)
	if err != nil {
		return fmt.Errorf("kafka wiring: build trade.executed event: %w", err)
	}
	return p.Publish(TopicTradeExecuted, trade.ID, event)
}

// PublishOrderCreated wraps a SecurityOrder in a GarudaX event envelope and
// publishes it to TopicOrderCreated. The order ID is used as the partition key.
// If producer is nil the call is a no-op and returns nil.
func PublishOrderCreated(p Producer, order *types.SecurityOrder) error {
	if p == nil {
		return nil
	}
	event, err := NewEvent("order.created", source, order.ID, order)
	if err != nil {
		return fmt.Errorf("kafka wiring: build order.created event: %w", err)
	}
	return p.Publish(TopicOrderCreated, order.ID, event)
}

// PublishOrderCancelled wraps a SecurityOrder in a GarudaX event envelope and
// publishes it to TopicOrderCancelled. The order ID is used as the partition key.
// If producer is nil the call is a no-op and returns nil.
func PublishOrderCancelled(p Producer, order *types.SecurityOrder) error {
	if p == nil {
		return nil
	}
	event, err := NewEvent("order.cancelled", source, order.ID, order)
	if err != nil {
		return fmt.Errorf("kafka wiring: build order.cancelled event: %w", err)
	}
	return p.Publish(TopicOrderCancelled, order.ID, event)
}
