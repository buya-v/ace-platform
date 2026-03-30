package store

import (
	"strings"
	"testing"

	"github.com/garudax-platform/warehouse-service/internal/types"
)

func setupFacility(t *testing.T, s *Store) *types.Facility {
	t.Helper()
	f := &types.Facility{
		FacilityCode:  "WH-01",
		Name:          "Test Warehouse",
		OperatorID:    "op-1",
		Region:        "East",
		TotalCapacity: types.DecimalFromInt(10000),
		CapacityUnit:  "MT",
		Status:        types.FacilityStatusActive,
	}
	if err := s.CreateFacility(f); err != nil {
		t.Fatalf("CreateFacility: %v", err)
	}
	return f
}

func setupInspection(t *testing.T, s *Store, facilityID string) *types.Inspection {
	t.Helper()
	insp := &types.Inspection{
		FacilityID:     facilityID,
		CommodityID:    "corn",
		LotNumber:      "LOT-001",
		InspectorID:    "inspector-1",
		InspectionType: types.InspectionTypeInitial,
		Status:         types.InspectionStatusPassed,
		ScheduledDate:  "2026-03-28",
	}
	if err := s.CreateInspection(insp); err != nil {
		t.Fatalf("CreateInspection: %v", err)
	}
	return insp
}

func TestStoreCreateAndGetFacility(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	got, err := s.GetFacility(f.FacilityID)
	if err != nil {
		t.Fatalf("GetFacility: %v", err)
	}
	if got.Name != "Test Warehouse" {
		t.Errorf("expected Test Warehouse, got %s", got.Name)
	}
}

func TestStoreGetFacilityNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetFacility("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing facility")
	}
}

func TestStoreDuplicateFacilityCode(t *testing.T) {
	s := NewStore()
	setupFacility(t, s)

	f2 := &types.Facility{
		FacilityCode:  "WH-01",
		Name:          "Dupe",
		OperatorID:    "op-2",
		Region:        "West",
		TotalCapacity: types.DecimalFromInt(5000),
		CapacityUnit:  "MT",
		Status:        types.FacilityStatusActive,
	}
	err := s.CreateFacility(f2)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestStoreCreateReceipt_LotUniqueness(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r1 := &types.Receipt{
		FacilityID:  f.FacilityID,
		HolderID:    "holder-1",
		CommodityID: "corn",
		Grade:       "G1",
		Quantity:    types.DecimalFromInt(100),
		GrossQuantity: types.DecimalFromInt(105),
		Unit:        "MT",
		LotNumber:   "LOT-001",
		HarvestYear: 2026,
	}
	if err := s.CreateReceipt(r1); err != nil {
		t.Fatalf("first receipt: %v", err)
	}

	r2 := &types.Receipt{
		FacilityID:  f.FacilityID,
		HolderID:    "holder-2",
		CommodityID: "corn",
		Grade:       "G1",
		Quantity:    types.DecimalFromInt(50),
		GrossQuantity: types.DecimalFromInt(52),
		Unit:        "MT",
		LotNumber:   "LOT-001",
		HarvestYear: 2026,
	}
	err := s.CreateReceipt(r2)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected lot uniqueness error, got %v", err)
	}
}

func TestStoreCreateReceipt_CapacityCheck(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s) // 10000 MT capacity

	r := &types.Receipt{
		FacilityID:  f.FacilityID,
		HolderID:    "holder-1",
		CommodityID: "corn",
		Quantity:    types.DecimalFromInt(10001),
		GrossQuantity: types.DecimalFromInt(10001),
		Unit:        "MT",
		LotNumber:   "LOT-BIG",
		HarvestYear: 2026,
	}
	err := s.CreateReceipt(r)
	if err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("expected capacity error, got %v", err)
	}
}

