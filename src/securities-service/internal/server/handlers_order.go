// Package server — order CRUD HTTP handlers.
package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/kafka"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// validOrderSides is the set of accepted OrderSide enum values.
var validOrderSides = map[types.OrderSide]bool{
	types.OrderSideBuy:       true,
	types.OrderSideSell:      true,
	types.OrderSideShortSell: true,
}

// validOrderTypes is the set of accepted OrderType enum values.
var validOrderTypes = map[types.OrderType]bool{
	types.OrderTypeLimit:     true,
	types.OrderTypeMarket:    true,
	types.OrderTypeStop:      true,
	types.OrderTypeStopLimit: true,
}

// handleSubmitOrder handles POST /api/v1/securities/orders.
//
// Validation:
//  a) instrument_id is required and instrument must exist
//  b) instrument trading_status must be ACTIVE
//  c) side must be BUY, SELL, or SHORT_SELL
//  d) order_type must be LIMIT, MARKET, STOP, or STOP_LIMIT
//  e) quantity > 0
//  f) quantity must be a whole-lot multiple of instrument.LotSize
//  g) For LIMIT/STOP_LIMIT: price > 0 and price must be a tick-size multiple
//  h) For STOP/STOP_LIMIT: stop_price > 0
//  i) SHORT_SELL: currently not enabled
//
// Defaults applied:
//   - ID generated via newUUID()
//   - Status set to PENDING
//   - FilledQuantity set to 0
//   - AvgFillPrice set to 0
//   - CreatedAt and UpdatedAt set to current UTC time
//   - TimeInForce defaults to GTC if empty
func (s *Server) handleSubmitOrder(w http.ResponseWriter, r *http.Request) {
	if err := s.checkPermission(r, types.PermOrderCreate); err != nil {
		s.writeError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
		return
	}
	// Extract tenant from context (set by TenantMiddleware).
	tenantID, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "TENANT_REQUIRED", "X-GarudaX-Tenant header is required", nil)
		return
	}

	var order types.SecurityOrder
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}

	// Order throttle — check per-firm rate limit before any validation.
	// Resolve the participant's firm, look up a per-firm ThrottleConfig, and
	// apply the configured limit.  Falls back to the default 100 orders/sec when
	// no config exists or the config is disabled.
	if s.throttleStore != nil && order.ParticipantID != "" {
		const defaultLimit = 100
		limit := defaultLimit

		if s.throttleConfigStore != nil && s.participantStore != nil {
			if participant, pErr := s.participantStore.Get(order.ParticipantID); pErr == nil {
				if tcfg, cErr := s.throttleConfigStore.Get(participant.FirmID); cErr == nil && tcfg.Enabled {
					limit = tcfg.MaxOrdersPerSecond
				}
			}
		}

		allowed, _ := s.throttleStore.CheckAndIncrement(order.ParticipantID, limit)
		if !allowed {
			s.writeError(w, http.StatusTooManyRequests, "THROTTLED",
				fmt.Sprintf("rate limit exceeded: max %d orders/sec", limit), nil)
			return
		}
	}

	// (a) instrument_id is required and must exist.
	if order.InstrumentID == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}
	inst, err := s.instrumentStore.Get(order.InstrumentID)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusUnprocessableEntity, "NOT_FOUND",
				fmt.Sprintf("instrument %s not found", order.InstrumentID), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// (b) instrument must be ACTIVE for trading.
	if inst.TradingStatus != types.TradingStatusActive {
		s.writeError(w, http.StatusUnprocessableEntity, "INSTRUMENT_NOT_ACTIVE",
			"instrument is not active for trading", nil)
		return
	}

	// (c) side must be BUY, SELL, or SHORT_SELL.
	if !validOrderSides[order.Side] {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
			fmt.Sprintf("invalid side %q: must be one of BUY, SELL, SHORT_SELL", order.Side), nil)
		return
	}

	// (i) SHORT_SELL: enforce short-sell compliance (instrument restriction,
	// locate presence, and locate validity/consumption) via the short-sell engine.
	if order.Side == types.OrderSideShortSell {
		sse := engine.NewShortSellEngine(s.instrumentStore, s.locateStore)
		if err := sse.EvaluateOrder(&order, inst); err != nil {
			if ssErr, ok := err.(*engine.ShortSellError); ok {
				s.writeError(w, ssErr.HTTPStatus(), ssErr.Code, ssErr.Message, nil)
				return
			}
			s.writeError(w, http.StatusUnprocessableEntity, "INVALID_LOCATE", err.Error(), nil)
			return
		}
	}

	// (d) order_type must be LIMIT, MARKET, STOP, or STOP_LIMIT.
	if !validOrderTypes[order.OrderType] {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
			fmt.Sprintf("invalid order_type %q: must be one of LIMIT, MARKET, STOP, STOP_LIMIT", order.OrderType), nil)
		return
	}

	// (e) quantity must be > 0.
	if order.Quantity <= 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
			"quantity must be greater than 0", nil)
		return
	}

	// (f) quantity must be a whole-lot multiple of instrument.LotSize.
	if inst.LotSize > 0 && order.Quantity%inst.LotSize != 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "INVALID_LOT_SIZE",
			fmt.Sprintf("quantity must be a multiple of lot_size (%d)", inst.LotSize), nil)
		return
	}

	// (g) For LIMIT and STOP_LIMIT: price must be > 0 and a valid tick-size multiple.
	if order.OrderType == types.OrderTypeLimit || order.OrderType == types.OrderTypeStopLimit {
		if order.Price <= 0 {
			s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
				"price must be greater than 0 for LIMIT and STOP_LIMIT orders", nil)
			return
		}

		// Part A: Try tiered tick table first, fall back to instrument default.
		tickValidated := false
		if s.tickTableStore != nil {
			tickTable, ttErr := s.tickTableStore.Get(order.InstrumentID)
			if ttErr == nil && tickTable != nil {
				if err := engine.ValidateTickSize(order.Price, tickTable, inst.TickSize); err != nil {
					s.writeError(w, http.StatusUnprocessableEntity, "INVALID_TICK_SIZE", err.Error(), nil)
					return
				}
				tickValidated = true
			}
			// If ErrNotFound or other error, fall through to default tick size check.
		}

		if !tickValidated && inst.TickSize > 0 {
			remainder := math.Remainder(order.Price, inst.TickSize)
			if math.Abs(remainder) > 1e-9 {
				s.writeError(w, http.StatusUnprocessableEntity, "INVALID_TICK_SIZE",
					fmt.Sprintf("price must be a multiple of tick_size (%g)", inst.TickSize), nil)
				return
			}
		}
	}

	// (h) For STOP and STOP_LIMIT: stop_price must be > 0.
	if order.OrderType == types.OrderTypeStop || order.OrderType == types.OrderTypeStopLimit {
		if order.StopPrice <= 0 {
			s.writeError(w, http.StatusUnprocessableEntity, "INVALID_FIELD",
				"stop_price must be greater than 0 for STOP and STOP_LIMIT orders", nil)
			return
		}
	}

	// (j) TradingParameterSet checks — all optional; skipped when no param set
	// is configured for this instrument.
	if s.tradingParamSetStore != nil {
		if ps, psErr := s.tradingParamSetStore.GetByInstrument(order.InstrumentID); psErr == nil && ps != nil {
			// Check allowed order types.
			if len(ps.AllowedOrderTypes) > 0 {
				allowed := false
				for _, ot := range ps.AllowedOrderTypes {
					if ot == string(order.OrderType) {
						allowed = true
						break
					}
				}
				if !allowed {
					s.writeError(w, http.StatusUnprocessableEntity, "ORDER_TYPE_NOT_ALLOWED",
						fmt.Sprintf("order_type %q is not permitted for this instrument", order.OrderType), nil)
					return
				}
			}
			// Check allowed time-in-force values.
			if len(ps.AllowedTimeInForce) > 0 && order.TimeInForce != "" {
				allowed := false
				for _, tif := range ps.AllowedTimeInForce {
					if tif == string(order.TimeInForce) {
						allowed = true
						break
					}
				}
				if !allowed {
					s.writeError(w, http.StatusUnprocessableEntity, "TIME_IN_FORCE_NOT_ALLOWED",
						fmt.Sprintf("time_in_force %q is not permitted for this instrument", order.TimeInForce), nil)
					return
				}
			}
			// Check minimum order size.
			if ps.MinOrderSize > 0 && order.Quantity < ps.MinOrderSize {
				s.writeError(w, http.StatusUnprocessableEntity, "ORDER_TOO_SMALL",
					fmt.Sprintf("quantity %d is below minimum order size %d", order.Quantity, ps.MinOrderSize), nil)
				return
			}
			// Check maximum order size.
			if ps.MaxOrderSize > 0 && order.Quantity > ps.MaxOrderSize {
				s.writeError(w, http.StatusUnprocessableEntity, "ORDER_TOO_LARGE",
					fmt.Sprintf("quantity %d exceeds maximum order size %d", order.Quantity, ps.MaxOrderSize), nil)
				return
			}
			// Check maximum order value (price * quantity).
			if ps.MaxOrderValue > 0 && order.Price > 0 {
				orderValue := order.Price * float64(order.Quantity)
				if orderValue > ps.MaxOrderValue {
					s.writeError(w, http.StatusUnprocessableEntity, "ORDER_VALUE_EXCEEDED",
						fmt.Sprintf("order value %.2f exceeds maximum allowed %.2f", orderValue, ps.MaxOrderValue), nil)
					return
				}
			}
			// Check short-selling allowance.
			if order.Side == types.OrderSideShortSell && !ps.ShortSellingAllowed {
				s.writeError(w, http.StatusUnprocessableEntity, "SHORT_SELLING_NOT_ALLOWED",
					"short selling is not permitted for this instrument by its trading parameter set", nil)
				return
			}
		}
	}

	// Set server-controlled defaults.
	id, err := newUUID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate id", nil)
		return
	}
	order.ID = id
	order.Status = types.OrderStatusPending
	order.FilledQuantity = 0
	order.AvgFillPrice = 0
	now := time.Now().UTC().Format(time.RFC3339)
	order.CreatedAt = now
	order.UpdatedAt = now
	if order.TimeInForce == "" {
		order.TimeInForce = types.TimeInForceGTC
	}

	if err := s.orderStore.Submit(&order); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}

	// Publish order created event (nil-safe: no-op if producer not configured).
	if err := kafka.PublishOrderCreated(s.producer, tenantID.String(), &order); err != nil {
		// Non-fatal: continue even if publishing fails.
		_ = err
	}

	// Audit log: order created (best-effort, do not fail the operation).
	if s.auditStore != nil {
		entryID, _ := newUUID()
		_ = s.auditStore.Log(types.AuditEntry{
			ID:         entryID,
			EntityType: "ORDER",
			EntityID:   order.ID,
			Action:     "CREATE",
			ActorID:    order.ParticipantID,
			TenantID:   tenantID.String(),
			Timestamp:  order.CreatedAt,
		})
	}

	// Route through SessionManager if available, otherwise fall back to direct matching.
	var trades []types.SecurityTrade
	if s.sessionManager != nil {
		matched, err := s.sessionManager.SubmitOrder(&order, tenantID.String())
		if err == nil {
			trades = matched
		}
		// Non-fatal: if matching/collection fails, the order is still stored as PENDING.
	} else if s.engine != nil {
		matched, err := s.engine.MatchOrder(tenantID.String(), &order)
		if err == nil {
			trades = matched
		}
		// Non-fatal: if matching fails, the order is still stored as PENDING.
	}
	if trades == nil {
		trades = []types.SecurityTrade{}
	}

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"tenant_id": tenantID.String(),
		"order":     order,
		"trades":    trades,
	})
}

