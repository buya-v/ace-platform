package store

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/warehouse-service/internal/types"
)

// Store provides thread-safe in-memory storage for all warehouse entities.
// In production, this would be replaced by a SQL repository with parameterized queries.
type Store struct {
	mu sync.RWMutex

	facilities   map[string]*types.Facility       // facilityID -> Facility
	receipts     map[string]*types.Receipt         // receiptID -> Receipt
	inspections  map[string]*types.Inspection      // inspectionID -> Inspection
	deliveries   map[string]*types.DeliveryInstruction // deliveryID -> Delivery
	receiptEvents []types.ReceiptEvent
	inventoryEvents []types.InventoryEvent

	// lot uniqueness: facilityID:commodityID:lotNumber -> receiptID (only active receipts)
	activeLots map[string]string

	idCounter uint64
}

// NewStore creates a new in-memory store.
func NewStore() *Store {
	return &Store{
		facilities:  make(map[string]*types.Facility),
		receipts:    make(map[string]*types.Receipt),
		inspections: make(map[string]*types.Inspection),
		deliveries:  make(map[string]*types.DeliveryInstruction),
		activeLots:  make(map[string]string),
	}
}

func (s *Store) nextID() string {
	n := atomic.AddUint64(&s.idCounter, 1)
	return fmt.Sprintf("wh-%d", n)
}

func lotKey(facilityID, commodityID, lotNumber string) string {
	return facilityID + ":" + commodityID + ":" + lotNumber
}

// --- Facility operations ---

func (s *Store) CreateFacility(f *types.Facility) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check facility_code uniqueness
	for _, existing := range s.facilities {
		if existing.FacilityCode == f.FacilityCode {
			return fmt.Errorf("facility code %q already exists", f.FacilityCode)
		}
	}

	f.FacilityID = s.nextID()
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	s.facilities[f.FacilityID] = f
	return nil
}

func (s *Store) GetFacility(facilityID string) (*types.Facility, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, ok := s.facilities[facilityID]
	if !ok {
		return nil, fmt.Errorf("facility %s not found", facilityID)
	}
	copy := *f
	return &copy, nil
}

func (s *Store) UpdateFacility(f *types.Facility) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.facilities[f.FacilityID]; !ok {
		return fmt.Errorf("facility %s not found", f.FacilityID)
	}
	f.UpdatedAt = time.Now().UTC()
	s.facilities[f.FacilityID] = f
	return nil
}

func (s *Store) ListFacilities(region string, status types.FacilityStatus) []*types.Facility {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*types.Facility
	for _, f := range s.facilities {
		if region != "" && f.Region != region {
			continue
		}
		if status != "" && f.Status != status {
			continue
		}
		copy := *f
		result = append(result, &copy)
	}
	return result
}

// --- Inspection operations ---

func (s *Store) CreateInspection(insp *types.Inspection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.facilities[insp.FacilityID]; !ok {
		return fmt.Errorf("facility %s not found", insp.FacilityID)
	}

	insp.InspectionID = s.nextID()
	now := time.Now().UTC()
	insp.CreatedAt = now
	insp.UpdatedAt = now
	s.inspections[insp.InspectionID] = insp
	return nil
}

func (s *Store) GetInspection(inspectionID string) (*types.Inspection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	insp, ok := s.inspections[inspectionID]
	if !ok {
		return nil, fmt.Errorf("inspection %s not found", inspectionID)
	}
	copy := *insp
	return &copy, nil
}

func (s *Store) UpdateInspection(insp *types.Inspection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.inspections[insp.InspectionID]; !ok {
		return fmt.Errorf("inspection %s not found", insp.InspectionID)
	}
	insp.UpdatedAt = time.Now().UTC()
	s.inspections[insp.InspectionID] = insp
	return nil
}

// --- Receipt operations ---

