package service

import (
	"strings"
	"testing"

	"github.com/garudax-platform/warehouse-service/internal/store"
	"github.com/garudax-platform/warehouse-service/internal/types"
)

// helper creates a service with a fresh store and a pre-registered active facility.
func setupTestService(t *testing.T) (*WarehouseService, *types.Facility) {
	t.Helper()
	st := store.NewStore()
	svc := New(st)

	fac, err := svc.RegisterFacility(RegisterFacilityRequest{
		FacilityCode:  "WH-TEST-01",
		Name:          "Test Warehouse",
		OperatorID:    "op-1",
		LicenseNumber: "LIC-001",
		LicenseExpiry: "2027-12-31",
		Address:       "123 Test St",
		Region:        "East Africa",
		TotalCapacity: "10000",
		CapacityUnit:  "MT",
		ApprovedCommodityIDs: []string{"corn-id", "wheat-id"},
	})
	if err != nil {
		t.Fatalf("setup: register facility: %v", err)
	}
	return svc, fac
}

// helper creates a passed inspection for receipt issuance.
func createPassedInspection(t *testing.T, svc *WarehouseService, facilityID, commodityID, lotNumber string) *types.Inspection {
	t.Helper()
	insp, err := svc.ScheduleInspection(ScheduleInspectionRequest{
		FacilityID:    facilityID,
		CommodityID:   commodityID,
		LotNumber:     lotNumber,
		InspectorID:   "inspector-1",
		ScheduledDate: "2026-03-28",
	})
	if err != nil {
		t.Fatalf("setup: schedule inspection: %v", err)
	}
	insp, err = svc.RecordInspectionResult(RecordInspectionResultRequest{
		InspectionID:  insp.InspectionID,
		GrossWeight:   "1050",
		NetWeight:     "1000",
		MoisturePct:   "12.5",
		GradeAssigned: "Grade-1",
		Passed:        true,
		CompletedDate: "2026-03-28",
	})
	if err != nil {
		t.Fatalf("setup: record inspection result: %v", err)
	}
	return insp
}

// issueTestReceipt is a convenience helper that creates inspection + receipt.
func issueTestReceipt(t *testing.T, svc *WarehouseService, facilityID, holderID, commodityID, lotNumber, qty string) *types.Receipt {
	t.Helper()
	insp := createPassedInspection(t, svc, facilityID, commodityID, lotNumber)
	r, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID:  facilityID,
		HolderID:    holderID,
		CommodityID: commodityID,
		Quantity:    qty,
		LotNumber:   lotNumber,
		HarvestYear: 2026,
		InspectionID: insp.InspectionID,
	})
	if err != nil {
		t.Fatalf("setup: issue receipt: %v", err)
	}
	return r
}

// ─── Facility Tests ──────────────────────────────────────────────────────────

func TestRegisterFacility(t *testing.T) {
	st := store.NewStore()
	svc := New(st)

	f, err := svc.RegisterFacility(RegisterFacilityRequest{
		FacilityCode:  "WH-01",
		Name:          "Main Warehouse",
		OperatorID:    "op-1",
		Region:        "East Africa",
		TotalCapacity: "5000",
	})
	if err != nil {
		t.Fatalf("RegisterFacility: %v", err)
	}
	if f.FacilityID == "" {
		t.Fatal("expected facility ID to be assigned")
	}
	if f.Status != types.FacilityStatusActive {
		t.Errorf("expected ACTIVE status, got %s", f.Status)
	}
	if f.CapacityUnit != "MT" {
		t.Errorf("expected default capacity unit MT, got %s", f.CapacityUnit)
	}
}

