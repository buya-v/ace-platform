package kafka

// Topic name constants for the securities-service event pipeline.
const (
	// TopicTradeExecuted is published when two orders are matched and a trade is created.
	TopicTradeExecuted = "ace-commodities.securities.trade.executed"

	// TopicOrderCreated is published when a new order is successfully submitted.
	TopicOrderCreated = "ace-commodities.securities.order.created"

	// TopicOrderCancelled is published when an order is cancelled by the participant.
	TopicOrderCancelled = "ace-commodities.securities.order.cancelled"
)
