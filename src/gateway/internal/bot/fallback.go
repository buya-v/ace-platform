package bot

import (
	"strings"
)

// FallbackResponse generates a built-in keyword response for MVP mode
// (when no orchestrator is available).
func FallbackResponse(message string) string {
	lower := strings.ToLower(message)

	switch {
	case containsAny(lower, "health", "status"):
		return "System health check: All core services (matching-engine, clearing-engine, margin-engine, settlement-engine) are monitored. Use the admin health endpoint or check the dashboard for real-time status."

	case containsAny(lower, "alert", "alerts"):
		return "To view compliance alerts, check the Surveillance page or use GET /api/v1/compliance/alerts. You can filter by status and resolve alerts from the admin panel."

	case containsAny(lower, "margin"):
		return "Margin status: Use the Margin page to view current margin calls and shortfalls. GET /api/v1/margin/calls/stats provides aggregate margin call statistics."

	case containsAny(lower, "ticket", "bug", "report"):
		return "To create a support ticket, go to the Tickets page and click 'New Ticket', or use POST /api/v1/tickets with title, description, and category (bug_report, support, feature_request, customization)."

	case containsAny(lower, "settlement", "settle"):
		return "Settlement cycles can be viewed on the Settlement page. GET /api/v1/settlement/cycles lists all cycles. Contact an admin to trigger a new settlement cycle."

	case containsAny(lower, "order", "trade", "trading"):
		return "Trading operations: Submit orders via POST /api/v1/orders, view the order book on the Dashboard, and check trade history on the Market Data page."

	case containsAny(lower, "kyc", "participant", "onboard"):
		return "KYC and participant management: View pending applications on the Participants page. Use POST /api/v1/participants to submit new applications."

	case containsAny(lower, "help"):
		return "I can help with: system health, alerts, margin status, settlement, trading, KYC/participants, and support tickets. What would you like to know?"

	default:
		return "I can help with system health, alerts, margin status, and tickets. What would you like to know?"
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
