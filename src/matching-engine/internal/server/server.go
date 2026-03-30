// Package server provides the gRPC server for the matching engine.
//
// Architecture notes:
// - Each instrument's order book is single-threaded (per T007 spec Section 5).
// - The engine layer handles per-instrument locking.
// - This server layer handles request validation, type conversion, and response mapping.
// - For direct pod-to-pod communication (bypassing Istio sidecar), the server
//   binds to 0.0.0.0 and clients connect directly to the pod IP.
//   Kubernetes service annotation: traffic.sidecar.istio.io/excludeInboundPorts: "50051"
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/matching-engine/internal/engine"
	"github.com/garudax-platform/matching-engine/internal/orderbook"
	"github.com/garudax-platform/matching-engine/internal/store"
	"github.com/garudax-platform/matching-engine/internal/types"
)

// Server is the matching engine gRPC server.
type Server struct {
	engine     *engine.Engine
	tradeStore store.TradeStore
	config     Config
	ready      int32 // atomic: 1 = ready
}

// NewServer creates a new matching engine server.
func NewServer(eng *engine.Engine, ts store.TradeStore, cfg Config) *Server {
	return &Server{
		engine:     eng,
		tradeStore: ts,
		config:     cfg,
	}
}

// SubmitOrder validates and submits an order to the matching engine.
func (s *Server) SubmitOrder(req SubmitOrderRequest) (types.ExecutionReport, error) {
	if req.InstrumentID == "" {
		return types.ExecutionReport{}, fmt.Errorf("instrument_id is required")
	}
	if req.AccountID == "" {
		return types.ExecutionReport{}, fmt.Errorf("account_id is required")
	}
	if req.Quantity == 0 {
		return types.ExecutionReport{}, fmt.Errorf("quantity must be greater than zero")
	}

	price, err := types.ParseDecimal(req.Price)
	if err != nil {
		return types.ExecutionReport{}, fmt.Errorf("invalid price: %w", err)
	}
	stopPrice, err := types.ParseDecimal(req.StopPrice)
	if err != nil {
		return types.ExecutionReport{}, fmt.Errorf("invalid stop_price: %w", err)
	}

	order := &types.Order{
		OrderID:       req.OrderID,
		ClientOrderID: req.ClientOrderID,
		InstrumentID:  req.InstrumentID,
		AccountID:     req.AccountID,
		Side:          req.Side,
		OrderType:     req.OrderType,
		TimeInForce:   req.TimeInForce,
		Price:         price,
		StopPrice:     stopPrice,
		Quantity:      req.Quantity,
		STPMode:       req.STPMode,
		ExpireAt:      req.ExpireAt,
	}

	result, err := s.engine.SubmitOrder(order)
	if err != nil {
		return types.ExecutionReport{}, err
	}

	// Persist trades (append-only)
	for _, trade := range result.Trades {
		if storeErr := s.tradeStore.Append(trade); storeErr != nil {
			log.Printf("ERROR: failed to persist trade %s: %v", trade.TradeID, storeErr)
		}
	}

	// Return the first execution report (the order acknowledgement)
	if len(result.ExecutionReports) > 0 {
		return result.ExecutionReports[0], nil
	}
	return types.ExecutionReport{}, fmt.Errorf("no execution report generated")
}

// CancelOrder cancels an existing order.
func (s *Server) CancelOrder(instrumentID, orderID, accountID string) (types.ExecutionReport, error) {
	if instrumentID == "" {
		return types.ExecutionReport{}, fmt.Errorf("instrument_id is required")
	}
	if orderID == "" {
		return types.ExecutionReport{}, fmt.Errorf("order_id is required")
	}
	return s.engine.CancelOrder(instrumentID, orderID)
}

// CancelAllOrders cancels all orders for an account.
func (s *Server) CancelAllOrders(instrumentID, accountID string, side types.Side) (uint32, []string, error) {
	if instrumentID == "" {
		return 0, nil, fmt.Errorf("instrument_id is required for cancel all")
	}
	if accountID == "" {
		return 0, nil, fmt.Errorf("account_id is required")
	}
	return s.engine.CancelAll(instrumentID, accountID, side)
}

