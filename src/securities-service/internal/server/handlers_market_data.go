// Package server — market data HTTP handlers (P3b).
package server

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleMarketDataBook handles GET /api/v1/securities/market-data/book/{instrument_id}.
//
// Builds an order-book snapshot from PENDING orders for the given instrument.
// Bids are grouped by price descending; asks are grouped by price ascending.
func (s *Server) handleMarketDataBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	instrumentID := extractMarketDataID(r.URL.Path, "book")
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	orders, err := s.orderStore.List(store.OrderFilters{
		InstrumentID: instrumentID,
		Status:       types.OrderStatusPending,
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Aggregate price levels.
	buyLevels := map[float64]*types.PriceLevel{}
	sellLevels := map[float64]*types.PriceLevel{}

	for _, o := range orders {
		remaining := o.Quantity - o.FilledQuantity
		if remaining <= 0 {
			continue
		}
		// PriceLevel.Price is a market-data display float (out of money-math scope);
		// take the float64 view of the Decimal order price at this boundary.
		priceKey := o.Price.Float64()
		switch o.Side {
		case types.OrderSideBuy:
			pl := buyLevels[priceKey]
			if pl == nil {
				pl = &types.PriceLevel{Price: priceKey}
				buyLevels[priceKey] = pl
			}
			pl.Quantity += remaining
			pl.OrderCount++
		case types.OrderSideSell, types.OrderSideShortSell:
			pl := sellLevels[priceKey]
			if pl == nil {
				pl = &types.PriceLevel{Price: priceKey}
				sellLevels[priceKey] = pl
			}
			pl.Quantity += remaining
			pl.OrderCount++
		}
	}

	// Sort bids descending, asks ascending.
	bids := make([]types.PriceLevel, 0, len(buyLevels))
	for _, pl := range buyLevels {
		bids = append(bids, *pl)
	}
	sort.Slice(bids, func(i, j int) bool { return bids[i].Price > bids[j].Price })

	asks := make([]types.PriceLevel, 0, len(sellLevels))
	for _, pl := range sellLevels {
		asks = append(asks, *pl)
	}
	sort.Slice(asks, func(i, j int) bool { return asks[i].Price < asks[j].Price })

	snapshot := types.OrderBookSnapshot{
		InstrumentID: instrumentID,
		Bids:         bids,
		Asks:         asks,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

// handleMarketDataTicker handles GET /api/v1/securities/market-data/ticker/{instrument_id}.
//
// Returns the latest ticker summary: last traded price, best bid/ask, daily volume,
// day high, and day low.
func (s *Server) handleMarketDataTicker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	instrumentID := extractMarketDataID(r.URL.Path, "ticker")
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	trades, err := s.tradeStore.ListByInstrument(instrumentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	var lastTrade *types.SecurityTrade
	var dailyVolume int
	var dayHigh, dayLow float64

	for i := range trades {
		t := &trades[i]
		// Track most recent trade overall.
		if lastTrade == nil || t.CreatedAt > lastTrade.CreatedAt {
			lastTrade = t
		}
		// Daily stats (trade date prefix match).
		if strings.HasPrefix(t.TradeDate, today) {
			dailyVolume += t.Quantity
			tp := t.Price.Float64()
			if dayHigh == 0 || tp > dayHigh {
				dayHigh = tp
			}
			if dayLow == 0 || tp < dayLow {
				dayLow = tp
			}
		}
	}

	var lastPrice float64
	if lastTrade != nil {
		lastPrice = lastTrade.Price.Float64()
	}

	// Best bid / best ask from PENDING orders.
	orders, err := s.orderStore.List(store.OrderFilters{
		InstrumentID: instrumentID,
		Status:       types.OrderStatusPending,
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	var bestBid, bestAsk float64
	for _, o := range orders {
		if o.Quantity-o.FilledQuantity <= 0 {
			continue
		}
		op := o.Price.Float64()
		switch o.Side {
		case types.OrderSideBuy:
			if bestBid == 0 || op > bestBid {
				bestBid = op
			}
		case types.OrderSideSell, types.OrderSideShortSell:
			if bestAsk == 0 || op < bestAsk {
				bestAsk = op
			}
		}
	}

	ticker := types.TickerData{
		InstrumentID: instrumentID,
		LastPrice:    lastPrice,
		BidPrice:     bestBid,
		AskPrice:     bestAsk,
		Volume:       dailyVolume,
		DayHigh:      dayHigh,
		DayLow:       dayLow,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
	s.writeJSON(w, http.StatusOK, ticker)
}

// handleMarketDataTrades handles GET /api/v1/securities/market-data/trades/{instrument_id}.
//
// Returns the trade history for the given instrument.
func (s *Server) handleMarketDataTrades(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	instrumentID := extractMarketDataID(r.URL.Path, "trades")
	if instrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "instrument_id is required", nil)
		return
	}

	trades, err := s.tradeStore.ListByInstrument(instrumentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if trades == nil {
		trades = []types.SecurityTrade{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  trades,
		"total": len(trades),
	})
}

// extractMarketDataID extracts the instrument_id from a path such as
// /api/v1/securities/market-data/{segment}/{instrument_id}.
// segment is one of "book", "ticker", "trades".
func extractMarketDataID(path, segment string) string {
	// path looks like: /api/v1/securities/market-data/book/TICKER123
	marker := "/market-data/" + segment + "/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	id := path[idx+len(marker):]
	id = strings.TrimSuffix(id, "/")
	return id
}
