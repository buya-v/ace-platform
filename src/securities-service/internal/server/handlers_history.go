// Package server — history archive HTTP handlers.
package server

import (
	"net/http"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleHistoryOrders handles GET /api/v1/securities/history/orders
// Query params: date_from, date_to (ISO 8601 date-time strings)
func (s *Server) handleHistoryOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.historyStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "history store not configured", nil)
		return
	}
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	orders, err := s.historyStore.ListOrders(dateFrom, dateTo)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, orders)
}

// handleHistoryTrades handles GET /api/v1/securities/history/trades
// Query params: date_from, date_to (ISO 8601 date-time strings)
func (s *Server) handleHistoryTrades(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.historyStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "history store not configured", nil)
		return
	}
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	trades, err := s.historyStore.ListTrades(dateFrom, dateTo)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, trades)
}

// handleHistoryArchive handles POST /api/v1/securities/history/archive
// It moves completed orders and settled trades from the live stores to the history store.
// Returns a summary of how many records were archived.
func (s *Server) handleHistoryArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.historyStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "history store not configured", nil)
		return
	}

	archivedOrders := 0
	archivedTrades := 0
	now := time.Now().UTC().Format(time.RFC3339)

	// Archive completed/cancelled/rejected/expired orders.
	if s.orderStore != nil {
		orders, err := s.orderStore.List(store.OrderFilters{})
		if err == nil {
			for _, o := range orders {
				switch o.Status {
				case types.OrderStatusFilled, types.OrderStatusCancelled, types.OrderStatusRejected, types.OrderStatusExpired:
					o.ArchivedAt = now
					if aerr := s.historyStore.ArchiveOrder(o); aerr == nil {
						archivedOrders++
					}
				}
			}
		}
	}

	// Archive settled and failed trades.
	if s.tradeStore != nil {
		trades, err := s.tradeStore.List()
		if err == nil {
			for _, t := range trades {
				switch t.Status {
				case types.TradeStatusSettled, types.TradeStatusFailed, types.TradeStatusBusted:
					t.ArchivedAt = now
					if aerr := s.historyStore.ArchiveTrade(t); aerr == nil {
						archivedTrades++
					}
				}
			}
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"archived_orders": archivedOrders,
		"archived_trades": archivedTrades,
		"archived_at":     now,
	})
}

// handleHistory dispatches /api/v1/securities/history/* routes.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v1/securities/history/orders":
		s.handleHistoryOrders(w, r)
	case r.URL.Path == "/api/v1/securities/history/trades":
		s.handleHistoryTrades(w, r)
	case r.URL.Path == "/api/v1/securities/history/archive":
		s.handleHistoryArchive(w, r)
	default:
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found", nil)
	}
}

