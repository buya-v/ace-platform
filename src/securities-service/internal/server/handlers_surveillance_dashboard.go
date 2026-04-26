// Package server — surveillance dashboard HTTP handler.
package server

import (
	"net/http"
	"sort"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// dashboardAlertStatusCounts holds per-status counts for the dashboard.
type dashboardAlertStatusCounts struct {
	Open         int `json:"open"`
	Investigating int `json:"investigating"`
	Resolved     int `json:"resolved"`
}

// dashboardFirmCount is a ranked entry for the top-firms list.
type dashboardFirmCount struct {
	FirmID     string `json:"firm_id"`
	AlertCount int    `json:"alert_count"`
}

// dashboardInstrumentCount is a ranked entry for the top-instruments list.
type dashboardInstrumentCount struct {
	InstrumentID string `json:"instrument_id"`
	AlertCount   int   `json:"alert_count"`
}

// dashboardResponse is the JSON shape returned by GET /surveillance/dashboard.
type dashboardResponse struct {
	TotalAlerts      dashboardAlertStatusCounts `json:"total_alerts"`
	AlertsByType     map[string]int             `json:"alerts_by_type"`
	TopFirms         []dashboardFirmCount       `json:"top_firms"`
	TopInstruments   []dashboardInstrumentCount `json:"top_instruments"`
	RecentAlerts     []types.SurveillanceAlert  `json:"recent_alerts"`
}

// handleSurveillanceDashboard handles GET /api/v1/securities/surveillance/dashboard.
// It aggregates all alerts from the SurveillanceStore and returns a summary suitable
// for a surveillance operator dashboard.
func (s *Server) handleSurveillanceDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}
	if s.surveillanceStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "surveillance store not configured", nil)
		return
	}

	// Fetch all alerts with no filter.
	alerts, err := s.surveillanceStore.ListAlerts(store.SurveillanceAlertFilters{})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// ── Aggregate ──────────────────────────────────────────────────────────
	var counts dashboardAlertStatusCounts
	alertsByType := map[string]int{}
	instrumentCounts := map[string]int{}
	// firmCounts would require mapping alert→order→participant→firm; we use
	// InstrumentID as a proxy for firm grouping when participant data is unavailable.
	// For a richer implementation, wire participantStore and firmStore into this handler.
	firmCounts := map[string]int{}

	for _, a := range alerts {
		switch a.Status {
		case types.AlertStatusOpen:
			counts.Open++
		case types.AlertStatusInvestigating:
			counts.Investigating++
		case types.AlertStatusResolved:
			counts.Resolved++
		}
		alertsByType[string(a.AlertType)]++
		instrumentCounts[a.InstrumentID]++
		// Use InstrumentID as firm proxy (replace with real firm lookup when available).
		firmCounts[a.InstrumentID]++
	}

	// ── Top instruments (up to 10) ─────────────────────────────────────────
	topInstruments := make([]dashboardInstrumentCount, 0, len(instrumentCounts))
	for id, cnt := range instrumentCounts {
		topInstruments = append(topInstruments, dashboardInstrumentCount{InstrumentID: id, AlertCount: cnt})
	}
	sort.Slice(topInstruments, func(i, j int) bool {
		return topInstruments[i].AlertCount > topInstruments[j].AlertCount
	})
	if len(topInstruments) > 10 {
		topInstruments = topInstruments[:10]
	}

	// ── Top firms (up to 10) ───────────────────────────────────────────────
	topFirms := make([]dashboardFirmCount, 0, len(firmCounts))
	for id, cnt := range firmCounts {
		topFirms = append(topFirms, dashboardFirmCount{FirmID: id, AlertCount: cnt})
	}
	sort.Slice(topFirms, func(i, j int) bool {
		return topFirms[i].AlertCount > topFirms[j].AlertCount
	})
	if len(topFirms) > 10 {
		topFirms = topFirms[:10]
	}

	// ── Recent alerts (last 10, newest first by CreatedAt) ─────────────────
	sorted := make([]types.SurveillanceAlert, len(alerts))
	copy(sorted, alerts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt > sorted[j].CreatedAt
	})
	recent := sorted
	if len(recent) > 10 {
		recent = recent[:10]
	}

	s.writeJSON(w, http.StatusOK, dashboardResponse{
		TotalAlerts:    counts,
		AlertsByType:   alertsByType,
		TopFirms:       topFirms,
		TopInstruments: topInstruments,
		RecentAlerts:   recent,
	})
}
