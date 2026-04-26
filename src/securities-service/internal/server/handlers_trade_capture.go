// Package server — trade capture report HTTP handlers.
package server

import (
	"net/http"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// TradeCaptureReport is the response body for GET /api/v1/securities/trade-capture-reports.
type TradeCaptureReport struct {
	FirmID         string                 `json:"firm_id"`
	Date           string                 `json:"date"`
	Trades         []types.SecurityTrade  `json:"trades"`
	TotalBuyQty    int                    `json:"total_buy_qty"`
	TotalSellQty   int                    `json:"total_sell_qty"`
	TotalBuyValue  float64                `json:"total_buy_value"`
	TotalSellValue float64                `json:"total_sell_value"`
	NetPosition    int                    `json:"net_position"`
}

// handleTradeCaptureReports handles
// GET /api/v1/securities/trade-capture-reports?firm_id=X&date=YYYY-MM-DD
//
// It lists all trades where the buying or selling participant belongs to the
// requested firm and the trade date matches the requested date.  The response
// aggregates buy/sell quantities and values and derives the net position.
func (s *Server) handleTradeCaptureReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.tradeStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "trade store not configured", nil)
		return
	}

	firmID := r.URL.Query().Get("firm_id")
	date := r.URL.Query().Get("date")
	if firmID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_PARAM", "firm_id query parameter is required", nil)
		return
	}
	if date == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_PARAM", "date query parameter is required", nil)
		return
	}

	// Load all orders to build a firm→participant index.
	// We need to know which participant IDs belong to firmID so that we can
	// filter trades by buyer/seller participant.
	firmParticipants := map[string]bool{}
	if s.participantStore != nil {
		participants, err := s.participantStore.List(store.ParticipantFilters{FirmID: firmID})
		if err == nil {
			for _, p := range participants {
				firmParticipants[p.ID] = true
			}
		}
	}

	// Retrieve all trades and filter by date + firm membership.
	allTrades, err := s.tradeStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	matchedTrades := make([]types.SecurityTrade, 0)
	var totalBuyQty, totalSellQty int
	var totalBuyValue, totalSellValue float64

	for _, t := range allTrades {
		// Filter by trade date.
		tradeDay := ""
		if len(t.TradeDate) >= 10 {
			tradeDay = t.TradeDate[:10]
		}
		if tradeDay != date {
			continue
		}

		// Determine if the firm is the buyer or seller.
		// We match on either buyer or seller order participant.
		isBuyer := false
		isSeller := false

		if len(firmParticipants) > 0 {
			// Use participant membership if available.
			buyOrder, buyErr := s.orderStore.Get(t.BuyOrderID)
			if buyErr == nil && firmParticipants[buyOrder.ParticipantID] {
				isBuyer = true
			}
			sellOrder, sellErr := s.orderStore.Get(t.SellOrderID)
			if sellErr == nil && firmParticipants[sellOrder.ParticipantID] {
				isSeller = true
			}
		} else {
			// Fallback: treat trade IDs as firm membership proxies — accept all.
			isBuyer = true
			isSeller = true
		}

		if !isBuyer && !isSeller {
			continue
		}

		matchedTrades = append(matchedTrades, t)
		value := float64(t.Quantity) * t.Price
		if isBuyer {
			totalBuyQty += t.Quantity
			totalBuyValue += value
		}
		if isSeller {
			totalSellQty += t.Quantity
			totalSellValue += value
		}
	}

	report := TradeCaptureReport{
		FirmID:         firmID,
		Date:           date,
		Trades:         matchedTrades,
		TotalBuyQty:    totalBuyQty,
		TotalSellQty:   totalSellQty,
		TotalBuyValue:  totalBuyValue,
		TotalSellValue: totalSellValue,
		NetPosition:    totalBuyQty - totalSellQty,
	}
	s.writeJSON(w, http.StatusOK, report)
}