// ModifyOrder modifies an existing order with cancel-replace semantics.
func (s *Server) ModifyOrder(instrumentID, orderID, accountID, newPrice string, newQty uint64) (types.ExecutionReport, error) {
	if instrumentID == "" {
		return types.ExecutionReport{}, fmt.Errorf("instrument_id is required")
	}

	price, err := types.ParseDecimal(newPrice)
	if err != nil {
		return types.ExecutionReport{}, fmt.Errorf("invalid new_price: %w", err)
	}

	result, err := s.engine.ModifyOrder(instrumentID, orderID, accountID, price, newQty)
	if err != nil {
		return types.ExecutionReport{}, err
	}

	// Persist any trades from the replacement order matching
	for _, trade := range result.Trades {
		if storeErr := s.tradeStore.Append(trade); storeErr != nil {
			log.Printf("ERROR: failed to persist trade %s: %v", trade.TradeID, storeErr)
		}
	}

	if len(result.ExecutionReports) > 0 {
		return result.ExecutionReports[0], nil
	}
	return types.ExecutionReport{}, fmt.Errorf("no execution report generated")
}

// GetOrder retrieves an order by ID.
func (s *Server) GetOrder(instrumentID, orderID string) (*types.Order, error) {
	return s.engine.GetOrder(instrumentID, orderID)
}

// GetOrderBookSnapshot returns an L2 order book snapshot.
func (s *Server) GetOrderBookSnapshot(instrumentID string, depth uint32) (*OrderBookSnapshot, error) {
	book, err := s.engine.GetOrderBook(instrumentID)
	if err != nil {
		return nil, err
	}

	if depth == 0 {
		depth = 10
	}

	snap := &OrderBookSnapshot{
		InstrumentID:   instrumentID,
		LastTradePrice: book.LastTradePrice,
		State:          book.State,
	}

	for i, level := range book.BidLevels() {
		if uint32(i) >= depth {
			break
		}
		snap.Bids = append(snap.Bids, PriceLevel{
			Price:      level.Price,
			Quantity:   level.TotalQty,
			OrderCount: level.OrderCount,
		})
	}

	for i, level := range book.AskLevels() {
		if uint32(i) >= depth {
			break
		}
		snap.Asks = append(snap.Asks, PriceLevel{
			Price:      level.Price,
			Quantity:   level.TotalQty,
			OrderCount: level.OrderCount,
		})
	}

	return snap, nil
}

// GetLastTrade returns the most recent trade for an instrument.
func (s *Server) GetLastTrade(instrumentID string) (types.Trade, error) {
	trade, ok := s.tradeStore.LastTrade(instrumentID)
	if !ok {
		return types.Trade{}, fmt.Errorf("no trades for instrument %s", instrumentID)
	}
	return trade, nil
}

// RegisterInstrument registers a new instrument for trading.
func (s *Server) RegisterInstrument(instrumentID string) error {
	return s.engine.RegisterInstrument(instrumentID)
}

// SetReady marks the server as ready to serve traffic.
func (s *Server) SetReady() {
	atomic.StoreInt32(&s.ready, 1)
}

// IsReady returns true if the server is ready.
func (s *Server) IsReady() bool {
	return atomic.LoadInt32(&s.ready) == 1
}

// StartHealthServer starts the HTTP health check server for Kubernetes probes
// and the REST API for order management.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	// --- REST API endpoints ---
	mux.HandleFunc("/orders", s.handleOrders)
	mux.HandleFunc("/book/", s.handleGetBook)
	mux.HandleFunc("/book", s.handleGetBook)
	mux.HandleFunc("/trades/latest/", s.handleGetLastTrade)
	mux.HandleFunc("/trades/latest", s.handleGetLastTrade)
	mux.HandleFunc("/circuit-breakers", s.handleGetCircuitBreakers)
	mux.HandleFunc("/instruments", s.handleListInstruments)

	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.HealthPort)
	log.Printf("Health/API server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// handleOrders dispatches POST /orders (submit) and DELETE /orders (cancel).
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodPost:
		s.handleSubmitOrder(w, r)
	case http.MethodDelete:
		s.handleCancelOrder(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
	}
}

