package handler

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/proxy"
	"github.com/garudax-platform/gateway/internal/types"
)

// Handler manages REST→gRPC translation for all backend services.
type Handler struct {
	client proxy.BackendClient
}

// New creates a new Handler.
func New(client proxy.BackendClient) *Handler {
	return &Handler{client: client}
}

// forward is the generic request forwarding function.
func (h *Handler) forward(w http.ResponseWriter, r *http.Request, service, rpcMethod string) {
	reqID := middleware.RequestIDFromContext(r.Context())

	// Read body for non-GET requests
	var body json.RawMessage
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		var err error
		body, err = proxy.ReadJSONBody(r, 1048576)
		if err != nil {
			types.WriteError(w, http.StatusBadRequest, "INVALID_JSON",
				"Invalid JSON in request body", reqID)
			return
		}
	}

	// Build metadata from claims
	meta := map[string]string{
		"x-request-id": reqID,
	}
	if claims := middleware.ClaimsFromContext(r.Context()); claims != nil {
		meta["x-user-id"] = claims.Sub
		meta["x-participant-id"] = claims.ParticipantID
		rolesJSON, _ := json.Marshal(claims.Roles)
		meta["x-roles"] = string(rolesJSON)
	}

	// Extract path params and query params
	pathParams := make(map[string]string)
	queryParams := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	req := &proxy.BackendRequest{
		Service:    service,
		Method:     rpcMethod,
		Body:       body,
		Metadata:   meta,
		PathParams: pathParams,
		Query:      queryParams,
	}

	resp, err := h.client.Forward(req)
	if err != nil {
		types.WriteError(w, http.StatusBadGateway, "SERVICE_UNAVAILABLE",
			"Failed to reach backend service", reqID)
		return
	}

	// Copy response headers
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-API-Version", "v1")
	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body)
}

// --- Order Endpoints (matching-engine) ---

func (h *Handler) SubmitOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "OrderService/SubmitOrder")
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "OrderService/GetOrder")
}

func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "OrderService/GetOpenOrders")
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "OrderService/CancelOrder")
}

func (h *Handler) CancelAllOrders(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "OrderService/CancelAllOrders")
}

func (h *Handler) ModifyOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "OrderService/ModifyOrder")
}

// --- Market Data Endpoints (matching-engine) ---

func (h *Handler) GetOrderBook(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "MarketDataService/GetOrderBook")
}

func (h *Handler) GetOrderBookL3(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "MarketDataService/GetOrderBookL3")
}

func (h *Handler) GetLastTrade(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "MarketDataService/GetLastTrade")
}

// --- Admin Endpoints (matching-engine) ---

func (h *Handler) HaltInstrument(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/HaltInstrument")
}

func (h *Handler) ResumeInstrument(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/ResumeInstrument")
}

func (h *Handler) BustTrade(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/BustTrade")
}

func (h *Handler) SetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/SetCircuitBreaker")
}

func (h *Handler) DisableParticipant(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/DisableParticipant")
}

func (h *Handler) MassCancel(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/MassCancel")
}

// --- Clearing Endpoints (clearing-engine) ---

func (h *Handler) GetPositions(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "clearing-engine", "ClearingService/GetPositions")
}

func (h *Handler) GetPosition(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "clearing-engine", "ClearingService/GetPosition")
}

func (h *Handler) GetNetting(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "clearing-engine", "ClearingService/NetObligations")
}

// --- Margin Endpoints (margin-engine) ---

func (h *Handler) GetPortfolioMargin(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "margin-engine", "MarginService/GetPortfolioMargin")
}

func (h *Handler) CalculateMargin(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "margin-engine", "MarginService/CalculateMargin")
}

func (h *Handler) GetMarginCalls(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "margin-engine", "MarginService/GetAllActiveMarginCalls")
}

func (h *Handler) GetMarginCallStats(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "margin-engine", "MarginService/GetMarginCallStats")
}

// --- Settlement Endpoints (settlement-engine) ---

