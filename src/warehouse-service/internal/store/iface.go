package store

import "github.com/garudax-platform/warehouse-service/internal/types"

// DataStore defines the persistence interface for all warehouse entities.
// Both the in-memory Store and PostgresStore implement this interface.
type DataStore interface {
	// Facility operations
	CreateFacility(f *types.Facility) error
	GetFacility(facilityID string) (*types.Facility, error)
	UpdateFacility(f *types.Facility) error
	ListFacilities(region string, status types.FacilityStatus) []*types.Facility

	// Inspection operations
	CreateInspection(insp *types.Inspection) error
	GetInspection(inspectionID string) (*types.Inspection, error)
	UpdateInspection(insp *types.Inspection) error

	// Receipt operations
	CreateReceipt(r *types.Receipt) error
	GetReceipt(receiptID string) (*types.Receipt, error)
	ListReceipts(holderID, facilityID, commodityID string, status types.ReceiptStatus) []*types.Receipt
	TransferReceipt(receiptID, newHolderID string) (*types.Receipt, error)
	CancelReceipt(receiptID, reason string) (*types.Receipt, error)
	PledgeReceipt(receiptID, clearingMemberID string) (*types.Receipt, error)
	ReleaseReceipt(receiptID, clearingMemberID string) (*types.Receipt, error)

	// Delivery operations
	CreateDelivery(d *types.DeliveryInstruction) error
	GetDelivery(deliveryID string) (*types.DeliveryInstruction, error)
	ListDeliveries(sellerID, buyerID string, status types.DeliveryStatus) []*types.DeliveryInstruction
	CompleteDelivery(deliveryID string, success bool, failureReason string) (*types.DeliveryInstruction, error)

	// Inventory operations
	GetInventory(facilityID, commodityID string) ([]types.InventoryItem, types.Decimal)
	GetFacilityCapacity(facilityID string) (*types.FacilityCapacity, error)

	// Receipt events
	GetReceiptEvents(receiptID string) []types.ReceiptEvent
}

// Compile-time interface check for the in-memory store.
var _ DataStore = (*Store)(nil)