func TestRegisterFacilityDuplicateCode(t *testing.T) {
	st := store.NewStore()
	svc := New(st)

	_, err := svc.RegisterFacility(RegisterFacilityRequest{
		FacilityCode: "WH-01", Name: "First", OperatorID: "op-1",
		Region: "East Africa", TotalCapacity: "5000",
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err = svc.RegisterFacility(RegisterFacilityRequest{
		FacilityCode: "WH-01", Name: "Second", OperatorID: "op-2",
		Region: "East Africa", TotalCapacity: "3000",
	})
	if err == nil {
		t.Fatal("expected error for duplicate facility code")
	}
}

func TestRegisterFacilityValidation(t *testing.T) {
	st := store.NewStore()
	svc := New(st)

	tests := []struct {
		name string
		req  RegisterFacilityRequest
		want string
	}{
		{"missing code", RegisterFacilityRequest{Name: "X", OperatorID: "o", Region: "R", TotalCapacity: "1"}, "facility_code"},
		{"missing name", RegisterFacilityRequest{FacilityCode: "C", OperatorID: "o", Region: "R", TotalCapacity: "1"}, "name"},
		{"missing operator", RegisterFacilityRequest{FacilityCode: "C", Name: "X", Region: "R", TotalCapacity: "1"}, "operator_id"},
		{"missing region", RegisterFacilityRequest{FacilityCode: "C", Name: "X", OperatorID: "o", TotalCapacity: "1"}, "region"},
		{"zero capacity", RegisterFacilityRequest{FacilityCode: "C", Name: "X", OperatorID: "o", Region: "R", TotalCapacity: "0"}, "positive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RegisterFacility(tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestUpdateFacility(t *testing.T) {
	svc, fac := setupTestService(t)

	updated, err := svc.UpdateFacility(UpdateFacilityRequest{
		FacilityID: fac.FacilityID,
		Name:       "Updated Warehouse",
		Status:     types.FacilityStatusSuspended,
	})
	if err != nil {
		t.Fatalf("UpdateFacility: %v", err)
	}
	if updated.Name != "Updated Warehouse" {
		t.Errorf("expected updated name, got %s", updated.Name)
	}
	if updated.Status != types.FacilityStatusSuspended {
		t.Errorf("expected SUSPENDED, got %s", updated.Status)
	}
}

func TestListFacilities(t *testing.T) {
	st := store.NewStore()
	svc := New(st)

	svc.RegisterFacility(RegisterFacilityRequest{
		FacilityCode: "WH-01", Name: "East", OperatorID: "op-1",
		Region: "East", TotalCapacity: "5000",
	})
	svc.RegisterFacility(RegisterFacilityRequest{
		FacilityCode: "WH-02", Name: "West", OperatorID: "op-2",
		Region: "West", TotalCapacity: "3000",
	})

	all := svc.ListFacilities("", "")
	if len(all) != 2 {
		t.Errorf("expected 2 facilities, got %d", len(all))
	}

	east := svc.ListFacilities("East", "")
	if len(east) != 1 {
		t.Errorf("expected 1 east facility, got %d", len(east))
	}
}

// ─── Inspection Tests ────────────────────────────────────────────────────────

func TestScheduleInspection(t *testing.T) {
	svc, fac := setupTestService(t)

	insp, err := svc.ScheduleInspection(ScheduleInspectionRequest{
		FacilityID:    fac.FacilityID,
		CommodityID:   "corn-id",
		LotNumber:     "LOT-001",
		InspectorID:   "inspector-1",
		ScheduledDate: "2026-04-01",
	})
	if err != nil {
		t.Fatalf("ScheduleInspection: %v", err)
	}
	if insp.Status != types.InspectionStatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", insp.Status)
	}
	if insp.InspectionType != types.InspectionTypeInitial {
		t.Errorf("expected INITIAL default, got %s", insp.InspectionType)
	}
}

func TestRecordInspectionResult(t *testing.T) {
	svc, fac := setupTestService(t)

	insp, _ := svc.ScheduleInspection(ScheduleInspectionRequest{
		FacilityID: fac.FacilityID, CommodityID: "corn-id",
		LotNumber: "LOT-001", InspectorID: "inspector-1", ScheduledDate: "2026-04-01",
	})

	result, err := svc.RecordInspectionResult(RecordInspectionResultRequest{
		InspectionID:  insp.InspectionID,
		GrossWeight:   "1050",
		NetWeight:     "1000",
		MoisturePct:   "12.5",
		GradeAssigned: "Grade-1",
		Passed:        true,
	})
	if err != nil {
		t.Fatalf("RecordInspectionResult: %v", err)
	}
	if result.Status != types.InspectionStatusPassed {
		t.Errorf("expected PASSED, got %s", result.Status)
	}
	if result.GradeAssigned != "Grade-1" {
		t.Errorf("expected Grade-1, got %s", result.GradeAssigned)
	}
}

func TestRecordInspectionFailed(t *testing.T) {
	svc, fac := setupTestService(t)

	insp, _ := svc.ScheduleInspection(ScheduleInspectionRequest{
		FacilityID: fac.FacilityID, CommodityID: "corn-id",
		LotNumber: "LOT-001", InspectorID: "inspector-1", ScheduledDate: "2026-04-01",
	})

	result, err := svc.RecordInspectionResult(RecordInspectionResultRequest{
		InspectionID: insp.InspectionID, Passed: false,
	})
	if err != nil {
		t.Fatalf("RecordInspectionResult: %v", err)
	}
	if result.Status != types.InspectionStatusFailed {
		t.Errorf("expected FAILED, got %s", result.Status)
	}
}

func TestRecordInspectionAlreadyCompleted(t *testing.T) {
	svc, fac := setupTestService(t)

	insp, _ := svc.ScheduleInspection(ScheduleInspectionRequest{
		FacilityID: fac.FacilityID, CommodityID: "corn-id",
		LotNumber: "LOT-001", InspectorID: "inspector-1", ScheduledDate: "2026-04-01",
	})
	svc.RecordInspectionResult(RecordInspectionResultRequest{
		InspectionID: insp.InspectionID, Passed: true,
	})

	_, err := svc.RecordInspectionResult(RecordInspectionResultRequest{
		InspectionID: insp.InspectionID, Passed: true,
	})
	if err == nil {
		t.Fatal("expected error for already completed inspection")
	}
}

// ─── Receipt Lifecycle Tests ─────────────────────────────────────────────────

func TestIssueReceipt(t *testing.T) {
	svc, fac := setupTestService(t)
	insp := createPassedInspection(t, svc, fac.FacilityID, "corn-id", "LOT-001")

	r, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID:      fac.FacilityID,
		HolderID:        "holder-1",
		CommodityID:     "corn-id",
		Quantity:        "500",
		LotNumber:       "LOT-001",
		HarvestYear:     2026,
		InspectionID:    insp.InspectionID,
		StorageLocation: "Bin-A",
	})
	if err != nil {
		t.Fatalf("IssueReceipt: %v", err)
	}
	if r.Status != types.ReceiptStatusActive {
		t.Errorf("expected ACTIVE, got %s", r.Status)
	}
	if r.Grade != "Grade-1" {
		t.Errorf("expected grade from inspection (Grade-1), got %s", r.Grade)
	}
	if r.Unit != "MT" {
		t.Errorf("expected default unit MT, got %s", r.Unit)
	}
	if !strings.HasPrefix(r.ReceiptNumber, "EWR-") {
		t.Errorf("expected receipt number starting with EWR-, got %s", r.ReceiptNumber)
	}
}

func TestIssueReceiptRequiresPassedInspection(t *testing.T) {
	svc, fac := setupTestService(t)

	// Schedule but don't complete
	insp, _ := svc.ScheduleInspection(ScheduleInspectionRequest{
		FacilityID: fac.FacilityID, CommodityID: "corn-id",
		LotNumber: "LOT-001", InspectorID: "inspector-1", ScheduledDate: "2026-04-01",
	})

	_, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID: fac.FacilityID, HolderID: "holder-1", CommodityID: "corn-id",
		Quantity: "100", LotNumber: "LOT-001", HarvestYear: 2026,
		InspectionID: insp.InspectionID,
	})
	if err == nil || !strings.Contains(err.Error(), "not passed") {
		t.Fatalf("expected inspection not passed error, got %v", err)
	}
}

