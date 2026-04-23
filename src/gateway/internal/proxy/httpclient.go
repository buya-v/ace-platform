package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// rpcToHTTP maps "Service/RPCMethod" to "HTTP_METHOD /path" on the backend service.
var rpcToHTTP = map[string]string{
	// Auth service (port 8085)
	"AuthService/Register":           "POST /api/v1/register",
	"AuthService/Login":              "POST /api/v1/login",
	"AuthService/RefreshToken":       "POST /api/v1/refresh",
	"AuthService/Authorize":          "POST /api/v1/authorize",
	"AuthService/Exchange":           "POST /api/v1/exchange",
	"AuthService/ValidateToken":      "POST /api/v1/token/validate",
	"AuthService/RevokeSession":      "POST /api/v1/session/revoke",
	"AuthService/CreateAPIKey":       "POST /api/v1/apikey/create",
	"AuthService/ValidateAPIKey":     "POST /api/v1/apikey/validate",
	"AuthService/RevokeAPIKey":       "POST /api/v1/apikey/revoke",
	"AuthService/ListUsers":          "GET /api/v1/users",
	"AuthService/Logout":             "POST /api/v1/session/revoke",
	"AuthService/GetProfile":         "POST /api/v1/token/validate",
	"AuthService/ChangePassword":     "POST /api/v1/register",
	"AuthService/RequestPasswordReset": "POST /api/v1/register",

	// Matching engine / Order service (port 8081)
	"OrderService/SubmitOrder":    "POST /orders",
	"OrderService/CancelOrder":    "DELETE /orders",
	"OrderService/GetOrder":       "GET /orders",
	"OrderService/GetOpenOrders":  "GET /orders",
	"OrderService/CancelAllOrders":"DELETE /orders",
	"OrderService/ModifyOrder":    "POST /orders",
	"AdminService/GetCircuitBreakers":  "GET /circuit-breakers",
	"MarketDataService/ListInstruments":"GET /instruments",
	"MarketDataService/GetOrderBook":  "GET /book/{instrument_id}",
	"MarketDataService/GetOrderBookL3":"GET /book/{instrument_id}",
	"MarketDataService/GetLastTrade":  "GET /trades/latest/{instrument_id}",

	// Clearing engine (port 8082)
	"ClearingService/GetPositions":   "GET /positions",
	"ClearingService/GetPosition":    "GET /positions",
	"ClearingService/NetObligations": "GET /netting",

	// Margin engine (port 8083)
	"MarginService/GetPortfolioMargin":       "GET /margin",
	"MarginService/CalculateMargin":          "GET /margin",
	"MarginService/GetAllActiveMarginCalls":  "GET /margin-calls",
	"MarginService/GetMarginCallStats":       "GET /margin-call-stats",

	// Settlement engine (port 8084)
	"SettlementService/GetAllCycles": "GET /cycles",
	"SettlementService/GetCycle":     "GET /cycles",

	// Compliance service (port 8086)
	"OnboardingService/SubmitApplication":  "POST /application",
	"OnboardingService/GetApplication":     "GET /participant-status?participant_id={participant_id}",
	"OnboardingService/ListApplications":   "GET /applications",
	"OnboardingService/UploadDocument":     "POST /application",
	"OnboardingService/ListDocuments":      "GET /application",
	"OnboardingService/ApproveApplication": "POST /application",
	"OnboardingService/RejectApplication":  "POST /application",
	"ComplianceService/GetStatus":          "GET /participant-status",
	"ScreeningService/ScreenParticipant":   "POST /participant-status",
	"ScreeningService/GetScreeningResult":  "GET /participant-status",
	"ScreeningService/BatchScreen":         "POST /participant-status",
	"ScreeningService/ResolveMatch":        "POST /participant-status",
	"ScreeningService/GetRiskScore":        "GET /risk-score",
	"ComplianceAdminService/ListAlerts":           "GET /alerts",
	"ComplianceAdminService/ResolveAlert":         "POST /alerts",
	"ComplianceAdminService/GetAuditTrail":        "GET /audit-trail",
	"ComplianceAdminService/FileSAR":              "POST /alerts",
	"ComplianceAdminService/SuspendParticipant":   "POST /participant-status",
	"ComplianceAdminService/ReinstateParticipant": "POST /participant-status",

	// Market data service (port 8087)
	"MarketDataService/GetCandles": "GET /candles",
	"MarketDataService/GetTicker":  "GET /ticker",
	"MarketDataService/GetTrades":  "GET /trades",

	// Warehouse service (port 8088)
	"WarehouseService/IssueReceipt":   "POST /receipts",
	"WarehouseService/PledgeReceipt":  "POST /receipts/pledge",
	"WarehouseService/CreateDelivery": "POST /deliveries",
	"WarehouseService/GetInventory":   "GET /inventory",
}

