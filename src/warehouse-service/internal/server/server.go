package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/ace-platform/warehouse-service/internal/service"
	"github.com/ace-platform/warehouse-service/internal/types"
)

// Server is the warehouse service gRPC server.
type Server struct {
	svc    *service.WarehouseService
	config Config
	ready  int32 // atomic: 1 = ready
}

// NewServer creates a new warehouse server.
func NewServer(svc *service.WarehouseService, cfg Config) *Server {
	return &Server{
		svc:    svc,
		config: cfg,
	}
}

// Service returns the warehouse service for handler registration.
func (s *Server) Service() *service.WarehouseService {
	return s.svc
}

// SetReady marks the server as ready to serve traffic.
func (s *Server) SetReady() {
	atomic.StoreInt32(&s.ready, 1)
}

// IsReady returns true if the server is ready.
func (s *Server) IsReady() bool {
	return atomic.LoadInt32(&s.ready) == 1
}

// StartHealthServer starts the HTTP health check and data query server for Kubernetes probes.
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

	// --- Data query endpoints ---

	mux.HandleFunc("/receipts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Issue a new receipt — use tagged struct for snake_case JSON
			var body struct {
				FacilityID      string `json:"facility_id"`
				HolderID        string `json:"holder_id"`
				CommodityID     string `json:"commodity_id"`
				Grade           string `json:"grade"`
				Quantity        string `json:"quantity"`
				GrossQuantity   string `json:"gross_quantity"`
				Unit            string `json:"unit"`
				LotNumber       string `json:"lot_number"`
				StorageLocation string `json:"storage_location"`
				HarvestYear     int    `json:"harvest_year"`
				InspectionID    string `json:"inspection_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			req := service.IssueReceiptRequest{
				FacilityID:      body.FacilityID,
				HolderID:        body.HolderID,
				CommodityID:     body.CommodityID,
				Grade:           body.Grade,
				Quantity:        body.Quantity,
				GrossQuantity:   body.GrossQuantity,
				Unit:            body.Unit,
				LotNumber:       body.LotNumber,
				StorageLocation: body.StorageLocation,
				HarvestYear:     body.HarvestYear,
				InspectionID:    body.InspectionID,
			}
			receipt, err := s.svc.IssueReceipt(req)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(receipt)
			return
		}
		// GET — list receipts
		holderID := r.URL.Query().Get("holder_id")
		facilityID := r.URL.Query().Get("facility_id")
		commodityID := r.URL.Query().Get("commodity_id")
		status := types.ReceiptStatus(r.URL.Query().Get("status"))
		receipts := s.svc.ListReceipts(holderID, facilityID, commodityID, status)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"receipts": receipts,
			"count":    len(receipts),
		})
	})

	mux.HandleFunc("/receipts/pledge", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ReceiptID       string `json:"receipt_id"`
			ClearingMemberID string `json:"clearing_member_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		receipt, err := s.svc.PledgeReceipt(req.ReceiptID, req.ClearingMemberID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(receipt)
	})

	mux.HandleFunc("/deliveries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				ReceiptID     string `json:"receipt_id"`
				ObligationID  string `json:"obligation_id"`
				BuyerID       string `json:"buyer_id"`
				DeliveryType  string `json:"delivery_type"`
				DestinationID string `json:"destination_id"`
				ScheduledDate string `json:"scheduled_date"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			req := service.InitiateDeliveryRequest{
				ReceiptID:     body.ReceiptID,
				ObligationID:  body.ObligationID,
				BuyerID:       body.BuyerID,
				DeliveryType:  types.DeliveryType(body.DeliveryType),
				DestinationID: body.DestinationID,
				ScheduledDate: body.ScheduledDate,
			}
			delivery, err := s.svc.InitiateDelivery(req)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(delivery)
			return
		}
		// GET — list deliveries
		sellerID := r.URL.Query().Get("seller_id")
		buyerID := r.URL.Query().Get("buyer_id")
		status := types.DeliveryStatus(r.URL.Query().Get("status"))
		deliveries := s.svc.ListDeliveries(sellerID, buyerID, status)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deliveries": deliveries,
			"count":      len(deliveries),
		})
	})

	mux.HandleFunc("/inventory", func(w http.ResponseWriter, r *http.Request) {
		facilityID := r.URL.Query().Get("facility_id")
		commodityID := r.URL.Query().Get("commodity_id")
		items, total := s.svc.GetInventory(facilityID, commodityID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       items,
			"count":       len(items),
			"total_quantity": total.String(),
		})
	})

	mux.HandleFunc("/facilities", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				FacilityCode         string   `json:"facility_code"`
				Name                 string   `json:"name"`
				OperatorID           string   `json:"operator_id"`
				LicenseNumber        string   `json:"license_number"`
				LicenseExpiry        string   `json:"license_expiry"`
				Address              string   `json:"address"`
				Latitude             string   `json:"latitude"`
				Longitude            string   `json:"longitude"`
				Region               string   `json:"region"`
				TotalCapacity        string   `json:"total_capacity"`
				CapacityUnit         string   `json:"capacity_unit"`
				ApprovedCommodityIDs []string `json:"approved_commodity_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			req := service.RegisterFacilityRequest{
				FacilityCode:         body.FacilityCode,
				Name:                 body.Name,
				OperatorID:           body.OperatorID,
				LicenseNumber:        body.LicenseNumber,
				LicenseExpiry:        body.LicenseExpiry,
				Address:              body.Address,
				Latitude:             body.Latitude,
				Longitude:            body.Longitude,
				Region:               body.Region,
				TotalCapacity:        body.TotalCapacity,
				CapacityUnit:         body.CapacityUnit,
				ApprovedCommodityIDs: body.ApprovedCommodityIDs,
			}
			facility, err := s.svc.RegisterFacility(req)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(facility)
			return
		}
		// GET — list facilities
		region := r.URL.Query().Get("region")
		status := types.FacilityStatus(r.URL.Query().Get("status"))
		facilities := s.svc.ListFacilities(region, status)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"facilities": facilities,
			"count":      len(facilities),
		})
	})

	mux.HandleFunc("/inspections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				FacilityID     string `json:"facility_id"`
				CommodityID    string `json:"commodity_id"`
				LotNumber      string `json:"lot_number"`
				InspectorID    string `json:"inspector_id"`
				InspectionType string `json:"inspection_type"`
				ScheduledDate  string `json:"scheduled_date"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			req := service.ScheduleInspectionRequest{
				FacilityID:     body.FacilityID,
				CommodityID:    body.CommodityID,
				LotNumber:      body.LotNumber,
				InspectorID:    body.InspectorID,
				InspectionType: types.InspectionType(body.InspectionType),
				ScheduledDate:  body.ScheduledDate,
			}
			inspection, err := s.svc.ScheduleInspection(req)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(inspection)
			return
		}
		// GET — get by ID
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		insp, err := s.svc.GetInspection(id)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(insp)
	})

	mux.HandleFunc("/inspections/result", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InspectionID      string `json:"inspection_id"`
			GrossWeight       string `json:"gross_weight"`
			NetWeight         string `json:"net_weight"`
			MoisturePct       string `json:"moisture_pct"`
			ForeignMatterPct  string `json:"foreign_matter_pct"`
			ProteinPct        string `json:"protein_pct"`
			TestWeight        string `json:"test_weight"`
			GradeAssigned     string `json:"grade_assigned"`
			Defects           string `json:"defects"`
			Notes             string `json:"notes"`
			CertificateNumber string `json:"certificate_number"`
			CompletedDate     string `json:"completed_date"`
			Passed            bool   `json:"passed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		req := service.RecordInspectionResultRequest{
			InspectionID:      body.InspectionID,
			GrossWeight:       body.GrossWeight,
			NetWeight:         body.NetWeight,
			MoisturePct:       body.MoisturePct,
			ForeignMatterPct:  body.ForeignMatterPct,
			ProteinPct:        body.ProteinPct,
			TestWeight:        body.TestWeight,
			GradeAssigned:     body.GradeAssigned,
			Defects:           body.Defects,
			Notes:             body.Notes,
			CertificateNumber: body.CertificateNumber,
			CompletedDate:     body.CompletedDate,
			Passed:            body.Passed,
		}
		insp, err := s.svc.RecordInspectionResult(req)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(insp)
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
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := s.GRPCAddr()
	log.Printf("gRPC server listening on %s", addr)
	return net.Listen("tcp", addr)
}
