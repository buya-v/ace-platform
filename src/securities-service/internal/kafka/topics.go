package kafka

// TopicTradeExecuted returns the Kafka topic name for trade execution events
// scoped to the given tenant.
func TopicTradeExecuted(tenantID string) string {
	return tenantID + ".securities.trade.executed"
}

// TopicOrderCreated returns the Kafka topic name for order creation events
// scoped to the given tenant.
func TopicOrderCreated(tenantID string) string {
	return tenantID + ".securities.order.created"
}

// TopicOrderCancelled returns the Kafka topic name for order cancellation events
// scoped to the given tenant.
func TopicOrderCancelled(tenantID string) string {
	return tenantID + ".securities.order.cancelled"
}