func TestIssueReceiptRequiresInspectionID(t *testing.T) {
	svc, fac := setupTestService(t)

	_, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID: fac.FacilityID, HolderID: "holder-1", CommodityID: "corn-id",
		Quantity: "100", LotNumber: "LOT-001", HarvestYear: 2026,
	})
	if err == nil || !strings.Contains(err.Error(), "inspection_id") {
		t.Fatalf("expected inspection_id required error, got %v", err)
	}
}

func TestIssueReceiptLotUniqueness(t *testing.T) {
	svc, fac := setupTestService(t)
	issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "100")

	// Second receipt for same lot should fail
	insp := createPassedInspection(t, svc, fac.FacilityID, "corn-id", "LOT-001-dup")
	_, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID: fac.FacilityID, HolderID: "holder-2", CommodityID: "corn-id",
		Quantity: "100", LotNumber: "LOT-001", HarvestYear: 2026,
		InspectionID: insp.InspectionID,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected lot uniqueness error, got %v", err)
	}
}

func TestIssueReceiptCapacityCheck(t *testing.T) {
	svc, fac := setupTestService(t)
	// Facility has 10000 MT capacity. Issue a receipt for 9500.
	issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-BIG", "9500")

	// Try to issue another for 600 - should exceed capacity
	insp := createPassedInspection(t, svc, fac.FacilityID, "wheat-id", "LOT-OVER")
	_, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID: fac.FacilityID, HolderID: "holder-2", CommodityID: "wheat-id",
		Quantity: "600", LotNumber: "LOT-OVER", HarvestYear: 2026,
		InspectionID: insp.InspectionID,
	})
	if err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("expected capacity error, got %v", err)
	}
}

