package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// instrumentAliases maps short names to full instrument IDs.
var instrumentAliases = map[string]string{
	"wheat":     "WHT-HRW-2026M07-UB",
	"wht":       "WHT-HRW-2026M07-UB",
	"corn":      "CRN-YEL-2026M09-UB",
	"crn":       "CRN-YEL-2026M09-UB",
	"soybeans":  "SBN-NO2-2026M11-UB",
	"soybean":   "SBN-NO2-2026M11-UB",
	"soy":       "SBN-NO2-2026M11-UB",
	"sbn":       "SBN-NO2-2026M11-UB",
	"barley":    "BRL-MALT-2026M07-UB",
	"brl":       "BRL-MALT-2026M07-UB",
	"cashmere":  "CSH-RAW-2026M09-UB",
	"csh":       "CSH-RAW-2026M09-UB",
	"cattle":    "LVS-CATTLE-2026M10-UB",
	"livestock": "LVS-CATTLE-2026M10-UB",
	"lvs":       "LVS-CATTLE-2026M10-UB",
}

// ActionExecutor executes bot actions by calling gateway endpoints internally.
type ActionExecutor struct {
	gatewayAddr string // e.g., "http://127.0.0.1:8080"
	client      *http.Client
}

// NewActionExecutor creates an executor that calls the gateway.
func NewActionExecutor(gatewayAddr string) *ActionExecutor {
	if gatewayAddr == "" {
		gatewayAddr = "http://127.0.0.1:8080"
	}
	return &ActionExecutor{
		gatewayAddr: gatewayAddr,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Execute processes a message and executes the appropriate action using the user's token.
func (e *ActionExecutor) Execute(message, userToken string) ChatResponse {
	lower := strings.ToLower(strings.TrimSpace(message))

	// Halt instrument
	if strings.HasPrefix(lower, "halt ") {
		name := strings.TrimPrefix(lower, "halt ")
		name = strings.TrimPrefix(name, "trading on ")
		name = strings.TrimPrefix(name, "trading ")
		instrumentID := resolveInstrument(strings.TrimSpace(name))
		if instrumentID == "" {
			return ChatResponse{Reply: fmt.Sprintf("Unknown instrument: '%s'. Try: wheat, corn, soybeans, barley, cashmere, cattle, or use full ID.", name)}
		}
		body, status := e.doRequest("POST", "/api/v1/admin/instruments/"+instrumentID+"/halt", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("✅ Trading HALTED on %s.", instrumentID),
				Actions: []Action{{Label: "Circuit Breakers", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to halt %s: %s", instrumentID, body)}
	}

	// Resume instrument
	if strings.HasPrefix(lower, "resume ") {
		name := strings.TrimPrefix(lower, "resume ")
		name = strings.TrimPrefix(name, "trading on ")
		name = strings.TrimPrefix(name, "trading ")
		instrumentID := resolveInstrument(strings.TrimSpace(name))
		if instrumentID == "" {
			return ChatResponse{Reply: fmt.Sprintf("Unknown instrument: '%s'.", name)}
		}
		body, status := e.doRequest("POST", "/api/v1/admin/instruments/"+instrumentID+"/resume", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("✅ Trading RESUMED on %s.", instrumentID),
				Actions: []Action{{Label: "Circuit Breakers", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to resume %s: %s", instrumentID, body)}
	}

	// Approve KYC
	if matchApprove := regexp.MustCompile(`(?i)^approve\s+(?:trader|participant|kyc|application)?\s*(.+)`).FindStringSubmatch(message); len(matchApprove) > 1 {
		pid := strings.TrimSpace(matchApprove[1])
		_, status := e.doRequest("POST", "/api/v1/participants/"+pid+"/approve", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("✅ KYC APPROVED for participant %s.", pid)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to approve %s. Check the participant ID.", pid)}
	}

	// Reject KYC
	if matchReject := regexp.MustCompile(`(?i)^reject\s+(?:trader|participant|kyc|application)?\s*(\S+)\s*(?:because|reason:?)?\s*(.*)`).FindStringSubmatch(message); len(matchReject) > 1 {
		pid := strings.TrimSpace(matchReject[1])
		reason := strings.TrimSpace(matchReject[2])
		if reason == "" {
			reason = "Rejected by admin"
		}
		_, status := e.doRequest("POST", "/api/v1/participants/"+pid+"/reject", map[string]string{"reason": reason}, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("✅ KYC REJECTED for %s. Reason: %s", pid, reason)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to reject %s.", pid)}
	}

	// Mass cancel
	if containsAny(lower, "mass cancel", "cancel all") {
		_, status := e.doRequest("POST", "/api/v1/admin/mass-cancel", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: "✅ Mass cancel executed. All open orders cancelled."}
		}
		return ChatResponse{Reply: "❌ Mass cancel failed."}
	}

	// System health
	if containsAny(lower, "health", "status", "services") {
		body, status := e.doRequest("GET", "/api/v1/admin/health", nil, userToken)
		if status >= 200 && status < 300 {
			return formatHealthResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch system health."}
	}

	// Margin calls
	if containsAny(lower, "margin") {
		body, status := e.doRequest("GET", "/api/v1/margin/calls/stats", nil, userToken)
		if status >= 200 && status < 300 {
			return formatMarginResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch margin data."}
	}

	// Settlement
	if lower == "run settlement" || lower == "trigger settlement" {
		_, status := e.doRequest("POST", "/api/v1/settlement/cycle", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: "✅ Settlement cycle triggered."}
		}
		return ChatResponse{Reply: "❌ Failed to trigger settlement."}
	}
	if containsAny(lower, "settlement") {
		body, status := e.doRequest("GET", "/api/v1/settlement/cycles", nil, userToken)
		if status >= 200 && status < 300 {
			return formatSettlementResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch settlement data."}
	}

	// Alerts
	if containsAny(lower, "alert") {
		body, status := e.doRequest("GET", "/api/v1/compliance/alerts", nil, userToken)
		if status >= 200 && status < 300 {
			return formatAlertsResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch alerts."}
	}

	// Participants / KYC
	if containsAny(lower, "participant", "kyc", "pending application", "pending kyc") {
		body, status := e.doRequest("GET", "/api/v1/participants", nil, userToken)
		if status >= 200 && status < 300 {
			return formatParticipantsResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch participants."}
	}

	// Instruments
	if containsAny(lower, "instrument", "commodity", "contract") {
		body, status := e.doRequest("GET", "/api/v1/instruments/list", nil, userToken)
		if status >= 200 && status < 300 {
			return formatInstrumentsResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch instruments."}
	}

	// Order book / price
	for alias, id := range instrumentAliases {
		if strings.Contains(lower, alias+" price") || strings.Contains(lower, alias+" ticker") {
			body, status := e.doRequest("GET", "/api/v1/market-data/ticker/"+id, nil, userToken)
			if status >= 200 && status < 300 {
				return ChatResponse{Reply: fmt.Sprintf("📊 %s ticker:\n%s", id, prettyJSON(body))}
			}
		}
		if strings.Contains(lower, alias+" book") || strings.Contains(lower, alias+" orderbook") || strings.Contains(lower, alias+" order book") {
			body, status := e.doRequest("GET", "/api/v1/instruments/"+id+"/book", nil, userToken)
			if status >= 200 && status < 300 {
				return ChatResponse{Reply: fmt.Sprintf("📋 %s order book:\n%s", id, prettyJSON(body))}
			}
		}
	}

	// Warehouse
	if containsAny(lower, "inventory", "warehouse") {
		body, status := e.doRequest("GET", "/api/v1/warehouse/inventory", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("🏭 Warehouse inventory:\n%s", prettyJSON(body))}
		}
	}

	// Tickets
	if containsAny(lower, "ticket") {
		body, status := e.doRequest("GET", "/api/v1/tickets", nil, userToken)
		if status >= 200 && status < 300 {
			return formatTicketsResponse(body)
		}
	}

	// Fees
	if containsAny(lower, "fee") {
		body, status := e.doRequest("GET", "/api/v1/admin/fees", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("💰 Fee schedule:\n%s", prettyJSON(body))}
		}
	}

	// Positions
	if containsAny(lower, "position") {
		body, status := e.doRequest("GET", "/api/v1/clearing/positions", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("📊 Clearing positions:\n%s", prettyJSON(body))}
		}
	}

	// Help
	if containsAny(lower, "help", "what can you do") {
		return ChatResponse{
			Reply: "I can execute these actions for you:\n\n" +
				"📊 **Trading**: `halt wheat`, `resume corn`, `show instruments`\n" +
				"👥 **KYC**: `show pending KYC`, `approve trader ABC`, `reject trader XYZ`\n" +
				"💰 **Margin**: `show margin calls`, `recalculate margin`\n" +
				"⚖️ **Settlement**: `run settlement`, `show settlement cycles`\n" +
				"🔍 **Compliance**: `show alerts`, `resolve alert 123`\n" +
				"📈 **Market**: `wheat price`, `corn order book`\n" +
				"🎫 **Tickets**: `show tickets`, `report a bug`\n" +
				"🏥 **System**: `system health`\n\n" +
				"Just tell me what you need in plain language!",
		}
	}

	// Default
	return ChatResponse{
		Reply: "I didn't understand that. Try:\n• `halt wheat` — stop trading\n• `show margin calls` — view margin status\n• `approve trader ABC` — approve KYC\n• `system health` — check services\n• `help` — see all commands",
	}
}

// doRequest makes an HTTP request to the gateway using the user's token.
func (e *ActionExecutor) doRequest(method, path string, body map[string]string, token string) (string, int) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = strings.NewReader(string(b))
	}

	req, err := http.NewRequest(method, e.gatewayAddr+path, reqBody)
	if err != nil {
		return err.Error(), 0
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err.Error(), 0
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	return string(data), resp.StatusCode
}

// resolveInstrument maps aliases to full instrument IDs.
func resolveInstrument(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Direct match
	if id, ok := instrumentAliases[name]; ok {
		return id
	}
	// Check if it's already a full ID
	if strings.Contains(strings.ToUpper(name), "-") && len(name) > 10 {
		return strings.ToUpper(name)
	}
	return ""
}

func prettyJSON(raw string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	if len(b) > 1000 {
		return string(b[:1000]) + "..."
	}
	return string(b)
}

func formatHealthResponse(raw string) ChatResponse {
	var data struct {
		OverallStatus string `json:"overall_status"`
		Services      []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"services"`
	}
	json.Unmarshal([]byte(raw), &data)

	lines := []string{fmt.Sprintf("🏥 System Health: %s", strings.ToUpper(data.OverallStatus))}
	for _, s := range data.Services {
		icon := "✅"
		if s.Status != "healthy" {
			icon = "❌"
		}
		lines = append(lines, fmt.Sprintf("  %s %s", icon, s.Name))
	}
	return ChatResponse{Reply: strings.Join(lines, "\n")}
}

func formatMarginResponse(raw string) ChatResponse {
	var data map[string]interface{}
	json.Unmarshal([]byte(raw), &data)
	return ChatResponse{
		Reply: fmt.Sprintf("💰 Margin Status:\n  Active calls: %v\n  Total shortfall: %v\n  Participants in call: %v",
			data["total_active"], data["total_shortfall"], data["participants_in_call"]),
		Actions: []Action{{Label: "Margin Calls", Type: "link", URL: "/dashboard/margin"}},
	}
}

func formatSettlementResponse(raw string) ChatResponse {
	var data map[string]interface{}
	json.Unmarshal([]byte(raw), &data)
	return ChatResponse{
		Reply:   fmt.Sprintf("⚖️ Settlement cycles:\n%s", prettyJSON(raw)),
		Actions: []Action{{Label: "Settlement", Type: "link", URL: "/dashboard/settlement"}},
	}
}

func formatAlertsResponse(raw string) ChatResponse {
	var data map[string]interface{}
	json.Unmarshal([]byte(raw), &data)
	alerts, _ := data["alerts"].([]interface{})
	if alerts == nil {
		if d, ok := data["data"].([]interface{}); ok {
			alerts = d
		}
	}
	return ChatResponse{
		Reply:   fmt.Sprintf("🔍 Compliance Alerts: %d total\n%s", len(alerts), prettyJSON(raw)),
		Actions: []Action{{Label: "Surveillance", Type: "link", URL: "/dashboard/surveillance"}},
	}
}

func formatParticipantsResponse(raw string) ChatResponse {
	var data map[string]interface{}
	json.Unmarshal([]byte(raw), &data)
	participants, _ := data["data"].([]interface{})
	if participants == nil {
		if d, ok := data["applications"].([]interface{}); ok {
			participants = d
		}
	}
	return ChatResponse{
		Reply:   fmt.Sprintf("👥 Participants: %d total\n%s", len(participants), prettyJSON(raw)),
		Actions: []Action{{Label: "Participants", Type: "link", URL: "/dashboard/participants"}},
	}
}

func formatInstrumentsResponse(raw string) ChatResponse {
	var data interface{}
	json.Unmarshal([]byte(raw), &data)
	return ChatResponse{
		Reply:   fmt.Sprintf("📊 Active instruments:\n%s", prettyJSON(raw)),
		Actions: []Action{{Label: "Order Book", Type: "link", URL: "/dashboard/orderbook"}},
	}
}

func formatTicketsResponse(raw string) ChatResponse {
	var data map[string]interface{}
	json.Unmarshal([]byte(raw), &data)
	tickets, _ := data["data"].([]interface{})
	return ChatResponse{
		Reply:   fmt.Sprintf("🎫 Tickets: %d total\n%s", len(tickets), prettyJSON(raw)),
		Actions: []Action{{Label: "Tickets", Type: "link", URL: "/dashboard/tickets"}},
	}
}