// handleListOrders handles GET /api/v1/securities/orders.
//
// Query parameters (all optional):
//
//	instrument_id   — filter by instrument
//	participant_id  — filter by participant
//	status          — filter by OrderStatus enum value
//	side            — filter by OrderSide (BUY, SELL, SHORT_SELL)
//	order_type      — filter by OrderType (LIMIT, MARKET, STOP, STOP_LIMIT)
//	time_in_force   — filter by TimeInForce (GTC, IOC, FOK, DAY, GTD)
//	price_min       — minimum price (inclusive, float64)
//	price_max       — maximum price (inclusive, float64)
//	quantity_min    — minimum quantity (inclusive, int)
//	quantity_max    — maximum quantity (inclusive, int)
//	date_from       — created_at >= this RFC3339 or date string
//	date_to         — created_at <= this RFC3339 or date string
//	client_order_id — exact match on client_order_id field
//	firm_id         — exact match on firm_id field
//	limit           — page size (default 50)
//	offset          — number of records to skip (default 0)
//
// Response: {data:[...], total:N, filters_applied:[...]}
func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filters := store.OrderFilters{
		InstrumentID:  q.Get("instrument_id"),
		ParticipantID: q.Get("participant_id"),
		Status:        types.OrderStatus(q.Get("status")),
	}

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// --- Parse rich filter params ---
	sideFilter := types.OrderSide(q.Get("side"))
	orderTypeFilter := types.OrderType(q.Get("order_type"))
	tifFilter := types.TimeInForce(q.Get("time_in_force"))
	clientOrderIDFilter := q.Get("client_order_id")
	firmIDFilter := q.Get("firm_id")
	dateFromFilter := q.Get("date_from")
	dateToFilter := q.Get("date_to")

	var priceMin, priceMax float64
	var quantityMin, quantityMax int

	if v := q.Get("price_min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			priceMin = f
		}
	}
	if v := q.Get("price_max"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			priceMax = f
		}
	}
	if v := q.Get("quantity_min"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			quantityMin = i
		}
	}
	if v := q.Get("quantity_max"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			quantityMax = i
		}
	}

	// Track which filter params were actually provided.
	filtersApplied := make([]string, 0, 12)
	if filters.InstrumentID != "" {
		filtersApplied = append(filtersApplied, "instrument_id")
	}
	if filters.ParticipantID != "" {
		filtersApplied = append(filtersApplied, "participant_id")
	}
	if filters.Status != "" {
		filtersApplied = append(filtersApplied, "status")
	}
	if sideFilter != "" {
		filtersApplied = append(filtersApplied, "side")
	}
	if orderTypeFilter != "" {
		filtersApplied = append(filtersApplied, "order_type")
	}
	if tifFilter != "" {
		filtersApplied = append(filtersApplied, "time_in_force")
	}
	if priceMin > 0 {
		filtersApplied = append(filtersApplied, "price_min")
	}
	if priceMax > 0 {
		filtersApplied = append(filtersApplied, "price_max")
	}
	if quantityMin > 0 {
		filtersApplied = append(filtersApplied, "quantity_min")
	}
	if quantityMax > 0 {
		filtersApplied = append(filtersApplied, "quantity_max")
	}
	if dateFromFilter != "" {
		filtersApplied = append(filtersApplied, "date_from")
	}
	if dateToFilter != "" {
		filtersApplied = append(filtersApplied, "date_to")
	}
	if clientOrderIDFilter != "" {
		filtersApplied = append(filtersApplied, "client_order_id")
	}
	if firmIDFilter != "" {
		filtersApplied = append(filtersApplied, "firm_id")
	}

	all, err := s.orderStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Apply in-memory filters.
	filtered := all[:0]
	for _, o := range all {
		if sideFilter != "" && o.Side != sideFilter {
			continue
		}
		if orderTypeFilter != "" && o.OrderType != orderTypeFilter {
			continue
		}
		if tifFilter != "" && o.TimeInForce != tifFilter {
			continue
		}
		if priceMin > 0 && o.Price < priceMin {
			continue
		}
		if priceMax > 0 && o.Price > priceMax {
			continue
		}
		if quantityMin > 0 && o.Quantity < quantityMin {
			continue
		}
		if quantityMax > 0 && o.Quantity > quantityMax {
			continue
		}
		if dateFromFilter != "" && o.CreatedAt < dateFromFilter {
			continue
		}
		if dateToFilter != "" && o.CreatedAt > dateToFilter {
			continue
		}
		if clientOrderIDFilter != "" && o.ClientOrderID != clientOrderIDFilter {
			continue
		}
		if firmIDFilter != "" && o.FirmID != firmIDFilter {
			continue
		}
		filtered = append(filtered, o)
	}
	all = filtered

	total := len(all)

	// Apply offset.
	if offset >= total {
		all = []types.SecurityOrder{}
	} else {
		all = all[offset:]
	}

	// Apply limit.
	if len(all) > limit {
		all = all[:limit]
	}

	// Ensure JSON array is never null.
	if all == nil {
		all = []types.SecurityOrder{}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":             all,
		"total":            total,
		"limit":            limit,
		"offset":           offset,
		"filters_applied":  filtersApplied,
	})
}