func TestTransferReceipt(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	transferred, err := svc.TransferReceipt(r.ReceiptID, "holder-2")
	if err != nil {
		t.Fatalf("TransferReceipt: %v", err)
	}
	if transferred.HolderID != "holder-2" {
		t.Errorf("expected holder-2, got %s", transferred.HolderID)
	}
	if transferred.Status != types.ReceiptStatusActive {
		t.Errorf("expected ACTIVE after transfer, got %s", transferred.Status)
	}
}

func TestTransferReceiptInvalidStatus(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	// Cancel the receipt first
	svc.CancelReceipt(r.ReceiptID, "test")

	_, err := svc.TransferReceipt(r.ReceiptID, "holder-2")
	if err == nil || !strings.Contains(err.Error(), "cannot be transferred") {
		t.Fatalf("expected error for cancelled receipt transfer, got %v", err)
	}
}

func TestCancelReceipt(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	cancelled, err := svc.CancelReceipt(r.ReceiptID, "damaged goods")
	if err != nil {
		t.Fatalf("CancelReceipt: %v", err)
	}
	if cancelled.Status != types.ReceiptStatusCancelled {
		t.Errorf("expected CANCELLED, got %s", cancelled.Status)
	}

	// After cancel, the same lot should be available again
	insp := createPassedInspection(t, svc, fac.FacilityID, "corn-id", "LOT-001-new")
	_, err = svc.IssueReceipt(IssueReceiptRequest{
		FacilityID: fac.FacilityID, HolderID: "holder-1", CommodityID: "corn-id",
		Quantity: "500", LotNumber: "LOT-001", HarvestYear: 2026,
		InspectionID: insp.InspectionID,
	})
	if err != nil {
		t.Fatalf("expected to re-issue after cancel, got %v", err)
	}
}