// submitOrderJSON is the JSON request body for POST /orders.
type submitOrderJSON struct {
	InstrumentID string `json:"instrument_id"`
	AccountID    string `json:"account_id"`
	Side         string `json:"side"`
	Type         string `json:"type"`
	Quantity     string `json:"quantity"`
	Price        string `json:"price"`
	OrderID      string `json:"order_id"`
	TimeInForce  string `json:"time_in_force"`
}

func (s *Server) handleSubmitOrder(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
		return
	}

	var req submitOrderJSON
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	// Parse side
	side := parseSide(req.Side)

	// Parse order type
	orderType := parseOrderType(req.Type)

	// Parse quantity (accept both string and number in JSON)
	qty, _ := strconv.ParseUint(req.Quantity, 10, 64)
	if qty == 0 {
		// Try parsing as float for values like "10"
		if f, err := strconv.ParseFloat(req.Quantity, 64); err == nil && f > 0 {
			qty = uint64(f)
		}
	}

	// Parse time in force
	tif := parseTIF(req.TimeInForce)

	// Use account_id from body, or fall back to x-user-id / x-participant-id header
	accountID := req.AccountID
	if accountID == "" {
		accountID = r.Header.Get("X-Participant-Id")
	}
	if accountID == "" {
		accountID = r.Header.Get("X-User-Id")
	}
	if accountID == "" {
		accountID = "anonymous"
	}

	report, err := s.SubmitOrder(SubmitOrderRequest{
		OrderID:     req.OrderID,
		InstrumentID: req.InstrumentID,
		AccountID:   accountID,
		Side:        side,
		OrderType:   orderType,
		TimeInForce: tif,
		Price:       req.Price,
		Quantity:    qty,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(execReportToJSON(report))
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.URL.Query().Get("order_id")
	instrumentID := r.URL.Query().Get("instrument_id")
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		accountID = r.Header.Get("X-Participant-Id")
	}

	if orderID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "order_id is required"})
		return
	}
	if instrumentID == "" {
		instrumentID = "WHT-HRW-2026M07-UB" // default instrument
	}

	report, err := s.CancelOrder(instrumentID, orderID, accountID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(execReportToJSON(report))
}

// handleGetBook handles GET /book/{instrument_id}.
func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Extract instrument_id from URL path: /book/{instrument_id}
	instrumentID := strings.TrimPrefix(r.URL.Path, "/book/")
	if instrumentID == "" {
		instrumentID = r.URL.Query().Get("instrument_id")
	}
	if instrumentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instrument_id is required"})
		return
	}

	depthStr := r.URL.Query().Get("depth")
	var depth uint32 = 10
	if depthStr != "" {
		if d, err := strconv.ParseUint(depthStr, 10, 32); err == nil {
			depth = uint32(d)
		}
	}

	snap, err := s.GetOrderBookSnapshot(instrumentID, depth)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]interface{}{
		"instrument_id":    snap.InstrumentID,
		"last_trade_price": snap.LastTradePrice.String(),
		"state":            int(snap.State),
		"bids":             priceLevelsToJSON(snap.Bids),
		"asks":             priceLevelsToJSON(snap.Asks),
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleGetLastTrade handles GET /trades/latest/{instrument_id}.
func (s *Server) handleGetLastTrade(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Extract instrument_id from URL path: /trades/latest/{instrument_id}
	instrumentID := strings.TrimPrefix(r.URL.Path, "/trades/latest/")
	if instrumentID == "" {
		instrumentID = r.URL.Query().Get("instrument_id")
	}
	if instrumentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instrument_id is required"})
		return
	}

	trade, err := s.GetLastTrade(instrumentID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tradeToJSON(trade))
}

// handleGetCircuitBreakers returns circuit breaker status for all instruments.
func (s *Server) handleGetCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	breakers := s.engine.GetCircuitBreakers()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"circuit_breakers": breakers,
		"total":            len(breakers),
	})
}

// handleListInstruments returns all registered instruments.
func (s *Server) handleListInstruments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	instruments := s.engine.ListInstruments()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instruments": instruments,
		"total":       len(instruments),
	})
}

// --- JSON helpers ---

func parseSide(s string) types.Side {
	switch strings.ToUpper(s) {
	case "BUY":
		return types.SideBuy
	case "SELL":
		return types.SideSell
	default:
		return types.SideBuy
	}
}

