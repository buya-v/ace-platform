package service

import (
	"fmt"

	"github.com/garudax-platform/warehouse-service/internal/store"
	"github.com/garudax-platform/warehouse-service/internal/types"
)

// WarehouseService implements the business logic for the warehouse domain.
type WarehouseService struct {
	store store.DataStore
}

// New creates a new WarehouseService with the given DataStore implementation.
func New(s store.DataStore) *WarehouseService {
	return &WarehouseService{store: s}
}

// --- Facility operations ---

func (svc *WarehouseService) RegisterFacility(req RegisterFacilityRequest) (*types.Facility, error) {
	if req.FacilityCode == "" {
		return nil, fmt.Errorf("facility_code is required")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.OperatorID == "" {
		return nil, fmt.Errorf("operator_id is required")
	}
	if req.Region == "" {
		return nil, fmt.Errorf("region is required")
	}

	cap, err := types.ParseDecimal(req.TotalCapacity)
	if err != nil {
		return nil, fmt.Errorf("invalid total_capacity: %w", err)
	}
	if cap.IsZero() || cap.IsNeg() {
		return nil, fmt.Errorf("total_capacity must be positive")
	}

	unit := req.CapacityUnit
	if unit == "" {
		unit = "MT"
	}

	f := &types.Facility{
		FacilityCode:        req.FacilityCode,
		Name:                req.Name,
		OperatorID:          req.OperatorID,
		LicenseNumber:       req.LicenseNumber,
		LicenseExpiry:       req.LicenseExpiry,
		Address:             req.Address,
		Latitude:            req.Latitude,
		Longitude:           req.Longitude,
		Region:              req.Region,
		TotalCapacity:       cap,
		CapacityUnit:        unit,
		ApprovedCommodityIDs: req.ApprovedCommodityIDs,
		Status:              types.FacilityStatusActive,
	}
	if err := svc.store.CreateFacility(f); err != nil {
		return nil, err
	}
	return f, nil
}

func (svc *WarehouseService) UpdateFacility(req UpdateFacilityRequest) (*types.Facility, error) {
	if req.FacilityID == "" {
		return nil, fmt.Errorf("facility_id is required")
	}
	f, err := svc.store.GetFacility(req.FacilityID)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		f.Name = req.Name
	}
	if req.LicenseNumber != "" {
		f.LicenseNumber = req.LicenseNumber
	}
	if req.LicenseExpiry != "" {
		f.LicenseExpiry = req.LicenseExpiry
	}
	if req.Address != "" {
		f.Address = req.Address
	}
	if req.TotalCapacity != "" {
		cap, err := types.ParseDecimal(req.TotalCapacity)
		if err != nil {
			return nil, fmt.Errorf("invalid total_capacity: %w", err)
		}
		f.TotalCapacity = cap
	}
	if req.Status != "" {
		f.Status = req.Status
	}
	if req.ApprovedCommodityIDs != nil {
		f.ApprovedCommodityIDs = req.ApprovedCommodityIDs
	}
	if err := svc.store.UpdateFacility(f); err != nil {
		return nil, err
	}
	return f, nil
}

func (svc *WarehouseService) GetFacility(facilityID string) (*types.Facility, error) {
	if facilityID == "" {
		return nil, fmt.Errorf("facility_id is required")
	}
	return svc.store.GetFacility(facilityID)
}

func (svc *WarehouseService) ListFacilities(region string, status types.FacilityStatus) []*types.Facility {
	return svc.store.ListFacilities(region, status)
}

// --- Inspection operations ---

func (svc *WarehouseService) ScheduleInspection(req ScheduleInspectionRequest) (*types.Inspection, error) {
	if req.FacilityID == "" {
		return nil, fmt.Errorf("facility_id is required")
	}
	if req.CommodityID == "" {
		return nil, fmt.Errorf("commodity_id is required")
	}
	if req.LotNumber == "" {
		return nil, fmt.Errorf("lot_number is required")
	}
	if req.InspectorID == "" {
		return nil, fmt.Errorf("inspector_id is required")
	}
	if req.ScheduledDate == "" {
		return nil, fmt.Errorf("scheduled_date is required")
	}

	inspType := req.InspectionType
	if inspType == "" {
		inspType = types.InspectionTypeInitial
	}

	insp := &types.Inspection{
		FacilityID:     req.FacilityID,
		CommodityID:    req.CommodityID,
		LotNumber:      req.LotNumber,
		InspectorID:    req.InspectorID,
		InspectionType: inspType,
		Status:         types.InspectionStatusScheduled,
		ScheduledDate:  req.ScheduledDate,
	}
	if err := svc.store.CreateInspection(insp); err != nil {
		return nil, err
	}
	return insp, nil
}