func TestGetAndListReceipts(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	got, err := svc.GetReceipt(r.ReceiptID)
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if got.ReceiptID != r.ReceiptID {
		t.Errorf("expected %s, got %s", r.ReceiptID, got.ReceiptID)
	}

	list := svc.ListReceipts("holder-1", "", "", "")
	if len(list) != 1 {
		t.Errorf("expected 1 receipt for holder-1, got %d", len(list))
	}

	list = svc.ListReceipts("holder-2", "", "", "")
	if len(list) != 0 {
		t.Errorf("expected 0 receipts for holder-2, got %d", len(list))
	}
}

// ─── Collateralization Tests ─────────────────────────────────────────────────

func TestPledgeAndReleaseReceipt(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	// Pledge
	pledged, err := svc.PledgeReceipt(r.ReceiptID, "clearing-member-1")
	if err != nil {
		t.Fatalf("PledgeReceipt: %v", err)
	}
	if pledged.Status != types.ReceiptStatusPledged {
		t.Errorf("expected PLEDGED, got %s", pledged.Status)
	}
	if pledged.PledgedTo != "clearing-member-1" {
		t.Errorf("expected pledged_to clearing-member-1, got %s", pledged.PledgedTo)
	}

	// Cannot transfer while pledged
	_, err = svc.TransferReceipt(r.ReceiptID, "holder-2")
	if err == nil {
		t.Fatal("expected error transferring pledged receipt")
	}

	// Cannot cancel while pledged
	_, err = svc.CancelReceipt(r.ReceiptID, "test")
	if err == nil {
		t.Fatal("expected error cancelling pledged receipt")
	}

	// Release
	released, err := svc.ReleaseReceipt(r.ReceiptID, "clearing-member-1")
	if err != nil {
		t.Fatalf("ReleaseReceipt: %v", err)
	}
	if released.Status != types.ReceiptStatusActive {
		t.Errorf("expected ACTIVE after release, got %s", released.Status)
	}
	if released.PledgedTo != "" {
		t.Errorf("expected empty pledged_to after release, got %s", released.PledgedTo)
	}
}

func TestReleaseWrongMember(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	svc.PledgeReceipt(r.ReceiptID, "clearing-member-1")

	_, err := svc.ReleaseReceipt(r.ReceiptID, "clearing-member-2")
	if err == nil || !strings.Contains(err.Error(), "not clearing-member-2") {
		t.Fatalf("expected wrong member error, got %v", err)
	}
}

func TestPledgeNonActiveReceipt(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	svc.CancelReceipt(r.ReceiptID, "test")

	_, err := svc.PledgeReceipt(r.ReceiptID, "clearing-member-1")
	if err == nil || !strings.Contains(err.Error(), "cannot be pledged") {
		t.Fatalf("expected error pledging cancelled receipt, got %v", err)
	}
}

// ─── Delivery Workflow Tests ─────────────────────────────────────────────────

func TestDeliveryWorkflow_Success(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	// Initiate delivery
	d, err := svc.InitiateDelivery(InitiateDeliveryRequest{
		ReceiptID:     r.ReceiptID,
		ObligationID:  "obligation-1",
		BuyerID:       "buyer-1",
		DeliveryType:  types.DeliveryTypePhysical,
		ScheduledDate: "2026-04-15",
	})
	if err != nil {
		t.Fatalf("InitiateDelivery: %v", err)
	}
	if d.Status != types.DeliveryStatusPending {
		t.Errorf("expected PENDING, got %s", d.Status)
	}
	if d.SellerID != "holder-1" {
		t.Errorf("expected seller holder-1, got %s", d.SellerID)
	}

	// Receipt should be DELIVERY_PENDING
	receipt, _ := svc.GetReceipt(r.ReceiptID)
	if receipt.Status != types.ReceiptStatusDeliveryPending {
		t.Errorf("expected DELIVERY_PENDING, got %s", receipt.Status)
	}

	// Complete delivery
	completed, err := svc.CompleteDelivery(d.DeliveryID, true, "")
	if err != nil {
		t.Fatalf("CompleteDelivery: %v", err)
	}
	if completed.Status != types.DeliveryStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", completed.Status)
	}

	// Receipt should be DELIVERED
	receipt, _ = svc.GetReceipt(r.ReceiptID)
	if receipt.Status != types.ReceiptStatusDelivered {
		t.Errorf("expected DELIVERED, got %s", receipt.Status)
	}
}

