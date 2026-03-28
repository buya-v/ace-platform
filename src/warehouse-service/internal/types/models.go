package types

import "time"

// ReceiptStatus represents the state of a warehouse receipt.
type ReceiptStatus string

const (
	ReceiptStatusActive          ReceiptStatus = "ACTIVE"
	ReceiptStatusPledged         ReceiptStatus = "PLEDGED"
	ReceiptStatusDeliveryPending ReceiptStatus = "DELIVERY_PENDING"
	ReceiptStatusDelivered       ReceiptStatus = "DELIVERED"
	ReceiptStatusCancelled       ReceiptStatus = "CANCELLED"
)

// FacilityStatus represents the state of a warehouse facility.
type FacilityStatus string

const (
	FacilityStatusActive          FacilityStatus = "ACTIVE"
	FacilityStatusSuspended       FacilityStatus = "SUSPENDED"
	FacilityStatusDecommissioned  FacilityStatus = "DECOMMISSIONED"
)

// DeliveryType represents how a delivery is settled.
type DeliveryType string

const (
	DeliveryTypePhysical       DeliveryType = "PHYSICAL"
	DeliveryTypeCashSettlement DeliveryType = "CASH_SETTLEMENT"
)

// DeliveryStatus represents the state of a delivery instruction.
type DeliveryStatus string

const (
	DeliveryStatusPending    DeliveryStatus = "PENDING"
	DeliveryStatusInProgress DeliveryStatus = "IN_PROGRESS"
	DeliveryStatusCompleted  DeliveryStatus = "COMPLETED"
	DeliveryStatusFailed     DeliveryStatus = "FAILED"
	DeliveryStatusCancelled  DeliveryStatus = "CANCELLED"
)

// InspectionType represents the reason for an inspection.
type InspectionType string

const (
	InspectionTypeInitial      InspectionType = "INITIAL"
	InspectionTypeReInspection InspectionType = "RE_INSPECTION"
	InspectionTypePeriodic     InspectionType = "PERIODIC"
)

// InspectionStatus represents the state of an inspection.
type InspectionStatus string

const (
	InspectionStatusScheduled  InspectionStatus = "SCHEDULED"
	InspectionStatusInProgress InspectionStatus = "IN_PROGRESS"
	InspectionStatusPassed     InspectionStatus = "PASSED"
	InspectionStatusFailed     InspectionStatus = "FAILED"
)

// ReceiptEventType represents a receipt audit trail event type.
type ReceiptEventType string

const (
	ReceiptEventIssued           ReceiptEventType = "ISSUED"
	ReceiptEventTransferred      ReceiptEventType = "TRANSFERRED"
	ReceiptEventPledged          ReceiptEventType = "PLEDGED"
	ReceiptEventReleased         ReceiptEventType = "RELEASED"
	ReceiptEventDeliveryInitiated ReceiptEventType = "DELIVERY_INITIATED"
	ReceiptEventDelivered        ReceiptEventType = "DELIVERED"
	ReceiptEventCancelled        ReceiptEventType = "CANCELLED"
)

// InventoryEventType represents inventory movement types.
type InventoryEventType string

const (
	InventoryEventDeposit     InventoryEventType = "DEPOSIT"
	InventoryEventWithdrawal  InventoryEventType = "WITHDRAWAL"
	InventoryEventAdjustment  InventoryEventType = "ADJUSTMENT"
)

// Receipt is an electronic warehouse receipt (eWR).
type Receipt struct {
	ReceiptID       string
	ReceiptNumber   string
	FacilityID      string
	HolderID        string
	CommodityID     string
	Grade           string
	Quantity        Decimal
	GrossQuantity   Decimal
	Unit            string
	LotNumber       string
	StorageLocation string
	HarvestYear     int
	InspectionID    string
	Status          ReceiptStatus
	PledgedTo       string
	IssuedAt        time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Facility is a registered warehouse facility.
type Facility struct {
	FacilityID          string
	FacilityCode        string
	Name                string
	OperatorID          string
	LicenseNumber       string
	LicenseExpiry       string
	Address             string
	Latitude            string
	Longitude           string
	Region              string
	TotalCapacity       Decimal
	CapacityUnit        string
	ApprovedCommodityIDs []string
	Status              FacilityStatus
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// DeliveryInstruction represents a delivery order.
type DeliveryInstruction struct {
	DeliveryID    string
	ReceiptID     string
	ObligationID  string
	SellerID      string
	BuyerID       string
	DeliveryType  DeliveryType
	FacilityID    string
	DestinationID string
	Quantity      Decimal
	ScheduledDate string
	Status        DeliveryStatus
	FailureReason string
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Inspection represents a quality inspection record.
type Inspection struct {
	InspectionID      string
	FacilityID        string
	CommodityID       string
	LotNumber         string
	InspectorID       string
	InspectionType    InspectionType
	Status            InspectionStatus
	ScheduledDate     string
	CompletedDate     string
	GrossWeight       Decimal
	NetWeight         Decimal
	MoisturePct       Decimal
	ForeignMatterPct  Decimal
	ProteinPct        Decimal
	TestWeight        Decimal
	GradeAssigned     string
	Defects           string
	Notes             string
	CertificateNumber string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ReceiptEvent is an audit trail entry for receipt state changes.
type ReceiptEvent struct {
	EventID      string
	ReceiptID    string
	EventType    ReceiptEventType
	FromHolderID string
	ToHolderID   string
	PledgedTo    string
	Metadata     string
	CreatedAt    time.Time
}

// InventoryEvent is a double-entry inventory movement record.
type InventoryEvent struct {
	EventID       string
	FacilityID    string
	CommodityID   string
	LotNumber     string
	EventType     InventoryEventType
	Quantity      Decimal
	ReferenceID   string
	ReferenceType string
	Notes         string
	CreatedBy     string
	CreatedAt     time.Time
}

// InventoryItem represents aggregated inventory for a facility/commodity/lot.
type InventoryItem struct {
	FacilityID      string
	CommodityID     string
	LotNumber       string
	CurrentQuantity Decimal
	Unit            string
}

// FacilityCapacity represents capacity utilization of a facility.
type FacilityCapacity struct {
	TotalCapacity     Decimal
	UsedCapacity      Decimal
	AvailableCapacity Decimal
	CapacityUnit      string
}