func (svc *WarehouseService) RecordInspectionResult(req RecordInspectionResultRequest) (*types.Inspection, error) {
	if req.InspectionID == "" {
		return nil, fmt.Errorf("inspection_id is required")
	}

	insp, err := svc.store.GetInspection(req.InspectionID)
	if err != nil {
		return nil, err
	}
	if insp.Status != types.InspectionStatusScheduled && insp.Status != types.InspectionStatusInProgress {
		return nil, fmt.Errorf("inspection %s already completed (status: %s)", req.InspectionID, insp.Status)
	}

	gw, _ := types.ParseDecimal(req.GrossWeight)
	nw, _ := types.ParseDecimal(req.NetWeight)
	mp, _ := types.ParseDecimal(req.MoisturePct)
	fm, _ := types.ParseDecimal(req.ForeignMatterPct)
	pp, _ := types.ParseDecimal(req.ProteinPct)
	tw, _ := types.ParseDecimal(req.TestWeight)

	insp.GrossWeight = gw
	insp.NetWeight = nw
	insp.MoisturePct = mp
	insp.ForeignMatterPct = fm
	insp.ProteinPct = pp
	insp.TestWeight = tw
	insp.GradeAssigned = req.GradeAssigned
	insp.Defects = req.Defects
	insp.Notes = req.Notes
	insp.CertificateNumber = req.CertificateNumber
	insp.CompletedDate = req.CompletedDate

	if req.Passed {
		insp.Status = types.InspectionStatusPassed
	} else {
		insp.Status = types.InspectionStatusFailed
	}

	if err := svc.store.UpdateInspection(insp); err != nil {
		return nil, err
	}
	return insp, nil
}

func (svc *WarehouseService) GetInspection(inspectionID string) (*types.Inspection, error) {
	if inspectionID == "" {
		return nil, fmt.Errorf("inspection_id is required")
	}
	return svc.store.GetInspection(inspectionID)
}

// --- Receipt operations ---

func (svc *WarehouseService) IssueReceipt(req IssueReceiptRequest) (*types.Receipt, error) {
	if req.FacilityID == "" {
		return nil, fmt.Errorf("facility_id is required")
	}
	if req.HolderID == "" {
		return nil, fmt.Errorf("holder_id is required")
	}
	if req.CommodityID == "" {
		return nil, fmt.Errorf("commodity_id is required")
	}
	if req.LotNumber == "" {
		return nil, fmt.Errorf("lot_number is required")
	}
	if req.InspectionID == "" {
		return nil, fmt.Errorf("inspection_id is required (inspection-gated issuance)")
	}

	// Verify inspection exists and passed
	insp, err := svc.store.GetInspection(req.InspectionID)
	if err != nil {
		return nil, fmt.Errorf("inspection not found: %w", err)
	}
	if insp.Status != types.InspectionStatusPassed {
		return nil, fmt.Errorf("inspection %s has not passed (status: %s)", req.InspectionID, insp.Status)
	}

	qty, err := types.ParseDecimal(req.Quantity)
	if err != nil {
		return nil, fmt.Errorf("invalid quantity: %w", err)
	}
	if qty.IsZero() || qty.IsNeg() {
		return nil, fmt.Errorf("quantity must be positive")
	}

	grossQty := qty
	if req.GrossQuantity != "" {
		grossQty, err = types.ParseDecimal(req.GrossQuantity)
		if err != nil {
			return nil, fmt.Errorf("invalid gross_quantity: %w", err)
		}
	}

	unit := req.Unit
	if unit == "" {
		unit = "MT"
	}

	grade := req.Grade
	if grade == "" {
		grade = insp.GradeAssigned
	}

	r := &types.Receipt{
		FacilityID:      req.FacilityID,
		HolderID:        req.HolderID,
		CommodityID:     req.CommodityID,
		Grade:           grade,
		Quantity:        qty,
		GrossQuantity:   grossQty,
		Unit:            unit,
		LotNumber:       req.LotNumber,
		StorageLocation: req.StorageLocation,
		HarvestYear:     req.HarvestYear,
		InspectionID:    req.InspectionID,
	}

	if err := svc.store.CreateReceipt(r); err != nil {
		return nil, err
	}
	return r, nil
}

func (svc *WarehouseService) TransferReceipt(receiptID, newHolderID string) (*types.Receipt, error) {
	if receiptID == "" {
		return nil, fmt.Errorf("receipt_id is required")
	}
	if newHolderID == "" {
		return nil, fmt.Errorf("new_holder_id is required")
	}
	return svc.store.TransferReceipt(receiptID, newHolderID)
}

func (svc *WarehouseService) CancelReceipt(receiptID, reason string) (*types.Receipt, error) {
	if receiptID == "" {
		return nil, fmt.Errorf("receipt_id is required")
	}
	return svc.store.CancelReceipt(receiptID, reason)
}

func (svc *WarehouseService) GetReceipt(receiptID string) (*types.Receipt, error) {
	if receiptID == "" {
		return nil, fmt.Errorf("receipt_id is required")
	}
	return svc.store.GetReceipt(receiptID)
}