func parseOrderType(s string) types.OrderType {
	switch strings.ToUpper(s) {
	case "LIMIT":
		return types.OrderTypeLimit
	case "MARKET":
		return types.OrderTypeMarket
	case "STOP_LIMIT":
		return types.OrderTypeStopLimit
	case "STOP_MARKET":
		return types.OrderTypeStopMarket
	default:
		return types.OrderTypeLimit
	}
}

func parseTIF(s string) types.TimeInForce {
	switch strings.ToUpper(s) {
	case "DAY":
		return types.TIFDay
	case "GTC":
		return types.TIFGTC
	case "GTD":
		return types.TIFGTD
	case "IOC":
		return types.TIFIOC
	case "FOK":
		return types.TIFFOK
	default:
		return types.TIFGTC
	}
}

func execReportToJSON(r types.ExecutionReport) map[string]interface{} {
	return map[string]interface{}{
		"order_id":        r.OrderID,
		"id":              r.OrderID,
		"exec_id":         r.ExecID,
		"client_order_id": r.ClientOrderID,
		"exec_type":       r.ExecType,
		"order_status":    r.OrderStatus.String(),
		"side":            r.Side.String(),
		"instrument_id":   r.InstrumentID,
		"price":           r.Price.String(),
		"quantity":        r.Quantity,
		"last_qty":        r.LastQty,
		"last_price":      r.LastPrice.String(),
		"cumulative_qty":  r.CumulativeQty,
		"leaves_qty":      r.LeavesQty,
		"trade_id":        r.TradeID,
		"account_id":      r.AccountID,
	}
}

func tradeToJSON(t types.Trade) map[string]interface{} {
	return map[string]interface{}{
		"trade_id":       t.TradeID,
		"instrument_id":  t.InstrumentID,
		"buy_order_id":   t.BuyOrderID,
		"sell_order_id":  t.SellOrderID,
		"price":          t.Price.String(),
		"quantity":       t.Quantity,
		"aggressor_side": t.AggressorSide.String(),
		"executed_at":    t.ExecutedAt.Format(time.RFC3339Nano),
	}
}

func priceLevelsToJSON(levels []PriceLevel) []map[string]interface{} {
	result := make([]map[string]interface{}, len(levels))
	for i, l := range levels {
		result[i] = map[string]interface{}{
			"price":       l.Price.String(),
			"quantity":    l.Quantity,
			"order_count": l.OrderCount,
		}
	}
	return result
}

// GRPCAddr returns the address the gRPC server should bind to.
func (s *Server) GRPCAddr() string {
	return fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.GRPCPort)
}

// ListenGRPC creates a TCP listener for the gRPC server.
// Binds to 0.0.0.0 for direct pod-to-pod communication (bypassing Istio sidecar).
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := s.GRPCAddr()
	log.Printf("gRPC server listening on %s (direct_pod_comms=%v)", addr, s.config.DirectPodComms)
	return net.Listen("tcp", addr)
}

// SubmitOrderRequest is the server-level request for order submission.
type SubmitOrderRequest struct {
	OrderID       string
	ClientOrderID string
	InstrumentID  string
	AccountID     string
	Side          types.Side
	OrderType     types.OrderType
	TimeInForce   types.TimeInForce
	Price         string // decimal string
	StopPrice     string // decimal string
	Quantity      uint64
	STPMode       types.STPMode
	ExpireAt      time.Time
}

// OrderBookSnapshot is the server-level order book snapshot.
type OrderBookSnapshot struct {
	InstrumentID   string
	Bids           []PriceLevel
	Asks           []PriceLevel
	LastTradePrice types.Decimal
	State          types.BookState
}

// PriceLevel is a summary of a price level in the book.
type PriceLevel struct {
	Price      types.Decimal
	Quantity   uint64
	OrderCount uint32
}

// ensure IDGenerator is imported for reference
var _ orderbook.IDGenerator = (*uuidGen)(nil)

// uuidGen is a placeholder for UUID v7 generation. Referenced for interface compliance.
type uuidGen struct{}

func (u *uuidGen) NewID() string { return "" }
