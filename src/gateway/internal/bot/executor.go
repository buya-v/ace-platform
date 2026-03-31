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

	// --- Profile ---
	// "who am I", "my profile", "whoami"
	if containsAny(lower, "who am i", "whoami", "my profile") {
		body, status := e.doRequest("GET", "/api/v1/auth/me", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("👤 Your profile:\n%s", prettyJSON(body))}
		}
		return ChatResponse{Reply: "❌ Unable to fetch profile."}
	}

	// --- Orders: buy / sell ---
	// Pattern: "buy 10 wheat at 325" or "sell 5 corn at 450"
	if reBuy := regexp.MustCompile(`(?i)^(buy|sell)\s+(\d+(?:\.\d+)?)\s+(\w+)\s+at\s+(\d+(?:\.\d+)?)`).FindStringSubmatch(message); len(reBuy) == 5 {
		side := strings.ToUpper(reBuy[1])
		qty := reBuy[2]
		instrumentName := reBuy[3]
		price := reBuy[4]
		instrumentID := resolveInstrument(instrumentName)
		if instrumentID == "" {
			return ChatResponse{Reply: fmt.Sprintf("Unknown instrument '%s'. Try: wheat, corn, soybeans, barley, cashmere, cattle.", instrumentName)}
		}
		payload := map[string]string{
			"instrument_id": instrumentID,
			"side":          side,
			"order_type":    "LIMIT",
			"quantity":      qty,
			"price":         price,
		}
		body, status := e.doRequest("POST", "/api/v1/orders", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ %s order placed: %s x%s %s @ %s", side, instrumentID, qty, side, price), userToken),
				Actions: []Action{{Label: "View Orders", Type: "link", URL: "/dashboard/orderbook"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Order failed: %s", body)}
	}

	// --- Orders: modify ---
	// Pattern: "modify order ABC price 330" or "change order ABC price 330"
	if reModify := regexp.MustCompile(`(?i)^(?:modify|change|update)\s+order\s+(\S+)\s+(?:price|to)\s+(\d+(?:\.\d+)?)`).FindStringSubmatch(message); len(reModify) == 3 {
		orderID := reModify[1]
		newPrice := reModify[2]
		payload := map[string]string{"price": newPrice}
		body, status := e.doRequest("PATCH", "/api/v1/orders/"+orderID, payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Order %s updated: price → %s", orderID, newPrice), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to modify order %s: %s", orderID, body)}
	}

	// --- Orders: cancel specific ---
	// Pattern: "cancel order ABC"
	if reCancel := regexp.MustCompile(`(?i)^cancel\s+order\s+(\S+)$`).FindStringSubmatch(message); len(reCancel) == 2 {
		orderID := reCancel[1]
		body, status := e.doRequest("DELETE", "/api/v1/orders/"+orderID, nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Order %s cancelled.", orderID), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to cancel order %s: %s", orderID, body)}
	}

	// --- Orders: list (my orders / show orders) ---
	if containsAny(lower, "show orders", "my orders", "list orders", "open orders") {
		body, status := e.doRequest("GET", "/api/v1/orders", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("📋 Orders:\n%s", prettyJSON(body)),
				Actions: []Action{{Label: "Order Book", Type: "link", URL: "/dashboard/orderbook"}},
			}
		}
		return ChatResponse{Reply: "❌ Unable to fetch orders."}
	}

	// --- Admin: bust trade ---
	// Pattern: "bust trade ABC"
	if reBust := regexp.MustCompile(`(?i)^bust\s+trade\s+(\S+)`).FindStringSubmatch(message); len(reBust) == 2 {
		tradeID := reBust[1]
		body, status := e.doRequest("POST", "/api/v1/admin/trades/"+tradeID+"/bust", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Trade %s busted.", tradeID), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to bust trade %s: %s", tradeID, body)}
	}

	// --- Admin: disable participant ---
	// Pattern: "disable participant ABC" or "disable trader ABC"
	if reDisable := regexp.MustCompile(`(?i)^disable\s+(?:participant|trader)\s+(\S+)`).FindStringSubmatch(message); len(reDisable) == 2 {
		pid := reDisable[1]
		body, status := e.doRequest("POST", "/api/v1/admin/participants/"+pid+"/disable", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Participant %s disabled.", pid), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to disable participant %s: %s", pid, body)}
	}

	// --- Admin: set circuit breaker ---
	// Pattern: "set circuit breaker wheat 15" or "circuit breaker corn 10"
	if reCB := regexp.MustCompile(`(?i)^(?:set\s+)?circuit[\s-]?breaker\s+(\w+)\s+(\d+(?:\.\d+)?)`).FindStringSubmatch(message); len(reCB) == 3 {
		instrumentName := reCB[1]
		limit := reCB[2]
		instrumentID := resolveInstrument(instrumentName)
		if instrumentID == "" {
			return ChatResponse{Reply: fmt.Sprintf("Unknown instrument '%s'.", instrumentName)}
		}
		payload := map[string]string{"limit_pct": limit}
		body, status := e.doRequest("PUT", "/api/v1/admin/instruments/"+instrumentID+"/circuit-breaker", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ Circuit breaker for %s set to %s%%.", instrumentID, limit), userToken),
				Actions: []Action{{Label: "Circuit Breakers", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to set circuit breaker: %s", body)}
	}

	// --- Compliance: suspend trader ---
	// Pattern: "suspend trader ABC" or "suspend trader ABC for insider trading"
	if reSuspend := regexp.MustCompile(`(?i)^suspend\s+(?:trader|participant)\s+(\S+)(?:\s+for\s+(.+))?`).FindStringSubmatch(message); len(reSuspend) >= 2 {
		pid := reSuspend[1]
		reason := "Suspended by admin"
		if len(reSuspend) == 3 && reSuspend[2] != "" {
			reason = strings.TrimSpace(reSuspend[2])
		}
		payload := map[string]string{"reason": reason}
		body, status := e.doRequest("POST", "/api/v1/compliance/participants/"+pid+"/suspend", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ Trader %s suspended. Reason: %s", pid, reason), userToken),
				Actions: []Action{{Label: "Surveillance", Type: "link", URL: "/dashboard/surveillance"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to suspend %s: %s", pid, body)}
	}

	// --- Compliance: reinstate trader ---
	// Pattern: "reinstate trader ABC" or "reinstate participant ABC"
	if reReinstate := regexp.MustCompile(`(?i)^reinstate\s+(?:trader|participant)\s+(\S+)`).FindStringSubmatch(message); len(reReinstate) == 2 {
		pid := reReinstate[1]
		body, status := e.doRequest("POST", "/api/v1/compliance/participants/"+pid+"/reinstate", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Trader %s reinstated.", pid), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to reinstate %s: %s", pid, body)}
	}

	// --- Compliance: file SAR ---
	// Pattern: "file SAR on trader ABC for money laundering" or "file SAR ABC"
	if reSAR := regexp.MustCompile(`(?i)^file\s+sar\s+(?:on\s+)?(?:trader|participant\s+)?(\S+)(?:\s+for\s+(.+))?`).FindStringSubmatch(message); len(reSAR) >= 2 {
		pid := reSAR[1]
		reason := "Suspicious activity"
		if len(reSAR) == 3 && reSAR[2] != "" {
			reason = strings.TrimSpace(reSAR[2])
		}
		payload := map[string]string{
			"participant_id": pid,
			"reason":        reason,
		}
		body, status := e.doRequest("POST", "/api/v1/compliance/sar", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ SAR filed for %s. Reason: %s", pid, reason), userToken),
				Actions: []Action{{Label: "Surveillance", Type: "link", URL: "/dashboard/surveillance"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to file SAR for %s: %s", pid, body)}
	}

	// --- Compliance: resolve alert ---
	// Pattern: "resolve alert 123"
	if reResolveAlert := regexp.MustCompile(`(?i)^resolve\s+alert\s+(\S+)`).FindStringSubmatch(message); len(reResolveAlert) == 2 {
		alertID := reResolveAlert[1]
		body, status := e.doRequest("POST", "/api/v1/compliance/alerts/"+alertID+"/resolve", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Alert %s resolved.", alertID), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to resolve alert %s: %s", alertID, body)}
	}

	// --- Market data: candles ---
	// Pattern: "wheat candles" or "corn candles daily"
	for alias, id := range instrumentAliases {
		if strings.Contains(lower, alias+" candles") || strings.Contains(lower, alias+" candle") {
			body, status := e.doRequest("GET", "/api/v1/market-data/candles/"+id, nil, userToken)
			if status >= 200 && status < 300 {
				return ChatResponse{Reply: fmt.Sprintf("🕯️ %s candles:\n%s", id, prettyJSON(body))}
			}
		}
		// Market data: recent trades
		if strings.Contains(lower, alias+" trades") || strings.Contains(lower, alias+" trade history") {
			body, status := e.doRequest("GET", "/api/v1/market-data/trades/"+id, nil, userToken)
			if status >= 200 && status < 300 {
				return ChatResponse{Reply: fmt.Sprintf("📈 %s recent trades:\n%s", id, prettyJSON(body))}
			}
		}
		// Market data: last trade
		if strings.Contains(lower, "last trade "+alias) || strings.Contains(lower, alias+" last trade") {
			body, status := e.doRequest("GET", "/api/v1/instruments/"+id+"/trades/latest", nil, userToken)
			if status >= 200 && status < 300 {
				return ChatResponse{Reply: fmt.Sprintf("🔄 %s last trade:\n%s", id, prettyJSON(body))}
			}
		}
	}

	// --- Clearing: netting ---
	if containsAny(lower, "show netting", "netting positions", "netting report") {
		body, status := e.doRequest("GET", "/api/v1/clearing/netting", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("⚖️ Netting positions:\n%s", prettyJSON(body)),
				Actions: []Action{{Label: "Clearing", Type: "link", URL: "/dashboard/settlement"}},
			}
		}
		return ChatResponse{Reply: "❌ Unable to fetch netting data."}
	}

	// --- Clearing: position for specific instrument ---
	// Pattern: "position for wheat" or "wheat position"
	for alias, id := range instrumentAliases {
		if strings.Contains(lower, "position for "+alias) || strings.Contains(lower, alias+" position") {
			body, status := e.doRequest("GET", "/api/v1/clearing/positions/"+id, nil, userToken)
			if status >= 200 && status < 300 {
				return ChatResponse{Reply: fmt.Sprintf("📊 Position for %s:\n%s", id, prettyJSON(body))}
			}
		}
	}

	// --- Risk: set max order limit ---
	// Pattern: "set wheat max order 500" or "set max order for corn 1000"
	if reRiskSet := regexp.MustCompile(`(?i)^set\s+(?:(\w+)\s+)?max\s+order(?:\s+(?:for|size|limit))?\s+(?:(\w+)\s+)?(\d+)`).FindStringSubmatch(message); len(reRiskSet) == 4 {
		instrumentName := reRiskSet[1]
		if instrumentName == "" {
			instrumentName = reRiskSet[2]
		}
		limit := reRiskSet[3]
		instrumentID := resolveInstrument(instrumentName)
		if instrumentID == "" {
			return ChatResponse{Reply: fmt.Sprintf("Unknown instrument '%s'.", instrumentName)}
		}
		payload := map[string]string{"max_order_size": limit}
		body, status := e.doRequest("PUT", "/api/v1/admin/risk/order-limits/"+instrumentID, payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ Max order size for %s set to %s.", instrumentID, limit), userToken),
				Actions: []Action{{Label: "Risk Limits", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to set risk limit: %s", body)}
	}

	// --- Risk: show limits ---
	if containsAny(lower, "show risk limits", "risk limits", "order limits") {
		body, status := e.doRequest("GET", "/api/v1/admin/risk/order-limits", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("⚠️ Risk / order limits:\n%s", prettyJSON(body)),
				Actions: []Action{{Label: "Risk Limits", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: "❌ Unable to fetch risk limits."}
	}

	// --- Reports: market summary ---
	// Pattern: "market summary today" or "market summary 2026-03-31"
	if containsAny(lower, "market summary", "daily summary", "trading summary") {
		date := time.Now().Format("2006-01-02")
		if reDateMatch := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`).FindString(message); reDateMatch != "" {
			date = reDateMatch
		}
		body, status := e.doRequest("GET", "/api/v1/reports/market-summary?date="+date, nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("📊 Market Summary (%s):\n%s", date, prettyJSON(body)),
				Actions: []Action{{Label: "Reports", Type: "link", URL: "/dashboard/settlement"}},
			}
		}
		// Reports endpoint may not exist yet; return helpful message
		return ChatResponse{Reply: fmt.Sprintf("📊 Market summary for %s: endpoint not yet available. Check the Settlement page for P&L data.", date)}
	}

	// --- Reports: large trader ---
	if containsAny(lower, "large trader report", "large trader") {
		body, status := e.doRequest("GET", "/api/v1/reports/large-traders", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("📋 Large trader report:\n%s", prettyJSON(body))}
		}
		return ChatResponse{Reply: "📋 Large trader report: endpoint not yet available. Check the Surveillance page for position monitoring."}
	}

	// --- Audit log ---
	if containsAny(lower, "audit log", "audit trail", "show audit") {
		body, status := e.doRequest("GET", "/api/v1/compliance/audit-trail", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("📝 Audit trail:\n%s", prettyJSON(body)),
				Actions: []Action{{Label: "Compliance", Type: "link", URL: "/dashboard/surveillance"}},
			}
		}
		return ChatResponse{Reply: "❌ Unable to fetch audit trail."}
	}

	// --- Halt instrument ---
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
				Reply:   e.withAttribution(fmt.Sprintf("✅ Trading HALTED on %s.", instrumentID), userToken),
				Actions: []Action{{Label: "Circuit Breakers", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to halt %s: %s", instrumentID, body)}
	}

	// --- Resume instrument ---
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
				Reply:   e.withAttribution(fmt.Sprintf("✅ Trading RESUMED on %s.", instrumentID), userToken),
				Actions: []Action{{Label: "Circuit Breakers", Type: "link", URL: "/dashboard/circuit-breakers"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to resume %s: %s", instrumentID, body)}
	}

	// --- Approve KYC ---
	if matchApprove := regexp.MustCompile(`(?i)^approve\s+(?:trader|participant|kyc|application)?\s*(.+)`).FindStringSubmatch(message); len(matchApprove) > 1 {
		pid := strings.TrimSpace(matchApprove[1])
		_, status := e.doRequest("POST", "/api/v1/participants/"+pid+"/approve", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ KYC APPROVED for participant %s.", pid), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to approve %s. Check the participant ID.", pid)}
	}

	// --- Reject KYC ---
	if matchReject := regexp.MustCompile(`(?i)^reject\s+(?:trader|participant|kyc|application)?\s*(\S+)\s*(?:because|reason:?)?\s*(.*)`).FindStringSubmatch(message); len(matchReject) > 1 {
		pid := strings.TrimSpace(matchReject[1])
		reason := strings.TrimSpace(matchReject[2])
		if reason == "" {
			reason = "Rejected by admin"
		}
		_, status := e.doRequest("POST", "/api/v1/participants/"+pid+"/reject", map[string]string{"reason": reason}, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ KYC REJECTED for %s. Reason: %s", pid, reason), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to reject %s.", pid)}
	}

	// --- Mass cancel ---
	if containsAny(lower, "mass cancel", "cancel all") {
		_, status := e.doRequest("POST", "/api/v1/admin/mass-cancel", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution("✅ Mass cancel executed. All open orders cancelled.", userToken)}
		}
		return ChatResponse{Reply: "❌ Mass cancel failed."}
	}

	// --- Create commodity ---
	// Pattern: "create commodity rice grain kg"
	if reCreateCommodity := regexp.MustCompile(`(?i)^create\s+commodity\s+(\S+)\s+(\S+)\s+(\S+)`).FindStringSubmatch(message); len(reCreateCommodity) >= 4 {
		id := strings.ToLower(reCreateCommodity[1])
		category := reCreateCommodity[2]
		unit := reCreateCommodity[3]
		name := strings.ToUpper(id[:1]) + id[1:]
		payload := map[string]string{"id": id, "name": name, "category": category, "unit": unit}
		respBody, status := e.doRequest("POST", "/api/v1/admin/commodities", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Commodity '%s' created (category: %s, unit: %s).", id, category, unit), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to create commodity: %s", respBody)}
	}

	// --- Create instrument ---
	// Pattern: "create instrument RIC-2027M07 rice jul 2027 contract 5000 tick 0.01"
	if reCreateInst := regexp.MustCompile(`(?i)^create\s+instrument\s+(\S+)\s+(\S+)\s+(\S+)\s+(\d{4})\s+contract\s+(\d+(?:\.\d+)?)\s+tick\s+(\d+(?:\.\d+)?)`).FindStringSubmatch(message); len(reCreateInst) == 7 {
		instID := strings.ToUpper(reCreateInst[1])
		commodity := strings.ToLower(reCreateInst[2])
		month := reCreateInst[3]
		year := reCreateInst[4]
		contractSize := reCreateInst[5]
		tickSize := reCreateInst[6]
		payload := map[string]string{
			"id":            instID,
			"commodity_id":  commodity,
			"expiry_month":  month,
			"expiry_year":   year,
			"contract_size": contractSize,
			"tick_size":     tickSize,
		}
		respBody, status := e.doRequest("POST", "/api/v1/admin/instruments", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ Instrument '%s' created (commodity: %s, contract: %s, tick: %s).", instID, commodity, contractSize, tickSize), userToken),
				Actions: []Action{{Label: "Instruments", Type: "link", URL: "/dashboard/orderbook"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to create instrument: %s", respBody)}
	}

	// --- Update instrument ---
	// Pattern: "update instrument WHT-HRW-2026M07-UB status suspended"
	if reUpdateInst := regexp.MustCompile(`(?i)^update\s+instrument\s+(\S+)\s+(\S+)\s+(\S+)`).FindStringSubmatch(message); len(reUpdateInst) == 4 {
		instID := strings.ToUpper(reUpdateInst[1])
		field := strings.ToLower(reUpdateInst[2])
		value := strings.ToLower(reUpdateInst[3])
		payload := map[string]string{field: value}
		respBody, status := e.doRequest("PUT", "/api/v1/admin/instruments/"+instID, payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Instrument '%s' updated: %s → %s.", instID, field, value), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to update instrument %s: %s", instID, respBody)}
	}

	// --- List commodities ---
	// Pattern: "list commodities" / "show commodities"
	if containsAny(lower, "list commodities", "show commodities") {
		respBody, status := e.doRequest("GET", "/api/v1/commodities", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("🌾 Commodities:\n%s", prettyJSON(respBody))}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to list commodities: %s", respBody)}
	}

	// --- Create fee schedule ---
	// Pattern: "create fee schedule Standard 2027"
	if reCreateFee := regexp.MustCompile(`(?i)^create\s+fee\s+schedule\s+(\S+)\s+(\d{4})`).FindStringSubmatch(message); len(reCreateFee) == 3 {
		name := reCreateFee[1]
		year := reCreateFee[2]
		payload := map[string]string{"name": name, "effective_year": year}
		respBody, status := e.doRequest("POST", "/api/v1/admin/fees/schedules", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Fee schedule '%s' (%s) created.", name, year), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to create fee schedule: %s", respBody)}
	}

	// --- Set tier ---
	// Pattern: "set tier farmer for trader1" or "set trader1 tier to farmer"
	if reSetTier1 := regexp.MustCompile(`(?i)^set\s+tier\s+(\S+)\s+for\s+(\S+)`).FindStringSubmatch(message); len(reSetTier1) == 3 {
		tier := strings.ToLower(reSetTier1[1])
		participantID := reSetTier1[2]
		payload := map[string]string{"tier": tier}
		respBody, status := e.doRequest("PUT", "/api/v1/admin/fees/tiers/"+participantID, payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Tier for participant '%s' set to '%s'.", participantID, tier), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to set tier: %s", respBody)}
	}
	if reSetTier2 := regexp.MustCompile(`(?i)^set\s+(\S+)\s+tier\s+to\s+(\S+)`).FindStringSubmatch(message); len(reSetTier2) == 3 {
		participantID := reSetTier2[1]
		tier := strings.ToLower(reSetTier2[2])
		payload := map[string]string{"tier": tier}
		respBody, status := e.doRequest("PUT", "/api/v1/admin/fees/tiers/"+participantID, payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Tier for participant '%s' set to '%s'.", participantID, tier), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to set tier: %s", respBody)}
	}

	// --- Add fee rule ---
	// Pattern: "add fee rule trading farmer 10bps"
	if reAddFeeRule := regexp.MustCompile(`(?i)^add\s+fee\s+rule\s+(\S+)\s+(\S+)\s+(\S+)`).FindStringSubmatch(message); len(reAddFeeRule) == 4 {
		feeType := strings.ToLower(reAddFeeRule[1])
		tier := strings.ToLower(reAddFeeRule[2])
		rate := strings.ToLower(reAddFeeRule[3])
		payload := map[string]string{"fee_type": feeType, "tier": tier, "rate": rate}
		respBody, status := e.doRequest("POST", "/api/v1/admin/fees/rules", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution(fmt.Sprintf("✅ Fee rule added: %s/%s at %s.", feeType, tier, rate), userToken)}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to add fee rule: %s", respBody)}
	}

	// --- Issue receipt ---
	// Pattern: "issue receipt farmer1 wheat 5000"
	if reIssueReceipt := regexp.MustCompile(`(?i)^issue\s+receipt\s+(\S+)\s+(\S+)\s+(\d+(?:\.\d+)?)`).FindStringSubmatch(message); len(reIssueReceipt) == 4 {
		holderID := reIssueReceipt[1]
		commodityID := strings.ToLower(reIssueReceipt[2])
		quantity := reIssueReceipt[3]
		payload := map[string]string{"holder_id": holderID, "commodity_id": commodityID, "quantity": quantity}
		respBody, status := e.doRequest("POST", "/api/v1/warehouse/receipts", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ Warehouse receipt issued: %s kg of %s for %s.", quantity, commodityID, holderID), userToken),
				Actions: []Action{{Label: "Warehouse", Type: "link", URL: "/dashboard/warehouse"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to issue receipt: %s", respBody)}
	}

	// --- Pledge receipt ---
	// Pattern: "pledge receipt RCP-001"
	if rePledgeReceipt := regexp.MustCompile(`(?i)^pledge\s+receipt\s+(\S+)`).FindStringSubmatch(message); len(rePledgeReceipt) == 2 {
		receiptID := strings.ToUpper(rePledgeReceipt[1])
		respBody, status := e.doRequest("POST", "/api/v1/warehouse/receipts/"+receiptID+"/pledge", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   e.withAttribution(fmt.Sprintf("✅ Receipt %s pledged as collateral.", receiptID), userToken),
				Actions: []Action{{Label: "Warehouse", Type: "link", URL: "/dashboard/warehouse"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to pledge receipt %s: %s", receiptID, respBody)}
	}

	// --- Screen participant ---
	// Pattern: "screen participant ABC" or "screen trader ABC"
	if reScreenParticipant := regexp.MustCompile(`(?i)^screen\s+(?:participant|trader)\s+(\S+)`).FindStringSubmatch(message); len(reScreenParticipant) == 2 {
		participantID := reScreenParticipant[1]
		payload := map[string]string{"participant_id": participantID}
		respBody, status := e.doRequest("POST", "/api/v1/screening/check", payload, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("🔍 Screening result for %s:\n%s", participantID, prettyJSON(respBody)),
				Actions: []Action{{Label: "Compliance", Type: "link", URL: "/dashboard/surveillance"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to screen participant %s: %s", participantID, respBody)}
	}

	// --- Batch screen ---
	// Pattern: "batch screen" or "screen all"
	if containsAny(lower, "batch screen", "screen all") {
		respBody, status := e.doRequest("POST", "/api/v1/screening/batch", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{
				Reply:   fmt.Sprintf("🔍 Batch screening initiated:\n%s", prettyJSON(respBody)),
				Actions: []Action{{Label: "Compliance", Type: "link", URL: "/dashboard/surveillance"}},
			}
		}
		return ChatResponse{Reply: fmt.Sprintf("❌ Failed to run batch screening: %s", respBody)}
	}

	// --- System health ---
	if containsAny(lower, "health", "status", "services") {
		body, status := e.doRequest("GET", "/api/v1/admin/health", nil, userToken)
		if status >= 200 && status < 300 {
			return formatHealthResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch system health."}
	}

	// --- Margin calls ---
	if containsAny(lower, "margin") {
		body, status := e.doRequest("GET", "/api/v1/margin/calls/stats", nil, userToken)
		if status >= 200 && status < 300 {
			return formatMarginResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch margin data."}
	}

	// --- Settlement ---
	if lower == "run settlement" || lower == "trigger settlement" {
		_, status := e.doRequest("POST", "/api/v1/settlement/cycle", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: e.withAttribution("✅ Settlement cycle triggered.", userToken)}
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

	// --- Alerts (generic, after resolve-alert handler above) ---
	if containsAny(lower, "alert") {
		body, status := e.doRequest("GET", "/api/v1/compliance/alerts", nil, userToken)
		if status >= 200 && status < 300 {
			return formatAlertsResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch alerts."}
	}

	// --- Participants / KYC ---
	if containsAny(lower, "participant", "kyc", "pending application", "pending kyc") {
		body, status := e.doRequest("GET", "/api/v1/participants", nil, userToken)
		if status >= 200 && status < 300 {
			return formatParticipantsResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch participants."}
	}

	// --- Instruments ---
	if containsAny(lower, "instrument", "commodity", "contract") {
		body, status := e.doRequest("GET", "/api/v1/instruments/list", nil, userToken)
		if status >= 200 && status < 300 {
			return formatInstrumentsResponse(body)
		}
		return ChatResponse{Reply: "❌ Unable to fetch instruments."}
	}

	// --- Order book / price (per instrument) ---
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

	// --- Warehouse ---
	if containsAny(lower, "inventory", "warehouse") {
		body, status := e.doRequest("GET", "/api/v1/warehouse/inventory", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("🏭 Warehouse inventory:\n%s", prettyJSON(body))}
		}
	}

	// --- Tickets ---
	if containsAny(lower, "ticket") {
		body, status := e.doRequest("GET", "/api/v1/tickets", nil, userToken)
		if status >= 200 && status < 300 {
			return formatTicketsResponse(body)
		}
	}

	// --- Fees ---
	if containsAny(lower, "fee") {
		body, status := e.doRequest("GET", "/api/v1/admin/fees", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("💰 Fee schedule:\n%s", prettyJSON(body))}
		}
	}

	// --- Positions (clearing, all) ---
	if containsAny(lower, "position") {
		body, status := e.doRequest("GET", "/api/v1/clearing/positions", nil, userToken)
		if status >= 200 && status < 300 {
			return ChatResponse{Reply: fmt.Sprintf("📊 Clearing positions:\n%s", prettyJSON(body))}
		}
	}

	// --- Help ---
	if containsAny(lower, "help", "what can you do") {
		return ChatResponse{
			Reply: "I can execute these actions for you:\n\n" +
				"📊 **Trading & Instruments**\n" +
				"  `halt wheat` — suspend trading on an instrument\n" +
				"  `resume corn` — re-enable trading\n" +
				"  `set circuit breaker wheat 15` — set price limit %\n" +
				"  `mass cancel` / `cancel all` — cancel all open orders\n" +
				"  `create instrument RIC-2027M07 rice jul 2027 contract 5000 tick 0.01` — create new instrument\n" +
				"  `update instrument WHT-HRW-2026M07-UB status suspended` — update instrument field\n" +
				"  `list instruments` — view all instruments\n\n" +
				"🌾 **Commodities**\n" +
				"  `create commodity rice grain kg` — create new commodity\n" +
				"  `list commodities` / `show commodities` — view all commodities\n\n" +
				"📋 **Orders**\n" +
				"  `buy 10 wheat at 325` — place a limit buy order\n" +
				"  `sell 5 corn at 450` — place a limit sell order\n" +
				"  `show orders` / `my orders` — list your open orders\n" +
				"  `cancel order <id>` — cancel a specific order\n" +
				"  `modify order <id> price 330` — change order price\n\n" +
				"📈 **Market Data**\n" +
				"  `wheat price` / `corn ticker` — live ticker\n" +
				"  `wheat order book` — L2 order book\n" +
				"  `wheat candles` — OHLCV candle data\n" +
				"  `corn trades` — recent trade history\n" +
				"  `last trade wheat` — latest executed trade\n\n" +
				"👥 **Participants & KYC**\n" +
				"  `show participants` / `show pending KYC` — list applications\n" +
				"  `approve trader <id>` — approve KYC\n" +
				"  `reject trader <id> reason: <text>` — reject KYC\n" +
				"  `suspend trader <id> for <reason>` — suspend access\n" +
				"  `reinstate trader <id>` — restore access\n" +
				"  `disable participant <id>` — disable a participant\n" +
				"  `screen participant <id>` / `screen trader <id>` — run screening check\n" +
				"  `batch screen` / `screen all` — screen all participants\n\n" +
				"💰 **Clearing & Margin**\n" +
				"  `show netting` — netting positions\n" +
				"  `position for wheat` — per-instrument position\n" +
				"  `show margin` / `margin calls` — margin call stats\n\n" +
				"⚖️ **Settlement**\n" +
				"  `run settlement` — trigger settlement cycle\n" +
				"  `show settlement` — cycle history\n\n" +
				"🔍 **Compliance**\n" +
				"  `show alerts` — view compliance alerts\n" +
				"  `resolve alert <id>` — resolve an alert\n" +
				"  `file SAR on trader <id> for <reason>` — file SAR\n" +
				"  `show audit log` — view audit trail\n\n" +
				"🏭 **Warehouse**\n" +
				"  `show inventory` — warehouse inventory\n" +
				"  `show receipts` — list warehouse receipts\n" +
				"  `issue receipt <holder_id> <commodity> <quantity>` — issue warehouse receipt\n" +
				"  `pledge receipt <receipt_id>` — pledge receipt as collateral\n\n" +
				"💵 **Fees**\n" +
				"  `show fees` — fee schedule\n" +
				"  `create fee schedule <name> <year>` — create new fee schedule\n" +
				"  `set tier <tier> for <participant_id>` — assign fee tier\n" +
				"  `set <participant_id> tier to <tier>` — assign fee tier (alt form)\n" +
				"  `add fee rule <type> <tier> <rate>` — add fee rule (e.g. trading farmer 10bps)\n\n" +
				"⚠️ **Risk**\n" +
				"  `set wheat max order 500` — order size limit\n" +
				"  `show risk limits` — all order limits\n\n" +
				"📈 **Reports**\n" +
				"  `market summary today` — daily market summary\n" +
				"  `large trader report` — large position holders\n\n" +
				"🎫 **Tickets**\n" +
				"  `show tickets` — list support tickets\n\n" +
				"🏥 **System**\n" +
				"  `system health` — service status\n" +
				"  `who am I` — your profile\n" +
				"  `help` — show this message\n",
		}
	}

	// Default
	return ChatResponse{
		Reply: "I can help with system health, alerts, margin status, and tickets. What would you like to know?",
	}
}

// fetchUserEmail fetches the email of the currently authenticated user from /auth/me.
// Returns an empty string if the request fails or the token is empty.
func (e *ActionExecutor) fetchUserEmail(token string) string {
	if token == "" {
		return ""
	}
	body, status := e.doRequest("GET", "/api/v1/auth/me", nil, token)
	if status < 200 || status >= 300 {
		return ""
	}
	var profile struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(body), &profile); err != nil {
		return ""
	}
	if profile.Data.Email != "" {
		return profile.Data.Email
	}
	return profile.Email
}

// withAttribution appends an "Executed by" line to a reply using the user's email.
// If the email cannot be fetched, the reply is returned unchanged.
func (e *ActionExecutor) withAttribution(reply, token string) string {
	email := e.fetchUserEmail(token)
	if email == "" {
		return reply
	}
	return reply + "\n\nExecuted by: " + email
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