func (s *Store) CreateReceipt(r *types.Receipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check lot uniqueness (one active receipt per lot per facility/commodity)
	lk := lotKey(r.FacilityID, r.CommodityID, r.LotNumber)
	if existingID, exists := s.activeLots[lk]; exists {
		return fmt.Errorf("active receipt %s already exists for lot %s at facility %s",
			existingID, r.LotNumber, r.FacilityID)
	}

	// Check facility exists and is active
	fac, ok := s.facilities[r.FacilityID]
	if !ok {
		return fmt.Errorf("facility %s not found", r.FacilityID)
	}
	if fac.Status != types.FacilityStatusActive {
		return fmt.Errorf("facility %s is not active (status: %s)", r.FacilityID, fac.Status)
	}

	// Check facility capacity
	usedCap := s.usedCapacityLocked(r.FacilityID)
	availCap := fac.TotalCapacity.Sub(usedCap)
	if r.Quantity.GreaterThan(availCap) {
		return fmt.Errorf("insufficient facility capacity: available=%s, requested=%s",
			availCap.String(), r.Quantity.String())
	}

	r.ReceiptID = s.nextID()
	r.ReceiptNumber = fmt.Sprintf("EWR-%s", r.ReceiptID)
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	r.IssuedAt = now
	if r.ExpiresAt.IsZero() {
		r.ExpiresAt = now.AddDate(1, 0, 0)
	}
	r.Status = types.ReceiptStatusActive

	s.receipts[r.ReceiptID] = r
	s.activeLots[lk] = r.ReceiptID

	// Record audit event
	s.appendReceiptEventLocked(types.ReceiptEvent{
		EventID:    s.nextID(),
		ReceiptID:  r.ReceiptID,
		EventType:  types.ReceiptEventIssued,
		ToHolderID: r.HolderID,
		CreatedAt:  now,
	})

	// Record inventory deposit
	s.appendInventoryEventLocked(types.InventoryEvent{
		EventID:       s.nextID(),
		FacilityID:    r.FacilityID,
		CommodityID:   r.CommodityID,
		LotNumber:     r.LotNumber,
		EventType:     types.InventoryEventDeposit,
		Quantity:      r.Quantity,
		ReferenceID:   r.ReceiptID,
		ReferenceType: "RECEIPT",
		CreatedBy:     r.HolderID,
		CreatedAt:     now,
	})

	return nil
}

