package handler

import (
	"github.com/garudax-platform/gateway/internal/router"
)

// RegisterRoutes registers all API routes on the router.
func (h *Handler) RegisterRoutes(rt *router.Router) {
	// --- Health ---
	rt.Handle("GET", "/healthz", h.healthz)
	rt.Handle("GET", "/readyz", h.readyz)

	// --- Orders (matching-engine) ---
	rt.Handle("POST", "/api/v1/orders", h.SubmitOrder)
	rt.Handle("GET", "/api/v1/orders/{order_id}", h.GetOrder)
	rt.Handle("GET", "/api/v1/orders", h.ListOrders)
	rt.Handle("DELETE", "/api/v1/orders/{order_id}", h.CancelOrder)
	rt.Handle("DELETE", "/api/v1/orders", h.CancelAllOrders)
	rt.Handle("PATCH", "/api/v1/orders/{order_id}", h.ModifyOrder)

	// --- Market Data (matching-engine, public) ---
	rt.Handle("GET", "/api/v1/instruments/list", h.ListInstruments)
	rt.Handle("GET", "/api/v1/instruments/{instrument_id}/book", h.GetOrderBook)
	rt.Handle("GET", "/api/v1/instruments/{instrument_id}/book/l3", h.GetOrderBookL3)
	rt.Handle("GET", "/api/v1/instruments/{instrument_id}/trades/latest", h.GetLastTrade)

	// --- Admin Health (aggregated) ---
	rt.Handle("GET", "/api/v1/admin/health", h.AdminHealth)

	// --- Admin (matching-engine) ---
	rt.Handle("POST", "/api/v1/admin/instruments/{instrument_id}/halt", h.HaltInstrument)
	rt.Handle("POST", "/api/v1/admin/instruments/{instrument_id}/resume", h.ResumeInstrument)
	rt.Handle("POST", "/api/v1/admin/trades/{trade_id}/bust", h.BustTrade)
	rt.Handle("PUT", "/api/v1/admin/instruments/{instrument_id}/circuit-breaker", h.SetCircuitBreaker)
	rt.Handle("GET", "/api/v1/admin/circuit-breakers", h.GetCircuitBreakers)
	rt.Handle("POST", "/api/v1/admin/participants/{participant_id}/disable", h.DisableParticipant)
	rt.Handle("POST", "/api/v1/admin/mass-cancel", h.MassCancel)

	// --- Clearing (clearing-engine) ---
	rt.Handle("GET", "/api/v1/clearing/positions", h.GetPositions)
	rt.Handle("GET", "/api/v1/clearing/positions/{instrument_id}", h.GetPosition)
	rt.Handle("GET", "/api/v1/clearing/netting", h.GetNetting)

	// --- Margin (margin-engine) ---
	rt.Handle("GET", "/api/v1/margin", h.GetPortfolioMargin)
	rt.Handle("POST", "/api/v1/margin/calculate", h.CalculateMargin)
	rt.Handle("GET", "/api/v1/margin/calls", h.GetMarginCalls)
	rt.Handle("GET", "/api/v1/margin/calls/stats", h.GetMarginCallStats)

	// --- Settlement (settlement-engine) ---
	rt.Handle("GET", "/api/v1/settlement/cycles", h.GetSettlementCycles)
	rt.Handle("GET", "/api/v1/settlement/cycles/{cycle_id}", h.GetSettlementCycle)

	// --- Auth (auth-service) ---
	rt.Handle("POST", "/api/v1/auth/login", h.Login)
	rt.Handle("POST", "/api/v1/auth/refresh", h.RefreshToken)
	rt.Handle("POST", "/api/v1/auth/logout", h.Logout)
	rt.Handle("POST", "/api/v1/auth/register", h.Register)
	rt.Handle("GET", "/api/v1/auth/me", h.GetProfile)
	rt.Handle("POST", "/api/v1/auth/password/change", h.ChangePassword)
	rt.Handle("POST", "/api/v1/auth/password/reset", h.ResetPassword)

	// --- Compliance Onboarding (compliance-service) ---
	rt.Handle("POST", "/api/v1/participants", h.SubmitApplication)
	rt.Handle("GET", "/api/v1/participants/{participant_id}", h.GetApplication)
	rt.Handle("GET", "/api/v1/participants", h.ListApplications)
	rt.Handle("POST", "/api/v1/participants/{participant_id}/documents", h.UploadDocument)
	rt.Handle("GET", "/api/v1/participants/{participant_id}/documents", h.ListDocuments)
	rt.Handle("POST", "/api/v1/participants/{participant_id}/approve", h.ApproveApplication)
	rt.Handle("POST", "/api/v1/participants/{participant_id}/reject", h.RejectApplication)

	// --- Compliance Screening (compliance-service) ---
	rt.Handle("POST", "/api/v1/screening/check", h.ScreenParticipant)
	rt.Handle("GET", "/api/v1/screening/{screening_id}", h.GetScreeningResult)
	rt.Handle("POST", "/api/v1/screening/batch", h.BatchScreen)
	rt.Handle("POST", "/api/v1/screening/{screening_id}/resolve", h.ResolveMatch)
	rt.Handle("GET", "/api/v1/risk-scores/{participant_id}", h.GetRiskScore)

	// --- Compliance Admin (compliance-service) ---
	rt.Handle("GET", "/api/v1/compliance/alerts", h.ListAlerts)
	rt.Handle("POST", "/api/v1/compliance/alerts/{alert_id}/resolve", h.ResolveAlert)
	rt.Handle("GET", "/api/v1/compliance/audit-trail", h.GetAuditTrail)
	rt.Handle("POST", "/api/v1/compliance/sar", h.FileSAR)
	rt.Handle("POST", "/api/v1/compliance/participants/{participant_id}/suspend", h.SuspendParticipant)
	rt.Handle("POST", "/api/v1/compliance/participants/{participant_id}/reinstate", h.ReinstateParticipant)

	// --- Market Data (market-data-service, public) ---
	rt.Handle("GET", "/api/v1/market-data/candles/{instrument_id}", h.GetCandles)
	rt.Handle("GET", "/api/v1/market-data/ticker/{instrument_id}", h.GetTicker)
	rt.Handle("GET", "/api/v1/market-data/trades/{instrument_id}", h.GetMarketTrades)

	// --- Warehouse (warehouse-service) ---
	rt.Handle("POST", "/api/v1/warehouse/receipts", h.IssueReceipt)
	rt.Handle("POST", "/api/v1/warehouse/receipts/{receipt_id}/pledge", h.PledgeReceipt)
	rt.Handle("POST", "/api/v1/warehouse/deliveries", h.CreateDelivery)
	rt.Handle("GET", "/api/v1/warehouse/inventory", h.GetInventory)

	// --- Demo Reset (auth-service) ---
	rt.Handle("POST", "/api/v1/admin/demo/reset", h.DemoReset)

	// --- Admin Risk (direct DB) ---
	rt.Handle("GET", "/api/v1/admin/risk/order-limits", h.ListOrderLimits)
	rt.Handle("PUT", "/api/v1/admin/risk/order-limits/{instrument_id}", h.UpdateOrderLimits)

	// --- Securities (securities-service) ---
	rt.Handle("GET", "/api/v1/securities/instruments", h.ListSecuritiesInstruments)
	rt.Handle("POST", "/api/v1/securities/instruments", h.CreateSecuritiesInstrument)
	rt.Handle("GET", "/api/v1/securities/instruments/{id}", h.GetSecuritiesInstrument)
	rt.Handle("PATCH", "/api/v1/securities/instruments/{id}", h.UpdateSecuritiesInstrument)
	rt.Handle("PUT", "/api/v1/securities/instruments/{id}", h.UpdateSecuritiesInstrument)
	rt.Handle("GET", "/api/v1/securities/orders", h.ListSecuritiesOrders)
	rt.Handle("POST", "/api/v1/securities/orders", h.SubmitSecuritiesOrder)
	rt.Handle("GET", "/api/v1/securities/orders/{id}", h.GetSecuritiesOrder)
	rt.Handle("DELETE", "/api/v1/securities/orders/{id}", h.CancelSecuritiesOrder)

	// --- Reference Data (direct DB, public) ---
	// These routes are registered separately via RefDataHandlers.RegisterRoutes()
}
