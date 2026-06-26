// Package server — FRC regulatory report HTTP handlers.
package server

import (
	"net/http"
	"time"

	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleFRCReport handles GET /api/v1/securities/reports/frc.
//
// Query parameters:
//
//	type — report type: DAILY_SUMMARY | LARGE_TRADER | SUSPICIOUS_ACTIVITY
//	date — report date in YYYY-MM-DD format (defaults to today)
func (s *Server) handleFRCReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	q := r.URL.Query()
	reportType := q.Get("type")
	reportDate := q.Get("date")

	if reportType == "" {
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "type query parameter is required", nil)
		return
	}
	if reportDate == "" {
		reportDate = time.Now().UTC().Format("2006-01-02")
	}

	tenantID, _ := middleware.TenantFromContext(r.Context())

	var data map[string]interface{}
	var err error

	switch reportType {
	case "DAILY_SUMMARY":
		data, err = s.buildDailySummary(reportDate)
	case "LARGE_TRADER":
		data, err = s.buildLargeTraderReport(reportDate)
	case "SUSPICIOUS_ACTIVITY":
		data, err = s.buildSuspiciousActivityReport(reportDate)
	default:
		s.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR",
			"unknown report type: must be DAILY_SUMMARY, LARGE_TRADER, or SUSPICIOUS_ACTIVITY", nil)
		return
	}

	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	id, genErr := newUUID()
	if genErr != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate report id", nil)
		return
	}

	report := types.FRCReport{
		ID:          id,
		TenantID:    string(tenantID),
		ReportType:  reportType,
		ReportDate:  reportDate,
		Data:        data,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.writeJSON(w, http.StatusOK, report)
}

// buildDailySummary computes trade count, total volume and total value for the given date.
// Iterates all instruments then collects their trades, filtering by date prefix.
func (s *Server) buildDailySummary(date string) (map[string]interface{}, error) {
	instruments, err := s.instrumentStore.List(store.InstrumentFilters{})
	if err != nil {
		return nil, err
	}

	tradeCount := 0
	totalVolume := 0
	var totalValue types.Decimal

	for _, inst := range instruments {
		trades, err := s.tradeStore.ListByInstrument(inst.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range trades {
			// TradeDate may be RFC3339 or YYYY-MM-DD; match on date prefix.
			if len(t.TradeDate) >= 10 && t.TradeDate[:10] == date {
				tradeCount++
				totalVolume += t.Quantity
				totalValue = totalValue.Add(t.Price.MulInt64(int64(t.Quantity)))
			}
		}
	}

	return map[string]interface{}{
		"date":         date,
		"trade_count":  tradeCount,
		"total_volume": totalVolume,
		"total_value":  totalValue,
	}, nil
}

// buildLargeTraderReport returns all positions with quantity > 1000 shares.
func (s *Server) buildLargeTraderReport(date string) (map[string]interface{}, error) {
	// List all positions by passing empty participantID (returns all).
	allPositions, err := s.positionStore.List("")
	if err != nil {
		return nil, err
	}

	type largeTraderEntry struct {
		ParticipantID string `json:"participant_id"`
		InstrumentID  string `json:"instrument_id"`
		Quantity      int    `json:"quantity"`
	}

	entries := make([]largeTraderEntry, 0)
	for _, pos := range allPositions {
		if pos.Quantity > 1000 {
			entries = append(entries, largeTraderEntry{
				ParticipantID: pos.ParticipantID,
				InstrumentID:  pos.InstrumentID,
				Quantity:      pos.Quantity,
			})
		}
	}

	return map[string]interface{}{
		"date":                   date,
		"threshold_shares":       1000,
		"large_trader_positions": entries,
		"count":                  len(entries),
	}, nil
}

// buildSuspiciousActivityReport returns a placeholder empty list.
func (s *Server) buildSuspiciousActivityReport(date string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"date":                date,
		"suspicious_activity": []interface{}{},
		"count":               0,
		"note":                "automated surveillance pending integration",
	}, nil
}