// handleGetOrder handles GET /api/v1/securities/orders/{id}.
func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "order id is required", nil)
		return
	}

	order, err := s.orderStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("order %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, order)
}

// handleCancelOrder handles DELETE /api/v1/securities/orders/{id}.
//
// An order may only be cancelled if its status is PENDING or PARTIALLY_FILLED.
// Returns 409 CONFLICT if the order is in any other state.
func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	if err := s.checkPermission(r, types.PermOrderCancel); err != nil {
		s.writeError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
		return
	}
	// Extract tenant from context (set by TenantMiddleware).
	tenantID, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "TENANT_REQUIRED", "X-GarudaX-Tenant header is required", nil)
		return
	}

	id := extractLastSegment(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "order id is required", nil)
		return
	}

	// Fetch the order to check its current status.
	order, err := s.orderStore.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("order %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Only PENDING and PARTIALLY_FILLED orders may be cancelled.
	if order.Status != types.OrderStatusPending && order.Status != types.OrderStatusPartiallyFilled {
		s.writeError(w, http.StatusConflict, "INVALID_STATE",
			"order cannot be cancelled in current status", nil)
		return
	}

	if err := s.orderStore.Cancel(id); err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("order %s not found", id), nil)
			return
		}
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Fetch the updated order to return it.
	cancelled, err := s.orderStore.Get(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	// Update the UpdatedAt timestamp on the returned object.
	cancelled.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Publish order cancelled event (nil-safe: no-op if producer not configured).
	if err := kafka.PublishOrderCancelled(s.producer, tenantID.String(), cancelled); err != nil {
		// Non-fatal: continue even if publishing fails.
		_ = err
	}

	s.writeJSON(w, http.StatusOK, cancelled)
}