func (s *Store) GetReceipt(receiptID string) (*types.Receipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.receipts[receiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	copy := *r
	return &copy, nil
}

func (s *Store) ListReceipts(holderID, facilityID, commodityID string, status types.ReceiptStatus) []*types.Receipt {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*types.Receipt
	for _, r := range s.receipts {
		if holderID != "" && r.HolderID != holderID {
			continue
		}
		if facilityID != "" && r.FacilityID != facilityID {
			continue
		}
		if commodityID != "" && r.CommodityID != commodityID {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		copy := *r
		result = append(result, &copy)
	}
	return result
}

func (s *Store) TransferReceipt(receiptID, newHolderID string) (*types.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.receipts[receiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if r.Status != types.ReceiptStatusActive {
		return nil, fmt.Errorf("receipt %s cannot be transferred (status: %s)", receiptID, r.Status)
	}

	oldHolder := r.HolderID
	now := time.Now().UTC()
	r.HolderID = newHolderID
	r.UpdatedAt = now

	s.appendReceiptEventLocked(types.ReceiptEvent{
		EventID:      s.nextID(),
		ReceiptID:    receiptID,
		EventType:    types.ReceiptEventTransferred,
		FromHolderID: oldHolder,
		ToHolderID:   newHolderID,
		CreatedAt:    now,
	})

	copy := *r
	return &copy, nil
}

func (s *Store) CancelReceipt(receiptID, reason string) (*types.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.receipts[receiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if r.Status != types.ReceiptStatusActive {
		return nil, fmt.Errorf("receipt %s cannot be cancelled (status: %s)", receiptID, r.Status)
	}

	now := time.Now().UTC()
	r.Status = types.ReceiptStatusCancelled
	r.UpdatedAt = now

	// Remove from active lots
	lk := lotKey(r.FacilityID, r.CommodityID, r.LotNumber)
	delete(s.activeLots, lk)

	s.appendReceiptEventLocked(types.ReceiptEvent{
		EventID:   s.nextID(),
		ReceiptID: receiptID,
		EventType: types.ReceiptEventCancelled,
		Metadata:  reason,
		CreatedAt: now,
	})

	// Record inventory withdrawal
	s.appendInventoryEventLocked(types.InventoryEvent{
		EventID:       s.nextID(),
		FacilityID:    r.FacilityID,
		CommodityID:   r.CommodityID,
		LotNumber:     r.LotNumber,
		EventType:     types.InventoryEventWithdrawal,
		Quantity:      types.DecimalFromRaw(-r.Quantity.Raw()),
		ReferenceID:   r.ReceiptID,
		ReferenceType: "RECEIPT_CANCEL",
		CreatedBy:     r.HolderID,
		CreatedAt:     now,
	})

	copy := *r
	return &copy, nil
}

func (s *Store) PledgeReceipt(receiptID, clearingMemberID string) (*types.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.receipts[receiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if r.Status != types.ReceiptStatusActive {
		return nil, fmt.Errorf("receipt %s cannot be pledged (status: %s)", receiptID, r.Status)
	}

	now := time.Now().UTC()
	r.Status = types.ReceiptStatusPledged
	r.PledgedTo = clearingMemberID
	r.UpdatedAt = now

	s.appendReceiptEventLocked(types.ReceiptEvent{
		EventID:   s.nextID(),
		ReceiptID: receiptID,
		EventType: types.ReceiptEventPledged,
		PledgedTo: clearingMemberID,
		CreatedAt: now,
	})

	copy := *r
	return &copy, nil
}

func (s *Store) ReleaseReceipt(receiptID, clearingMemberID string) (*types.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.receipts[receiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if r.Status != types.ReceiptStatusPledged {
		return nil, fmt.Errorf("receipt %s is not pledged (status: %s)", receiptID, r.Status)
	}
	if r.PledgedTo != clearingMemberID {
		return nil, fmt.Errorf("receipt %s is pledged to %s, not %s", receiptID, r.PledgedTo, clearingMemberID)
	}

	now := time.Now().UTC()
	r.Status = types.ReceiptStatusActive
	r.PledgedTo = ""
	r.UpdatedAt = now

	s.appendReceiptEventLocked(types.ReceiptEvent{
		EventID:   s.nextID(),
		ReceiptID: receiptID,
		EventType: types.ReceiptEventReleased,
		PledgedTo: clearingMemberID,
		CreatedAt: now,
	})

	copy := *r
	return &copy, nil
}

// --- Delivery operations ---

func (s *Store) CreateDelivery(d *types.DeliveryInstruction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.receipts[d.ReceiptID]
	if !ok {
		return fmt.Errorf("receipt %s not found", d.ReceiptID)
	}
	if r.Status != types.ReceiptStatusActive && r.Status != types.ReceiptStatusPledged {
		return fmt.Errorf("receipt %s cannot be delivered (status: %s)", d.ReceiptID, r.Status)
	}

	// Mark receipt as delivery pending
	now := time.Now().UTC()
	r.Status = types.ReceiptStatusDeliveryPending
	r.UpdatedAt = now

	d.DeliveryID = s.nextID()
	d.SellerID = r.HolderID
	d.FacilityID = r.FacilityID
	d.Quantity = r.Quantity
	d.Status = types.DeliveryStatusPending
	d.CreatedAt = now
	d.UpdatedAt = now

	s.deliveries[d.DeliveryID] = d

	s.appendReceiptEventLocked(types.ReceiptEvent{
		EventID:   s.nextID(),
		ReceiptID: d.ReceiptID,
		EventType: types.ReceiptEventDeliveryInitiated,
		CreatedAt: now,
	})

	return nil
}

func (s *Store) GetDelivery(deliveryID string) (*types.DeliveryInstruction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	d, ok := s.deliveries[deliveryID]
	if !ok {
		return nil, fmt.Errorf("delivery %s not found", deliveryID)
	}
	copy := *d
	return &copy, nil
}

func (s *Store) ListDeliveries(sellerID, buyerID string, status types.DeliveryStatus) []*types.DeliveryInstruction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*types.DeliveryInstruction
	for _, d := range s.deliveries {
		if sellerID != "" && d.SellerID != sellerID {
			continue
		}
		if buyerID != "" && d.BuyerID != buyerID {
			continue
		}
		if status != "" && d.Status != status {
			continue
		}
		copy := *d
		result = append(result, &copy)
	}
	return result
}

func (s *Store) CompleteDelivery(deliveryID string, success bool, failureReason string) (*types.DeliveryInstruction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.deliveries[deliveryID]
	if !ok {
		return nil, fmt.Errorf("delivery %s not found", deliveryID)
	}
	if d.Status != types.DeliveryStatusPending && d.Status != types.DeliveryStatusInProgress {
		return nil, fmt.Errorf("delivery %s cannot be completed (status: %s)", deliveryID, d.Status)
	}

	now := time.Now().UTC()
	d.CompletedAt = &now
	d.UpdatedAt = now

	r, ok := s.receipts[d.ReceiptID]
	if !ok {
		return nil, fmt.Errorf("receipt %s not found for delivery", d.ReceiptID)
	}

	if success {
		d.Status = types.DeliveryStatusCompleted
		r.Status = types.ReceiptStatusDelivered
		r.UpdatedAt = now

		// Remove from active lots
		lk := lotKey(r.FacilityID, r.CommodityID, r.LotNumber)
		delete(s.activeLots, lk)

		s.appendReceiptEventLocked(types.ReceiptEvent{
			EventID:   s.nextID(),
			ReceiptID: d.ReceiptID,
			EventType: types.ReceiptEventDelivered,
			CreatedAt: now,
		})

		// Record inventory withdrawal for delivery
		s.appendInventoryEventLocked(types.InventoryEvent{
			EventID:       s.nextID(),
			FacilityID:    r.FacilityID,
			CommodityID:   r.CommodityID,
			LotNumber:     r.LotNumber,
			EventType:     types.InventoryEventWithdrawal,
			Quantity:      types.DecimalFromRaw(-r.Quantity.Raw()),
			ReferenceID:   d.DeliveryID,
			ReferenceType: "DELIVERY",
			CreatedBy:     d.BuyerID,
			CreatedAt:     now,
		})
	} else {
		d.Status = types.DeliveryStatusFailed
		d.FailureReason = failureReason
		// Revert receipt to active
		r.Status = types.ReceiptStatusActive
		r.UpdatedAt = now
	}

	copy := *d
	return &copy, nil
}

// --- Inventory operations ---

func (s *Store) GetInventory(facilityID, commodityID string) ([]types.InventoryItem, types.Decimal) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Aggregate inventory events
	type lotKey struct {
		facilityID  string
		commodityID string
		lotNumber   string
	}
	agg := make(map[lotKey]int64)

	for _, e := range s.inventoryEvents {
		if facilityID != "" && e.FacilityID != facilityID {
			continue
		}
		if commodityID != "" && e.CommodityID != commodityID {
			continue
		}
		k := lotKey{e.FacilityID, e.CommodityID, e.LotNumber}
		agg[k] += e.Quantity.Raw()
	}

	var items []types.InventoryItem
	total := types.DecimalZero()
	for k, raw := range agg {
		if raw <= 0 {
			continue
		}
		qty := types.DecimalFromRaw(raw)
		items = append(items, types.InventoryItem{
			FacilityID:      k.facilityID,
			CommodityID:     k.commodityID,
			LotNumber:       k.lotNumber,
			CurrentQuantity: qty,
			Unit:            "MT",
		})
		total = total.Add(qty)
	}
	return items, total
}

func (s *Store) GetFacilityCapacity(facilityID string) (*types.FacilityCapacity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fac, ok := s.facilities[facilityID]
	if !ok {
		return nil, fmt.Errorf("facility %s not found", facilityID)
	}

	used := s.usedCapacityLocked(facilityID)
	return &types.FacilityCapacity{
		TotalCapacity:     fac.TotalCapacity,
		UsedCapacity:      used,
		AvailableCapacity: fac.TotalCapacity.Sub(used),
		CapacityUnit:      fac.CapacityUnit,
	}, nil
}

// usedCapacityLocked computes used capacity from active receipts. Must be called with mu held.
func (s *Store) usedCapacityLocked(facilityID string) types.Decimal {
	used := types.DecimalZero()
	for _, r := range s.receipts {
		if r.FacilityID != facilityID {
			continue
		}
		if r.Status == types.ReceiptStatusActive ||
			r.Status == types.ReceiptStatusPledged ||
			r.Status == types.ReceiptStatusDeliveryPending {
			used = used.Add(r.Quantity)
		}
	}
	return used
}

// --- Receipt events ---

func (s *Store) GetReceiptEvents(receiptID string) []types.ReceiptEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var events []types.ReceiptEvent
	for _, e := range s.receiptEvents {
		if e.ReceiptID == receiptID {
			events = append(events, e)
		}
	}
	return events
}

func (s *Store) appendReceiptEventLocked(e types.ReceiptEvent) {
	s.receiptEvents = append(s.receiptEvents, e)
}

func (s *Store) appendInventoryEventLocked(e types.InventoryEvent) {
	s.inventoryEvents = append(s.inventoryEvents, e)
}
