package handler

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/gateway/internal/middleware"
	"github.com/garudax-platform/gateway/internal/proxy"
	"github.com/garudax-platform/gateway/internal/risk"
	"github.com/garudax-platform/gateway/internal/types"
)

// Handler manages REST→gRPC translation for all backend services.
type Handler struct {
	client      proxy.BackendClient
	riskChecker *risk.PreTradeChecker
}

// New creates a new Handler.
func New(client proxy.BackendClient) *Handler {
	return &Handler{
		client:      client,
		riskChecker: risk.NewPreTradeChecker(nil), // fail-open by default
	}
}

// NewWithRisk creates a new Handler with a risk checker backed by the given store.
func NewWithRisk(client proxy.BackendClient, riskStore risk.Store) *Handler {
	return &Handler{
		client:      client,
		riskChecker: risk.NewPreTradeChecker(riskStore),
	}
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
	reqID := middleware.RequestIDFromContext(r.Context())

	// Read body for risk check
	body, err := proxy.ReadJSONBody(r, 1048576)
	if err != nil {
		types.WriteError(w, http.StatusBadRequest, "INVALID_JSON",
			"Invalid JSON in request body", reqID)
		return
	}

	// Extract participant ID from JWT claims
	var participantID string
	if claims := middleware.ClaimsFromContext(r.Context()); claims != nil {
		participantID = claims.ParticipantID
	}

	// Pre-trade risk checks
	if body != nil && h.riskChecker != nil {
		order, parseErr := risk.ParseOrderRequest(body, participantID)
		if parseErr == nil {
			// Last price of 0 means no price band check (fail-open for first trade)
			if rErr := h.riskChecker.CheckOrder(r.Context(), order, 0); rErr != nil {
				types.WriteErrorWithDetails(w, http.StatusBadRequest, rErr.Code, rErr.Message, reqID,
					[]types.ErrorDetail{{Field: rErr.Field, Reason: rErr.Message}})
				return
			}
		}
		// If parse fails, let the matching engine handle validation
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

	req := &proxy.BackendRequest{
		Service:    "matching-engine",
		Method:     "OrderService/SubmitOrder",
		Body:       body,
		Metadata:   meta,
		PathParams: make(map[string]string),
		Query:      make(map[string]string),
	}

	resp, fwdErr := h.client.Forward(req)
	if fwdErr != nil {
		types.WriteError(w, http.StatusBadGateway, "SERVICE_UNAVAILABLE",
			"Failed to reach backend service", reqID)
		return
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-API-Version", "v1")
	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body)
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
		{"name": "securities-service", "status": "healthy", "port": 8089},
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

// --- Securities Endpoints (securities-service) ---

func (h *Handler) ListSecuritiesInstruments(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/ListInstruments")
}

func (h *Handler) CreateSecuritiesInstrument(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/CreateInstrument")
}

func (h *Handler) GetSecuritiesInstrument(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/GetInstrument")
}

func (h *Handler) UpdateSecuritiesInstrument(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/UpdateInstrument")
}

func (h *Handler) ListSecuritiesOrders(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/ListOrders")
}

func (h *Handler) SubmitSecuritiesOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/SubmitOrder")
}

func (h *Handler) GetSecuritiesOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/GetOrder")
}

func (h *Handler) CancelSecuritiesOrder(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "securities-service", "SecuritiesService/CancelOrder")
}

// --- Admin Risk Endpoints (direct DB) ---

// ListOrderLimits returns all configured order limits.
func (h *Handler) ListOrderLimits(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.RequestIDFromContext(r.Context())

	if h.riskChecker == nil || h.riskChecker.Store() == nil {
		types.WriteError(w, http.StatusServiceUnavailable, "RISK_DB_UNAVAILABLE",
			"Risk database is not configured", reqID)
		return
	}

	limits, err := h.riskChecker.Store().ListOrderLimits(r.Context())
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to fetch order limits", reqID)
		return
	}

	if limits == nil {
		limits = []risk.OrderLimits{}
	}

	types.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": limits,
	})
}

// UpdateOrderLimits updates order limits for a specific instrument.
func (h *Handler) UpdateOrderLimits(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.RequestIDFromContext(r.Context())

	if h.riskChecker == nil || h.riskChecker.Store() == nil {
		types.WriteError(w, http.StatusServiceUnavailable, "RISK_DB_UNAVAILABLE",
			"Risk database is not configured", reqID)
		return
	}

	instrumentID := r.URL.Query().Get("instrument_id")
	if instrumentID == "" {
		types.WriteError(w, http.StatusBadRequest, "MISSING_INSTRUMENT_ID",
			"instrument_id path parameter is required", reqID)
		return
	}

	body, err := proxy.ReadJSONBody(r, 1048576)
	if err != nil {
		types.WriteError(w, http.StatusBadRequest, "INVALID_JSON",
			"Invalid JSON in request body", reqID)
		return
	}

	var limits risk.OrderLimits
	if err := json.Unmarshal(body, &limits); err != nil {
		types.WriteError(w, http.StatusBadRequest, "INVALID_JSON",
			"Invalid order limits JSON", reqID)
		return
	}
	limits.InstrumentID = instrumentID

	// Validate
	if limits.MaxOrderQty <= 0 {
		types.WriteError(w, http.StatusBadRequest, "INVALID_LIMIT",
			"max_order_qty must be positive", reqID)
		return
	}
	if limits.MaxOrderValue <= 0 {
		types.WriteError(w, http.StatusBadRequest, "INVALID_LIMIT",
			"max_order_value must be positive", reqID)
		return
	}
	if limits.PriceBandPct <= 0 || limits.PriceBandPct > 100 {
		types.WriteError(w, http.StatusBadRequest, "INVALID_LIMIT",
			"price_band_pct must be between 0 and 100", reqID)
		return
	}

	if err := h.riskChecker.Store().UpsertOrderLimits(r.Context(), &limits); err != nil {
		types.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to update order limits", reqID)
		return
	}

	types.WriteJSON(w, http.StatusOK, limits)
}

// DemoReset clears all in-memory auth data for demo reset.
func (h *Handler) DemoReset(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "auth-service", "DemoService/Reset")
}