// massCancelRequest is the optional request body for POST .../orders/mass-cancel.
// All fields are optional — an empty body cancels all PENDING orders.
type massCancelRequest struct {
	InstrumentID  string          `json:"instrument_id"`
	ParticipantID string          `json:"participant_id"`
	Side          types.OrderSide `json:"side"`
}

// massCancelResponse is the response body for the mass-cancel endpoint.
type massCancelResponse struct {
	CancelledCount int                    `json:"cancelled_count"`
	Orders         []types.SecurityOrder  `json:"orders"`
}

// handleMassCancel handles POST /api/v1/securities/orders/mass-cancel.
//
// Filters all PENDING orders matching the optional request body filters, cancels
// each one, and returns the count and updated order list.
//
// Request body (all optional):
//
//	instrument_id  — restrict to orders for this instrument
//	participant_id — restrict to orders from this participant
//	side           — restrict to orders with this side (BUY, SELL, SHORT_SELL)
func (s *Server) handleMassCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if err := s.checkPermission(r, types.PermOrderMassCancel); err != nil {
		s.writeError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
		return
	}

	var req massCancelRequest
	// Body is optional — ignore decode errors for empty bodies.
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
			return
		}
	}

	// List all PENDING orders matching the filters.
	filters := store.OrderFilters{
		InstrumentID:  req.InstrumentID,
		ParticipantID: req.ParticipantID,
		Status:        types.OrderStatusPending,
	}

	pending, err := s.orderStore.List(filters)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Apply optional side filter in-memory.
	if req.Side != "" {
		filtered := pending[:0]
		for _, o := range pending {
			if o.Side == req.Side {
				filtered = append(filtered, o)
			}
		}
		pending = filtered
	}

	cancelled := make([]types.SecurityOrder, 0, len(pending))
	for _, o := range pending {
		if err := s.orderStore.Cancel(o.ID); err != nil {
			// Skip orders that are no longer cancellable (race condition).
			continue
		}
		updated, err := s.orderStore.Get(o.ID)
		if err != nil {
			continue
		}
		cancelled = append(cancelled, *updated)
	}

	s.writeJSON(w, http.StatusOK, massCancelResponse{
		CancelledCount: len(cancelled),
		Orders:         cancelled,
	})
}
