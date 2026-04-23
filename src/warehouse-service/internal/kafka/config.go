package kafka

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Topics used by the GarudaX platform.
const (
	TopicTradesExecuted        = "ace-commodities.trades.executed"
	TopicClearingNovated       = "ace-commodities.clearing.novated"
	TopicMarginCallIssued      = "ace-commodities.margin.call-issued"
	TopicSettlementCompleted   = "ace-commodities.settlement.completed"
	TopicComplianceChanged     = "ace-commodities.compliance.status-changed"
	TopicMarketDataIngested    = "ace-commodities.market-data.trade-ingested"
	TopicWarehouseReceiptPledged   = "ace-commodities.warehouse.receipt-pledged"
	TopicWarehouseDeliveryCompleted = "ace-commodities.warehouse.delivery-completed"
	TopicAuthUserRegistered    = "ace-commodities.auth.user-registered"
)

// ConfigFromEnv loads Kafka configuration from environment variables.
// KAFKA_BROKERS: comma-separated broker list (default: localhost:9092)
// KAFKA_MAX_RETRIES: max publish/consume retries (default: 3)
// KAFKA_RETRY_BACKOFF_MS: base retry backoff in milliseconds (default: 100)
func ConfigFromEnv() ProducerConfig {
	cfg := DefaultProducerConfig()
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Brokers = strings.Split(brokers, ",")
	}
	if v := os.Getenv("KAFKA_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxRetries = n
		}
	}
	if v := os.Getenv("KAFKA_RETRY_BACKOFF_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RetryBackoff = time.Duration(n) * time.Millisecond
		}
	}
	return cfg
}

// ConsumerConfigFromEnv loads consumer configuration from environment variables.
// KAFKA_BROKERS, KAFKA_MAX_RETRIES, KAFKA_RETRY_BACKOFF_MS as above.
// KAFKA_MAX_TRACKED_IDS: max number of event IDs to track for idempotency (default: 100000)
func ConsumerConfigFromEnv(groupID string) ConsumerConfig {
	cfg := DefaultConsumerConfig(groupID)
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Brokers = strings.Split(brokers, ",")
	}
	if v := os.Getenv("KAFKA_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxRetries = n
		}
	}
	if v := os.Getenv("KAFKA_RETRY_BACKOFF_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RetryBackoff = time.Duration(n) * time.Millisecond
		}
	}
	if v := os.Getenv("KAFKA_MAX_TRACKED_IDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxTrackedIDs = n
		}
	}
	return cfg
}
