package kafka

import (
	"fmt"

	"github.com/garudax-platform/securities-service/internal/types"
)

const source = "securities-service"

// PublishTradeExecuted wraps a SecurityTrade in a GarudaX event envelope and
// publishes it to the tenant-scoped TopicTradeExecuted topic. The trade ID is
// used as the partition key. If producer is nil the call is a no-op and returns nil.
func PublishTradeExecuted(p Producer, tenantID string, trade *types.SecurityTrade) error {
	if p == nil {
		return nil
	}
	event, err := NewEvent("trade.executed", source, trade.ID, trade)
	if err != nil {
		return fmt.Errorf("kafka wiring: build trade.executed event: %w", err)
	}
	return p.Publish(TopicTradeExecuted(tenantID), trade.ID, event)
}

// PublishOrderCreated wraps a SecurityOrder in a GarudaX event envelope and
// publishes it to the tenant-scoped TopicOrderCreated topic. The order ID is
// used as the partition key. If producer is nil the call is a no-op and returns nil.
func PublishOrderCreated(p Producer, tenantID string, order *types.SecurityOrder) error {
	if p == nil {
		return nil
	}
	event, err := NewEvent("order.created", source, order.ID, order)
	if err != nil {
		return fmt.Errorf("kafka wiring: build order.created event: %w", err)
	}
	return p.Publish(TopicOrderCreated(tenantID), order.ID, event)
}

// PublishOrderCancelled wraps a SecurityOrder in a GarudaX event envelope and
// publishes it to the tenant-scoped TopicOrderCancelled topic. The order ID is
// used as the partition key. If producer is nil the call is a no-op and returns nil.
func PublishOrderCancelled(p Producer, tenantID string, order *types.SecurityOrder) error {
	if p == nil {
		return nil
	}
	event, err := NewEvent("order.cancelled", source, order.ID, order)
	if err != nil {
		return fmt.Errorf("kafka wiring: build order.cancelled event: %w", err)
	}
	return p.Publish(TopicOrderCancelled(tenantID), order.ID, event)
}