func TestDeliveryWorkflow_Failure(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	d, _ := svc.InitiateDelivery(InitiateDeliveryRequest{
		ReceiptID: r.ReceiptID, ObligationID: "obligation-1",
		BuyerID: "buyer-1", ScheduledDate: "2026-04-15",
	})

	// Fail delivery
	failed, err := svc.CompleteDelivery(d.DeliveryID, false, "truck broke down")
	if err != nil {
		t.Fatalf("CompleteDelivery(fail): %v", err)
	}
	if failed.Status != types.DeliveryStatusFailed {
		t.Errorf("expected FAILED, got %s", failed.Status)
	}
	if failed.FailureReason != "truck broke down" {
		t.Errorf("expected failure reason, got %s", failed.FailureReason)
	}

	// Receipt should revert to ACTIVE
	receipt, _ := svc.GetReceipt(r.ReceiptID)
	if receipt.Status != types.ReceiptStatusActive {
		t.Errorf("expected ACTIVE after failed delivery, got %s", receipt.Status)
	}
}

func TestDeliveryCannotDeliverCancelledReceipt(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	svc.CancelReceipt(r.ReceiptID, "test")

	_, err := svc.InitiateDelivery(InitiateDeliveryRequest{
		ReceiptID: r.ReceiptID, ObligationID: "obligation-1",
		BuyerID: "buyer-1",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be delivered") {
		t.Fatalf("expected error initiating delivery for cancelled receipt, got %v", err)
	}
}

func TestListDeliveries(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	svc.InitiateDelivery(InitiateDeliveryRequest{
		ReceiptID: r.ReceiptID, ObligationID: "ob-1",
		BuyerID: "buyer-1",
	})

	list := svc.ListDeliveries("holder-1", "", "")
	if len(list) != 1 {
		t.Errorf("expected 1 delivery for seller, got %d", len(list))
	}

	list = svc.ListDeliveries("", "buyer-1", "")
	if len(list) != 1 {
		t.Errorf("expected 1 delivery for buyer, got %d", len(list))
	}

	list = svc.ListDeliveries("", "", types.DeliveryStatusCompleted)
	if len(list) != 0 {
		t.Errorf("expected 0 completed deliveries, got %d", len(list))
	}
}

// ─── Inventory Tests ─────────────────────────────────────────────────────────

func TestInventoryTracking(t *testing.T) {
	svc, fac := setupTestService(t)
	issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")
	issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-002", "300")

	items, total := svc.GetInventory(fac.FacilityID, "corn-id")
	if len(items) != 2 {
		t.Errorf("expected 2 inventory items, got %d", len(items))
	}

	expectedTotal, _ := types.ParseDecimal("800")
	if total.Raw() != expectedTotal.Raw() {
		t.Errorf("expected total 800, got %s", total.String())
	}
}

func TestInventoryAfterCancel(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	svc.CancelReceipt(r.ReceiptID, "test")

	items, total := svc.GetInventory(fac.FacilityID, "corn-id")
	if len(items) != 0 {
		t.Errorf("expected 0 items after cancel, got %d", len(items))
	}
	if !total.IsZero() {
		t.Errorf("expected zero total after cancel, got %s", total.String())
	}
}

func TestInventoryAfterDelivery(t *testing.T) {
	svc, fac := setupTestService(t)
	r := issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "500")

	d, _ := svc.InitiateDelivery(InitiateDeliveryRequest{
		ReceiptID: r.ReceiptID, ObligationID: "ob-1", BuyerID: "buyer-1",
	})
	svc.CompleteDelivery(d.DeliveryID, true, "")

	items, total := svc.GetInventory(fac.FacilityID, "corn-id")
	if len(items) != 0 {
		t.Errorf("expected 0 items after delivery, got %d", len(items))
	}
	if !total.IsZero() {
		t.Errorf("expected zero total after delivery, got %s", total.String())
	}
}