// HTTPBackendClient forwards requests to backend services over HTTP.
type HTTPBackendClient struct {
	backends map[string]string // service name -> HTTP base URL
	client   *http.Client
}

// NewHTTPBackendClient creates a client that forwards to real backend HTTP APIs.
func NewHTTPBackendClient(backends map[string]string) *HTTPBackendClient {
	return &HTTPBackendClient{
		backends: backends,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Forward sends the request to the backend service's HTTP API.
func (c *HTTPBackendClient) Forward(req *BackendRequest) (*BackendResponse, error) {
	baseURL, ok := c.backends[req.Service]
	if !ok {
		return errorResponse(req.Service, "unknown service"), nil
	}

	// Look up the RPC method in our mapping
	target, ok := rpcToHTTP[req.Method]
	if !ok {
		return errorResponse(req.Service, fmt.Sprintf("no route for %s", req.Method)), nil
	}

	parts := strings.SplitN(target, " ", 2)
	if len(parts) != 2 {
		return errorResponse(req.Service, "invalid route mapping"), nil
	}
	httpMethod := parts[0]
	path := parts[1]

	// Track which query keys were consumed as path params
	consumed := make(map[string]bool)

	// Substitute path params (check both PathParams and Query since router
	// puts path params into query string)
	for k, v := range req.PathParams {
		if strings.Contains(path, "{"+k+"}") {
			path = strings.ReplaceAll(path, "{"+k+"}", v)
			consumed[k] = true
		}
	}
	for k, v := range req.Query {
		if strings.Contains(path, "{"+k+"}") {
			path = strings.ReplaceAll(path, "{"+k+"}", v)
			consumed[k] = true
		}
	}

	// Build query string from remaining (unconsumed) params
	targetURL := baseURL + path
	qparts := []string{}
	for k, v := range req.Query {
		if !consumed[k] {
			qparts = append(qparts, k+"="+v)
		}
	}
	if len(qparts) > 0 {
		if strings.Contains(targetURL, "?") {
			targetURL += "&" + strings.Join(qparts, "&")
		} else {
			targetURL += "?" + strings.Join(qparts, "&")
		}
	}

	// Build HTTP request
	var bodyReader io.Reader
	if req.Body != nil && httpMethod != "GET" {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(httpMethod, targetURL, bodyReader)
	if err != nil {
		return errorResponse(req.Service, err.Error()), nil
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Forward metadata as headers
	for k, v := range req.Metadata {
		httpReq.Header.Set(k, v)
	}

	// Execute
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return errorResponse(req.Service, fmt.Sprintf("connection failed: %s", err.Error())), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return errorResponse(req.Service, err.Error()), nil
	}

	return &BackendResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
	}, nil
}

func errorResponse(service, msg string) *BackendResponse {
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    "SERVICE_UNAVAILABLE",
			"message": "Backend service " + service + ": " + msg,
		},
	}
	body, _ := json.Marshal(resp)
	return &BackendResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       body,
	}
}
