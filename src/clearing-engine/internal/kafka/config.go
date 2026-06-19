package kafka

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// TenantID is the GarudaX tenant this deployment serves. Every Kafka topic
// this service produces to or consumes from is scoped under this tenant, per
// the platform invariant that tenant ID is never optional. A second tenant
// (e.g. "mse-equities") runs as a separate deployment with its own TenantID;
// changing this one constant re-scopes every topic below to that tenant.
const TenantID = "ace-commodities"

// Topics used by the GarudaX platform. Each name follows the canonical
// {tenant_id}.{domain}.{event} convention and is derived from TenantID so the
// tenant prefix is single-sourced and cannot drift across services.
const (
	TopicTradesExecuted             = TenantID + ".trades.executed"
	TopicClearingNovated            = TenantID + ".clearing.novated"
	TopicMarginCallIssued           = TenantID + ".margin.call-issued"
	TopicSettlementCompleted        = TenantID + ".settlement.completed"
	TopicComplianceChanged          = TenantID + ".compliance.status-changed"
	TopicMarketDataIngested         = TenantID + ".market-data.trade-ingested"
	TopicWarehouseReceiptPledged    = TenantID + ".warehouse.receipt-pledged"
	TopicWarehouseDeliveryCompleted = TenantID + ".warehouse.delivery-completed"
	TopicAuthUserRegistered         = TenantID + ".auth.user-registered"
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