func (svc *WarehouseService) ListReceipts(holderID, facilityID, commodityID string, status types.ReceiptStatus) []*types.Receipt {
	return svc.store.ListReceipts(holderID, facilityID, commodityID, status)
}

func (svc *WarehouseService) PledgeReceipt(receiptID, clearingMemberID string) (*types.Receipt, error) {
	if receiptID == "" {
		return nil, fmt.Errorf("receipt_id is required")
	}
	if clearingMemberID == "" {
		return nil, fmt.Errorf("clearing_member_id is required")
	}
	return svc.store.PledgeReceipt(receiptID, clearingMemberID)
}

func (svc *WarehouseService) ReleaseReceipt(receiptID, clearingMemberID string) (*types.Receipt, error) {
	if receiptID == "" {
		return nil, fmt.Errorf("receipt_id is required")
	}
	if clearingMemberID == "" {
		return nil, fmt.Errorf("clearing_member_id is required")
	}
	return svc.store.ReleaseReceipt(receiptID, clearingMemberID)
}

// --- Delivery operations ---

func (svc *WarehouseService) InitiateDelivery(req InitiateDeliveryRequest) (*types.DeliveryInstruction, error) {
	if req.ReceiptID == "" {
		return nil, fmt.Errorf("receipt_id is required")
	}
	if req.ObligationID == "" {
		return nil, fmt.Errorf("obligation_id is required")
	}
	if req.BuyerID == "" {
		return nil, fmt.Errorf("buyer_id is required")
	}

	dt := req.DeliveryType
	if dt == "" {
		dt = types.DeliveryTypePhysical
	}

	d := &types.DeliveryInstruction{
		ReceiptID:     req.ReceiptID,
		ObligationID:  req.ObligationID,
		BuyerID:       req.BuyerID,
		DeliveryType:  dt,
		DestinationID: req.DestinationID,
		ScheduledDate: req.ScheduledDate,
	}
	if err := svc.store.CreateDelivery(d); err != nil {
		return nil, err
	}

	// Re-read the delivery to get the populated fields
	return svc.store.GetDelivery(d.DeliveryID)
}

func (svc *WarehouseService) CompleteDelivery(deliveryID string, success bool, failureReason string) (*types.DeliveryInstruction, error) {
	if deliveryID == "" {
		return nil, fmt.Errorf("delivery_id is required")
	}
	return svc.store.CompleteDelivery(deliveryID, success, failureReason)
}

func (svc *WarehouseService) GetDelivery(deliveryID string) (*types.DeliveryInstruction, error) {
	if deliveryID == "" {
		return nil, fmt.Errorf("delivery_id is required")
	}
	return svc.store.GetDelivery(deliveryID)
}

func (svc *WarehouseService) ListDeliveries(sellerID, buyerID string, status types.DeliveryStatus) []*types.DeliveryInstruction {
	return svc.store.ListDeliveries(sellerID, buyerID, status)
}

// --- Inventory operations ---

func (svc *WarehouseService) GetInventory(facilityID, commodityID string) ([]types.InventoryItem, types.Decimal) {
	return svc.store.GetInventory(facilityID, commodityID)
}

func (svc *WarehouseService) GetFacilityCapacity(facilityID string) (*types.FacilityCapacity, error) {
	if facilityID == "" {
		return nil, fmt.Errorf("facility_id is required")
	}
	return svc.store.GetFacilityCapacity(facilityID)
}

// --- Request types ---

type RegisterFacilityRequest struct {
	FacilityCode        string
	Name                string
	OperatorID          string
	LicenseNumber       string
	LicenseExpiry       string
	Address             string
	Latitude            string
	Longitude           string
	Region              string
	TotalCapacity       string
	CapacityUnit        string
	ApprovedCommodityIDs []string
}

type UpdateFacilityRequest struct {
	FacilityID          string
	Name                string
	LicenseNumber       string
	LicenseExpiry       string
	Address             string
	TotalCapacity       string
	Status              types.FacilityStatus
	ApprovedCommodityIDs []string
}

type ScheduleInspectionRequest struct {
	FacilityID     string
	CommodityID    string
	LotNumber      string
	InspectorID    string
	InspectionType types.InspectionType
	ScheduledDate  string
}

type RecordInspectionResultRequest struct {
	InspectionID      string
	GrossWeight       string
	NetWeight         string
	MoisturePct       string
	ForeignMatterPct  string
	ProteinPct        string
	TestWeight        string
	GradeAssigned     string
	Defects           string
	Notes             string
	CertificateNumber string
	CompletedDate     string
	Passed            bool
}

type IssueReceiptRequest struct {
	FacilityID      string
	HolderID        string
	CommodityID     string
	Grade           string
	Quantity        string
	GrossQuantity   string
	Unit            string
	LotNumber       string
	StorageLocation string
	HarvestYear     int
	InspectionID    string
}

type InitiateDeliveryRequest struct {
	ReceiptID     string
	ObligationID  string
	BuyerID       string
	DeliveryType  types.DeliveryType
	DestinationID string
	ScheduledDate string
}
