package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/compliance-service/integration"
	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/screening"
	"github.com/garudax-platform/compliance-service/internal/types"
	"github.com/garudax-platform/compliance-service/reporting"
)

// Server wraps the compliance services with health checks and HTTP endpoints.
type Server struct {
	onboarding *onboarding.Service
	screening  *screening.Service
	cfg        Config
	ready      atomic.Bool

	// frc and frcPublisher drive the FRC regulatory reporting pipeline. Both
	// are optional — routes are only registered when a reporter is wired.
	frc          *reporting.Reporter
	frcPublisher *reporting.RecordingPublisher

	// csd is the MCSD integration adapter. Optional — routes are only
	// registered when an adapter is wired.
	csd integration.CSDAdapter
}

// SetFRCReporter wires the FRC reporting pipeline. The RecordingPublisher backs
// the GET /frc/reports listing; pass the same publisher the reporter was built
// with. Passing nil for either leaves FRC reporting disabled.
func (s *Server) SetFRCReporter(r *reporting.Reporter, pub *reporting.RecordingPublisher) {
	s.frc = r
	s.frcPublisher = pub
}

// SetCSDAdapter wires the MCSD integration adapter, enabling the /csd routes.
func (s *Server) SetCSDAdapter(a integration.CSDAdapter) {
	s.csd = a
}

// AuditEvent represents a compliance audit trail event.
type AuditEvent struct {
	EventID       string `json:"event_id"`
	Timestamp     string `json:"timestamp"`
	EventType     string `json:"event_type"`
	ParticipantID string `json:"participant_id,omitempty"`
	Description   string `json:"description"`
	Officer       string `json:"officer,omitempty"`
}

var (
	auditMu     sync.Mutex
	auditEvents []AuditEvent
	auditSeq    uint64
)

func addAuditEvent(eventType, participantID, description, officer string) {
	auditMu.Lock()
	defer auditMu.Unlock()
	auditSeq++
	auditEvents = append(auditEvents, AuditEvent{
		EventID:       fmt.Sprintf("audit-%d", auditSeq),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		EventType:     eventType,
		ParticipantID: participantID,
		Description:   description,
		Officer:       officer,
	})
}

func NewServer(onboardingSvc *onboarding.Service, screeningSvc *screening.Service, cfg Config) *Server {
	return &Server{
		onboarding: onboardingSvc,
		screening:  screeningSvc,
		cfg:        cfg,
	}
}

func (s *Server) SetReady() {
	s.ready.Store(true)
}

// mountRoutes registers all HTTP route handlers onto the provided mux.
// Extracted for testability — tests can call this directly with httptest.
func (s *Server) mountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.ready.Load() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "not ready")
		}
	})

	mux.HandleFunc("/application", s.handleApplication)
	mux.HandleFunc("/applications", s.handleApplications)
	mux.HandleFunc("/participant-status", s.handleParticipantStatus)
	mux.HandleFunc("/alerts", s.handleAlerts)
	mux.HandleFunc("/audit-trail", s.handleAuditTrail)
	mux.HandleFunc("/risk-score", s.handleRiskScore)

	// FRC regulatory reporting pipeline (mse-equities flagship). Registered
	// only when a reporter is wired so existing deployments are unaffected.
	if s.frc != nil {
		mux.HandleFunc("/frc/reports", s.handleFRCReports)
	}

	// MCSD custody/settlement integration. Registered only when an adapter is wired.
	if s.csd != nil {
		mux.HandleFunc("/csd/accounts", s.handleCSDAccounts)
		mux.HandleFunc("/csd/accounts/balance", s.handleCSDBalance)
		mux.HandleFunc("/csd/transfers", s.handleCSDTransfers)
		mux.HandleFunc("/csd/transfers/dvp", s.handleCSDInstructDvP)
		mux.HandleFunc("/csd/transfers/fop", s.handleCSDInstructFoP)
		mux.HandleFunc("/csd/corporate-actions", s.handleCSDCorporateActions)
	}
}

