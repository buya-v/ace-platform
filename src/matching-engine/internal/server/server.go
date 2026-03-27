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
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ace-platform/matching-engine/internal/engine"
	"github.com/ace-platform/matching-engine/internal/orderbook"
	"github.com/ace-platform/matching-engine/internal/store"
	"github.com/ace-platform/matching-engine/internal/types"
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

// StartHealthServer starts the HTTP health check server for Kubernetes probes.
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

	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.HealthPort)
	log.Printf("Health server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
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