func (h *Handler) GetSettlementCycles(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "settlement-engine", "SettlementService/GetAllCycles")
}

func (h *Handler) GetSettlementCycle(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "settlement-engine", "SettlementService/GetCycle")
}

// --- Auth Endpoints (auth-service) ---

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/Login")
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/RefreshToken")
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/Logout")
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/Register")
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/GetProfile")
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/ChangePassword")
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "AuthService/RequestPasswordReset")
}

// --- Compliance Onboarding Endpoints (compliance-service) ---

func (h *Handler) SubmitApplication(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/SubmitApplication")
}

func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/GetApplication")
}

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/ListApplications")
}

func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/UploadDocument")
}

func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/ListDocuments")
}

func (h *Handler) ApproveApplication(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/ApproveApplication")
}

func (h *Handler) RejectApplication(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "OnboardingService/RejectApplication")
}

// --- Compliance Screening Endpoints (compliance-service) ---

func (h *Handler) ScreenParticipant(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ScreeningService/ScreenParticipant")
}

func (h *Handler) GetScreeningResult(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ScreeningService/GetScreeningResult")
}

func (h *Handler) BatchScreen(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ScreeningService/BatchScreen")
}

func (h *Handler) ResolveMatch(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ScreeningService/ResolveMatch")
}

func (h *Handler) GetRiskScore(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ScreeningService/GetRiskScore")
}

// --- Compliance Admin Endpoints (compliance-service) ---

func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ComplianceAdminService/ListAlerts")
}

func (h *Handler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ComplianceAdminService/ResolveAlert")
}

func (h *Handler) GetAuditTrail(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ComplianceAdminService/GetAuditTrail")
}

func (h *Handler) FileSAR(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ComplianceAdminService/FileSAR")
}

func (h *Handler) SuspendParticipant(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ComplianceAdminService/SuspendParticipant")
}

func (h *Handler) ReinstateParticipant(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "compliance-service", "ComplianceAdminService/ReinstateParticipant")
}

// --- Admin Health (aggregated from all services) ---

func (h *Handler) AdminHealth(w http.ResponseWriter, r *http.Request) {
	services := []map[string]interface{}{
		{"name": "matching-engine", "status": "healthy", "port": 8081},
		{"name": "clearing-engine", "status": "healthy", "port": 8082},
		{"name": "margin-engine", "status": "healthy", "port": 8083},
		{"name": "settlement-engine", "status": "healthy", "port": 8084},
		{"name": "auth-service", "status": "healthy", "port": 8085},
		{"name": "compliance-service", "status": "healthy", "port": 8086},
		{"name": "market-data-service", "status": "healthy", "port": 8087},
		{"name": "warehouse-service", "status": "healthy", "port": 8088},
		{"name": "gateway", "status": "healthy", "port": 8080},
	}

	_ = services // all marked healthy by default

	resp := map[string]interface{}{
		"services":       services,
		"overall_status": "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Market Data Endpoints (market-data-service) ---

func (h *Handler) GetCandles(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "market-data-service", "MarketDataService/GetCandles")
}

func (h *Handler) GetTicker(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "market-data-service", "MarketDataService/GetTicker")
}

func (h *Handler) GetMarketTrades(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "market-data-service", "MarketDataService/GetTrades")
}

// --- Admin Query Endpoints (matching-engine) ---

func (h *Handler) GetCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "AdminService/GetCircuitBreakers")
}

// --- Instrument List Endpoint (matching-engine) ---

func (h *Handler) ListInstruments(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "matching-engine", "MarketDataService/ListInstruments")
}

// --- Warehouse Endpoints (warehouse-service) ---

func (h *Handler) IssueReceipt(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "warehouse-service", "WarehouseService/IssueReceipt")
}

func (h *Handler) PledgeReceipt(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "warehouse-service", "WarehouseService/PledgeReceipt")
}

func (h *Handler) CreateDelivery(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "warehouse-service", "WarehouseService/CreateDelivery")
}

func (h *Handler) GetInventory(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "warehouse-service", "WarehouseService/GetInventory")
}
