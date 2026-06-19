package kafka

import (
	"strings"
	"testing"
)

// TestTenantID pins the tenant this deployment is scoped to. mse-equities runs
// as a separate deployment; if this value ever changes here it must change
// deliberately, not by accident.
func TestTenantID(t *testing.T) {
	if TenantID != "ace-commodities" {
		t.Fatalf("TenantID = %q, want %q", TenantID, "ace-commodities")
	}
}

// TestTopicsAreTenantScoped enforces the platform invariant that every topic
// this service produces to or consumes from is prefixed with the active tenant
// and follows the canonical {tenant_id}.{domain}.{event} shape. This guards
// against a regression that would let an ACE handler read or write a bare or
// cross-tenant topic.
func TestTopicsAreTenantScoped(t *testing.T) {
	topics := map[string]string{
		"TopicTradesExecuted":             TopicTradesExecuted,
		"TopicClearingNovated":            TopicClearingNovated,
		"TopicMarginCallIssued":           TopicMarginCallIssued,
		"TopicSettlementCompleted":        TopicSettlementCompleted,
		"TopicComplianceChanged":          TopicComplianceChanged,
		"TopicMarketDataIngested":         TopicMarketDataIngested,
		"TopicWarehouseReceiptPledged":    TopicWarehouseReceiptPledged,
		"TopicWarehouseDeliveryCompleted": TopicWarehouseDeliveryCompleted,
		"TopicAuthUserRegistered":         TopicAuthUserRegistered,
	}

	wantPrefix := TenantID + "."
	for name, topic := range topics {
		if !strings.HasPrefix(topic, wantPrefix) {
			t.Errorf("%s = %q is not scoped to tenant %q", name, topic, TenantID)
		}
		// {tenant_id}.{domain}.{event} → at least three dot-separated segments.
		if parts := strings.Split(topic, "."); len(parts) < 3 {
			t.Errorf("%s = %q does not follow {tenant_id}.{domain}.{event}", name, topic)
		}
	}
}

// TestDLQTopicIsTenantScoped verifies the dead-letter prefix derives from the
// active tenant, so failed records never spill into another tenant's DLQ.
func TestDLQTopicIsTenantScoped(t *testing.T) {
	got := TenantID + ".dlq." + topicWithoutPrefix(TopicClearingNovated)
	want := "ace-commodities.dlq.clearing.novated"
	if got != want {
		t.Errorf("DLQ topic = %q, want %q", got, want)
	}
}