// handleApplication handles single-application creation (POST) and retrieval (GET).
// POST /application — submit a new KYC application; returns a JSON object.
// GET /application?id=<app_id> — retrieve an application by ID.
func (s *Server) handleApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			ParticipantID   string `json:"participant_id"`
			ParticipantType string `json:"participant_type"`
			LegalName       string `json:"legal_name"`
			TradingName     string `json:"trading_name"`
			Nationality     string `json:"nationality"`
			SourceOfFunds   string `json:"source_of_funds"`
			Contact         struct {
				Email             string `json:"email"`
				Phone             string `json:"phone"`
				ContactPersonName string `json:"contact_person_name"`
			} `json:"contact"`
			RegisteredAddress struct {
				Line1      string `json:"line1"`
				Line2      string `json:"line2"`
				City       string `json:"city"`
				Province   string `json:"province"`
				PostalCode string `json:"postal_code"`
				Country    string `json:"country"`
			} `json:"registered_address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		pt := types.ParticipantType(req.ParticipantType)
		if pt == "" {
			pt = types.ParticipantIndividual
		}
		contact := types.ContactInfo{
			Email:             req.Contact.Email,
			Phone:             req.Contact.Phone,
			ContactPersonName: req.Contact.ContactPersonName,
		}
		addr := types.Address{
			Line1:      req.RegisteredAddress.Line1,
			Line2:      req.RegisteredAddress.Line2,
			City:       req.RegisteredAddress.City,
			Province:   req.RegisteredAddress.Province,
			PostalCode: req.RegisteredAddress.PostalCode,
			Country:    req.RegisteredAddress.Country,
		}
		app, err := s.onboarding.SubmitApplication(req.ParticipantID, pt, req.LegalName, req.TradingName, req.Nationality, contact, addr, req.SourceOfFunds)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		addAuditEvent("APPLICATION_SUBMITTED", req.ParticipantID, "KYC application submitted: "+req.LegalName, "")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(app)
		return
	}
	// GET
	appID := r.URL.Query().Get("id")
	if appID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	app, err := s.onboarding.GetApplication(appID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app)
}

// handleApplications handles list queries for KYC applications.
// GET /applications — returns {"applications":[...], "total":N}.
func (s *Server) handleApplications(w http.ResponseWriter, r *http.Request) {
	statusFilter := types.KYCStatus(r.URL.Query().Get("status"))
	typeFilter := types.ParticipantType(r.URL.Query().Get("type"))
	apps, err := s.onboarding.ListApplications(statusFilter, typeFilter, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Ensure we return an empty array (not null) when there are no applications.
	if apps == nil {
		apps = []*types.KYCApplication{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"applications": apps,
		"total":        len(apps),
	})
}

// handleParticipantStatus handles participant clearance status checks.
func (s *Server) handleParticipantStatus(w http.ResponseWriter, r *http.Request) {
	participantID := r.URL.Query().Get("participant_id")
	if participantID == "" {
		http.Error(w, "participant_id required", http.StatusBadRequest)
		return
	}
	status, err := s.onboarding.CheckParticipantStatus(participantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleAlerts handles alert creation (POST) and listing (GET).
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			ParticipantID string `json:"participant_id"`
			RuleID        string `json:"rule_id"`
			Description   string `json:"description"`
			Details       string `json:"details"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		alert, err := s.screening.CreateAlert(req.ParticipantID, req.RuleID, req.Description, req.Details)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		addAuditEvent("ALERT_CREATED", req.ParticipantID, "Alert created: "+req.Description, "")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(alert)
		return
	}
	// GET — list alerts (participant_id is optional)
	statusFilter := types.AlertStatus(r.URL.Query().Get("status"))
	participantID := r.URL.Query().Get("participant_id")
	alerts, err := s.screening.ListAlerts(statusFilter, participantID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if alerts == nil {
		alerts = []*types.MonitoringAlert{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"total":  len(alerts),
	})
}

// handleAuditTrail returns compliance audit events.
func (s *Server) handleAuditTrail(w http.ResponseWriter, r *http.Request) {
	participantID := r.URL.Query().Get("participant_id")
	auditMu.Lock()
	var filtered []AuditEvent
	for _, e := range auditEvents {
		if participantID == "" || e.ParticipantID == participantID {
			filtered = append(filtered, e)
		}
	}
	auditMu.Unlock()
	if filtered == nil {
		filtered = []AuditEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": filtered,
		"total":  len(filtered),
	})
}

// handleRiskScore returns a risk score for a participant.
func (s *Server) handleRiskScore(w http.ResponseWriter, r *http.Request) {
	participantID := r.URL.Query().Get("participant_id")
	if participantID == "" {
		http.Error(w, "participant_id required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"participant_id": participantID,
		"risk_score":     50,
		"risk_level":     "MEDIUM",
	})
}

// StartHealthServer starts HTTP health, readiness, and query endpoints.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()
	s.mountRoutes(mux)

	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.HealthPort)
	return http.ListenAndServe(addr, mux)
}

// ListenGRPC creates a TCP listener for the gRPC port.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.GRPCPort)
	return net.Listen("tcp", addr)
}