func TestStoreCreateReceipt_InactiveFacility(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)
	f.Status = types.FacilityStatusSuspended
	s.UpdateFacility(f)

	r := &types.Receipt{
		FacilityID:  f.FacilityID,
		HolderID:    "holder-1",
		CommodityID: "corn",
		Quantity:    types.DecimalFromInt(100),
		GrossQuantity: types.DecimalFromInt(100),
		Unit:        "MT",
		LotNumber:   "LOT-001",
		HarvestYear: 2026,
	}
	err := s.CreateReceipt(r)
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected inactive facility error, got %v", err)
	}
}

func TestStoreTransferReceipt(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "holder-1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(100), GrossQuantity: types.DecimalFromInt(100),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)

	transferred, err := s.TransferReceipt(r.ReceiptID, "holder-2")
	if err != nil {
		t.Fatalf("TransferReceipt: %v", err)
	}
	if transferred.HolderID != "holder-2" {
		t.Errorf("expected holder-2, got %s", transferred.HolderID)
	}
}

func TestStorePledgeAndRelease(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "holder-1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(100), GrossQuantity: types.DecimalFromInt(100),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)

	pledged, err := s.PledgeReceipt(r.ReceiptID, "cm-1")
	if err != nil {
		t.Fatalf("Pledge: %v", err)
	}
	if pledged.Status != types.ReceiptStatusPledged {
		t.Errorf("expected PLEDGED, got %s", pledged.Status)
	}

	released, err := s.ReleaseReceipt(r.ReceiptID, "cm-1")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if released.Status != types.ReceiptStatusActive {
		t.Errorf("expected ACTIVE, got %s", released.Status)
	}
}

func TestStoreDeliveryLifecycle(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "holder-1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(100), GrossQuantity: types.DecimalFromInt(100),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)

	d := &types.DeliveryInstruction{
		ReceiptID:    r.ReceiptID,
		ObligationID: "ob-1",
		BuyerID:      "buyer-1",
		DeliveryType: types.DeliveryTypePhysical,
	}
	if err := s.CreateDelivery(d); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	completed, err := s.CompleteDelivery(d.DeliveryID, true, "")
	if err != nil {
		t.Fatalf("CompleteDelivery: %v", err)
	}
	if completed.Status != types.DeliveryStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", completed.Status)
	}

	// Receipt should be DELIVERED
	receipt, _ := s.GetReceipt(r.ReceiptID)
	if receipt.Status != types.ReceiptStatusDelivered {
		t.Errorf("expected DELIVERED, got %s", receipt.Status)
	}
}

func TestStoreInventory(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "holder-1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(500), GrossQuantity: types.DecimalFromInt(500),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)

	items, total := s.GetInventory(f.FacilityID, "corn")
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if total.String() != "500" {
		t.Errorf("expected total 500, got %s", total.String())
	}

	// Cancel -> inventory goes to zero
	s.CancelReceipt(r.ReceiptID, "test")
	items, total = s.GetInventory(f.FacilityID, "corn")
	if len(items) != 0 {
		t.Errorf("expected 0 items after cancel, got %d", len(items))
	}
}

func TestStoreFacilityCapacity(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "holder-1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(3000), GrossQuantity: types.DecimalFromInt(3000),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)

	cap, err := s.GetFacilityCapacity(f.FacilityID)
	if err != nil {
		t.Fatalf("GetFacilityCapacity: %v", err)
	}
	if cap.UsedCapacity.String() != "3000" {
		t.Errorf("expected used 3000, got %s", cap.UsedCapacity.String())
	}
	if cap.AvailableCapacity.String() != "7000" {
		t.Errorf("expected available 7000, got %s", cap.AvailableCapacity.String())
	}
}

func TestStoreReceiptEvents(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "holder-1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(100), GrossQuantity: types.DecimalFromInt(100),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)
	s.TransferReceipt(r.ReceiptID, "holder-2")
	s.CancelReceipt(r.ReceiptID, "test")

	events := s.GetReceiptEvents(r.ReceiptID)
	if len(events) != 3 {
		t.Errorf("expected 3 events (ISSUED, TRANSFERRED, CANCELLED), got %d", len(events))
	}
	if events[0].EventType != types.ReceiptEventIssued {
		t.Errorf("first event: expected ISSUED, got %s", events[0].EventType)
	}
	if events[1].EventType != types.ReceiptEventTransferred {
		t.Errorf("second event: expected TRANSFERRED, got %s", events[1].EventType)
	}
	if events[2].EventType != types.ReceiptEventCancelled {
		t.Errorf("third event: expected CANCELLED, got %s", events[2].EventType)
	}
}

