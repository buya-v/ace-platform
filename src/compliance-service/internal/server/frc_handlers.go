// Package server — HTTP handlers for the FRC regulatory reporting pipeline.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/compliance-service/reporting"
)

// frcReportRequest is the POST body for generating an FRC report. Only the
// fields relevant to the requested report_type need be supplied.
type frcReportRequest struct {
	ReportType  reporting.ReportType                 `json:"report_type"`
	Format      reporting.Format                     `json:"format"`
	SessionDate string                               `json:"session_date"`
	Instruments []reporting.InstrumentVolume         `json:"instruments"`
	LargeTrader *reporting.LargeTraderPosition       `json:"large_trader"`
	Fails       []reporting.SettlementFail           `json:"fails"`
	Alert       *reporting.SuspiciousTradingAlert    `json:"alert"`
	Compliance  *reporting.QuarterlyComplianceReport `json:"compliance"`
}

// handleFRCReports dispatches FRC report generation (POST) and listing (GET).
func (s *Server) handleFRCReports(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleGenerateFRCReport(w, r)
	case http.MethodGet:
		s.handleListFRCReports(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGenerateFRCReport generates a report, publishes it to its FRC delivery
// targets, and returns the report envelope plus the delivery receipt.
func (s *Server) handleGenerateFRCReport(w http.ResponseWriter, r *http.Request) {
	var req frcReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	rep, err := s.buildFRCReport(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	delivery, err := s.frc.Publish(r.Context(), rep)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	addAuditEvent("FRC_REPORT_GENERATED", "", "FRC report generated: "+string(rep.Type), "")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"report": rep,
		"delivery": map[string]interface{}{
			"report_id":   delivery.ReportID,
			"kafka_topic": delivery.KafkaTopic,
			"s3_key":      delivery.S3Key,
			"format":      delivery.Format,
		},
	})
}

// buildFRCReport routes the request to the matching reporter method.
func (s *Server) buildFRCReport(req frcReportRequest) (*reporting.Report, error) {
	switch req.ReportType {
	case reporting.ReportDailyTradingSummary:
		return s.frc.GenerateDailyTradingSummary(req.SessionDate, req.Instruments, req.Format)
	case reporting.ReportLargeTrader:
		if req.LargeTrader == nil {
			return nil, reporting.ErrNoData
		}
		return s.frc.GenerateLargeTraderReport(*req.LargeTrader)
	case reporting.ReportSettlementFails:
		return s.frc.GenerateSettlementFailsReport(req.SessionDate, req.Fails)
	case reporting.ReportSuspiciousTrading:
		if req.Alert == nil {
			return nil, reporting.ErrNoData
		}
		return s.frc.GenerateSuspiciousTradingAlert(*req.Alert)
	case reporting.ReportQuarterlyCompliance:
		if req.Compliance == nil {
			return nil, reporting.ErrNoData
		}
		return s.frc.GenerateQuarterlyComplianceReport(*req.Compliance)
	default:
		return nil, reporting.ErrInvalidType
	}
}

// handleListFRCReports returns the reports published so far (most recent first).
func (s *Server) handleListFRCReports(w http.ResponseWriter, r *http.Request) {
	type reportSummary struct {
		ReportID   string               `json:"report_id"`
		TenantID   string               `json:"tenant_id"`
		ReportType reporting.ReportType `json:"report_type"`
		Format     reporting.Format     `json:"format"`
		KafkaTopic string               `json:"kafka_topic"`
		S3Key      string               `json:"s3_key"`
	}
	out := []reportSummary{}
	if s.frcPublisher != nil {
		deliveries := s.frcPublisher.Deliveries()
		typeFilter := reporting.ReportType(r.URL.Query().Get("report_type"))
		// Reverse for most-recent-first ordering.
		for i := len(deliveries) - 1; i >= 0; i-- {
			d := deliveries[i]
			if typeFilter != "" && d.Type != typeFilter {
				continue
			}
			out = append(out, reportSummary{
				ReportID:   d.ReportID,
				TenantID:   d.TenantID,
				ReportType: d.Type,
				Format:     d.Format,
				KafkaTopic: d.KafkaTopic,
				S3Key:      d.S3Key,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"reports": out,
		"total":   len(out),
	})
}