func TestFacilityCapacity(t *testing.T) {
	svc, fac := setupTestService(t)
	issueTestReceipt(t, svc, fac.FacilityID, "holder-1", "corn-id", "LOT-001", "3000")

	cap, err := svc.GetFacilityCapacity(fac.FacilityID)
	if err != nil {
		t.Fatalf("GetFacilityCapacity: %v", err)
	}

	expectedTotal, _ := types.ParseDecimal("10000")
	expectedUsed, _ := types.ParseDecimal("3000")
	expectedAvail, _ := types.ParseDecimal("7000")

	if cap.TotalCapacity.Raw() != expectedTotal.Raw() {
		t.Errorf("total: expected 10000, got %s", cap.TotalCapacity.String())
	}
	if cap.UsedCapacity.Raw() != expectedUsed.Raw() {
		t.Errorf("used: expected 3000, got %s", cap.UsedCapacity.String())
	}
	if cap.AvailableCapacity.Raw() != expectedAvail.Raw() {
		t.Errorf("available: expected 7000, got %s", cap.AvailableCapacity.String())
	}
}

// ─── Full Receipt Lifecycle Test ─────────────────────────────────────────────

func TestFullReceiptLifecycle(t *testing.T) {
	svc, fac := setupTestService(t)

	// 1. Schedule and pass inspection
	insp := createPassedInspection(t, svc, fac.FacilityID, "wheat-id", "LOT-W001")

	// 2. Issue receipt
	r, err := svc.IssueReceipt(IssueReceiptRequest{
		FacilityID: fac.FacilityID, HolderID: "farmer-1", CommodityID: "wheat-id",
		Quantity: "1000", LotNumber: "LOT-W001", HarvestYear: 2026,
		InspectionID: insp.InspectionID, Grade: "Premium",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	// 3. Transfer to trader
	r, err = svc.TransferReceipt(r.ReceiptID, "trader-1")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if r.HolderID != "trader-1" {
		t.Errorf("expected holder trader-1, got %s", r.HolderID)
	}

	// 4. Pledge as collateral
	r, err = svc.PledgeReceipt(r.ReceiptID, "clearing-member-A")
	if err != nil {
		t.Fatalf("pledge: %v", err)
	}

	// 5. Release from pledge
	r, err = svc.ReleaseReceipt(r.ReceiptID, "clearing-member-A")
	if err != nil {
		t.Fatalf("release: %v", err)
	}

	// 6. Initiate delivery
	d, err := svc.InitiateDelivery(InitiateDeliveryRequest{
		ReceiptID: r.ReceiptID, ObligationID: "ob-100",
		BuyerID: "buyer-1", DeliveryType: types.DeliveryTypePhysical,
		ScheduledDate: "2026-05-01",
	})
	if err != nil {
		t.Fatalf("initiate delivery: %v", err)
	}

	// 7. Complete delivery
	_, err = svc.CompleteDelivery(d.DeliveryID, true, "")
	if err != nil {
		t.Fatalf("complete delivery: %v", err)
	}

	// 8. Verify final state
	finalReceipt, _ := svc.GetReceipt(r.ReceiptID)
	if finalReceipt.Status != types.ReceiptStatusDelivered {
		t.Errorf("expected final status DELIVERED, got %s", finalReceipt.Status)
	}

	// 9. Verify inventory is zero
	_, total := svc.GetInventory(fac.FacilityID, "wheat-id")
	if !total.IsZero() {
		t.Errorf("expected zero inventory after delivery, got %s", total.String())
	}

	// 10. Verify capacity freed
	cap, _ := svc.GetFacilityCapacity(fac.FacilityID)
	if cap.UsedCapacity.Raw() != 0 {
		t.Errorf("expected zero used capacity, got %s", cap.UsedCapacity.String())
	}
}