func TestStoreInspectionCreation(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	insp := setupInspection(t, s, f.FacilityID)
	if insp.InspectionID == "" {
		t.Fatal("expected inspection ID")
	}

	got, err := s.GetInspection(insp.InspectionID)
	if err != nil {
		t.Fatalf("GetInspection: %v", err)
	}
	if got.Status != types.InspectionStatusPassed {
		t.Errorf("expected PASSED, got %s", got.Status)
	}
}

func TestStoreInspectionFacilityNotFound(t *testing.T) {
	s := NewStore()
	insp := &types.Inspection{
		FacilityID: "nonexistent", CommodityID: "corn",
		LotNumber: "LOT-001", InspectorID: "inspector-1",
		ScheduledDate: "2026-04-01",
	}
	err := s.CreateInspection(insp)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected facility not found, got %v", err)
	}
}

func TestStoreListReceipts(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r1 := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "h1", CommodityID: "corn",
		Quantity: types.DecimalFromInt(100), GrossQuantity: types.DecimalFromInt(100),
		Unit: "MT", LotNumber: "LOT-A", HarvestYear: 2026,
	}
	r2 := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "h2", CommodityID: "wheat",
		Quantity: types.DecimalFromInt(200), GrossQuantity: types.DecimalFromInt(200),
		Unit: "MT", LotNumber: "LOT-B", HarvestYear: 2026,
	}
	s.CreateReceipt(r1)
	s.CreateReceipt(r2)

	all := s.ListReceipts("", "", "", "")
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}

	byHolder := s.ListReceipts("h1", "", "", "")
	if len(byHolder) != 1 {
		t.Errorf("expected 1 for h1, got %d", len(byHolder))
	}

	byCommodity := s.ListReceipts("", "", "wheat", "")
	if len(byCommodity) != 1 {
		t.Errorf("expected 1 for wheat, got %d", len(byCommodity))
	}
}

func TestStoreListDeliveries(t *testing.T) {
	s := NewStore()
	f := setupFacility(t, s)

	r := &types.Receipt{
		FacilityID: f.FacilityID, HolderID: "seller", CommodityID: "corn",
		Quantity: types.DecimalFromInt(100), GrossQuantity: types.DecimalFromInt(100),
		Unit: "MT", LotNumber: "LOT-001", HarvestYear: 2026,
	}
	s.CreateReceipt(r)

	d := &types.DeliveryInstruction{
		ReceiptID: r.ReceiptID, ObligationID: "ob-1", BuyerID: "buyer-1",
		DeliveryType: types.DeliveryTypePhysical,
	}
	s.CreateDelivery(d)

	all := s.ListDeliveries("", "", "")
	if len(all) != 1 {
		t.Errorf("expected 1 delivery, got %d", len(all))
	}

	bySeller := s.ListDeliveries("seller", "", "")
	if len(bySeller) != 1 {
		t.Errorf("expected 1 for seller, got %d", len(bySeller))
	}
}

func TestStoreListFacilities(t *testing.T) {
	s := NewStore()
	setupFacility(t, s)

	f2 := &types.Facility{
		FacilityCode:  "WH-02",
		Name:          "West Warehouse",
		OperatorID:    "op-2",
		Region:        "West",
		TotalCapacity: types.DecimalFromInt(5000),
		CapacityUnit:  "MT",
		Status:        types.FacilityStatusActive,
	}
	s.CreateFacility(f2)

	all := s.ListFacilities("", "")
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}

	east := s.ListFacilities("East", "")
	if len(east) != 1 {
		t.Errorf("expected 1 for East, got %d", len(east))
	}
}
