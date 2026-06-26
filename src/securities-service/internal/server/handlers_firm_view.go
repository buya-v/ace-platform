// Package server — firm-view surveillance aggregation handler.
package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleFirmView handles GET /api/v1/securities/surveillance/firm-view/{firm_id}.
// It aggregates orders, trades, positions, and alerts for a single firm and
// returns a unified surveillance snapshot.
func (s *Server) handleFirmView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	// Extract firm_id from path: /api/v1/securities/surveillance/firm-view/{firm_id}
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	firmID := parts[len(parts)-1]
	if firmID == "" || firmID == "firm-view" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELD", "firm_id is required", nil)
		return
	}

	// ── Orders ────────────────────────────────────────────────────────────────
	type ordersSummary struct {
		Total     int `json:"total"`
		Pending   int `json:"pending"`
		Filled    int `json:"filled"`
		Cancelled int `json:"cancelled"`
	}
	var orders ordersSummary
	var allOrders []types.SecurityOrder

	if s.orderStore != nil {
		// List all participants for this firm so we can filter orders by participant.
		participantIDs := []string{}
		if s.participantStore != nil {
			ps, _ := s.participantStore.List(store.ParticipantFilters{FirmID: firmID})
			for _, p := range ps {
				participantIDs = append(participantIDs, p.ID)
			}
		}

		// Collect orders across all participants in this firm.
		seen := map[string]bool{}
		for _, pid := range participantIDs {
			os, err := s.orderStore.List(store.OrderFilters{ParticipantID: pid})
			if err == nil {
				for _, o := range os {
					if seen[o.ID] {
						continue
					}
					seen[o.ID] = true
					allOrders = append(allOrders, o)
					orders.Total++
					switch o.Status {
					case types.OrderStatusPending, types.OrderStatusPartiallyFilled:
						orders.Pending++
					case types.OrderStatusFilled:
						orders.Filled++
					case types.OrderStatusCancelled:
						orders.Cancelled++
					}
				}
			}
		}
	}

	// ── Trades ────────────────────────────────────────────────────────────────
	type tradesSummary struct {
		Total      int           `json:"total"`
		BuyVolume  types.Decimal `json:"buy_volume"`
		SellVolume types.Decimal `json:"sell_volume"`
	}
	var trades tradesSummary
	var allTrades []types.SecurityTrade

	if s.tradeStore != nil {
		ts, err := s.tradeStore.List()
		if err == nil {
			// Build a set of participant IDs that belong to this firm.
			firmParticipants := map[string]bool{}
			if s.participantStore != nil {
				ps, _ := s.participantStore.List(store.ParticipantFilters{FirmID: firmID})
				for _, p := range ps {
					firmParticipants[p.ID] = true
				}
			}

			// Map orders to participants so we can attribute trades to a firm.
			orderParticipant := map[string]string{}
			for _, o := range allOrders {
				orderParticipant[o.ID] = o.ParticipantID
			}

			for _, t := range ts {
				buyPID := orderParticipant[t.BuyOrderID]
				sellPID := orderParticipant[t.SellOrderID]
				isFirmBuy := firmParticipants[buyPID]
				isFirmSell := firmParticipants[sellPID]
				if !isFirmBuy && !isFirmSell {
					continue
				}
				allTrades = append(allTrades, t)
				trades.Total++
				value := t.Price.MulInt64(int64(t.Quantity))
				if isFirmBuy {
					trades.BuyVolume = trades.BuyVolume.Add(value)
				}
				if isFirmSell {
					trades.SellVolume = trades.SellVolume.Add(value)
				}
			}
		}
	}

	// ── Positions ─────────────────────────────────────────────────────────────
	var positions []types.Position
	if s.positionStore != nil && s.participantStore != nil {
		ps, _ := s.participantStore.List(store.ParticipantFilters{FirmID: firmID})
		for _, p := range ps {
			pp, err := s.positionStore.List(p.ID)
			if err == nil {
				positions = append(positions, pp...)
			}
		}
	}
	if positions == nil {
		positions = []types.Position{}
	}

	// ── Alerts ────────────────────────────────────────────────────────────────
	type alertsSummary struct {
		Total    int `json:"total"`
		Open     int `json:"open"`
		Resolved int `json:"resolved"`
	}
	var alerts alertsSummary

	if s.surveillanceStore != nil {
		as, err := s.surveillanceStore.ListAlerts(store.SurveillanceAlertFilters{})
		if err == nil {
			for _, a := range as {
				alerts.Total++
				switch a.Status {
				case types.AlertStatusOpen, types.AlertStatusInvestigating:
					alerts.Open++
				case types.AlertStatusResolved:
					alerts.Resolved++
				}
			}
		}
	}

	// ── Recent Orders (last 10, newest first) ─────────────────────────────────
	recentOrders := allOrders
	sort.Slice(recentOrders, func(i, j int) bool {
		return recentOrders[i].CreatedAt > recentOrders[j].CreatedAt
	})
	if len(recentOrders) > 10 {
		recentOrders = recentOrders[:10]
	}
	if recentOrders == nil {
		recentOrders = []types.SecurityOrder{}
	}

	// ── Recent Trades (last 10, newest first) ─────────────────────────────────
	recentTrades := allTrades
	sort.Slice(recentTrades, func(i, j int) bool {
		return recentTrades[i].CreatedAt > recentTrades[j].CreatedAt
	})
	if len(recentTrades) > 10 {
		recentTrades = recentTrades[:10]
	}
	if recentTrades == nil {
		recentTrades = []types.SecurityTrade{}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"firm_id":       firmID,
		"orders":        orders,
		"trades":        trades,
		"positions":     positions,
		"alerts":        alerts,
		"recent_orders": recentOrders,
		"recent_trades": recentTrades,
	})
}
