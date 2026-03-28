# Warehouse Service Architecture Specification

**Document ID:** T037-SPEC-001
**Version:** 1.0
**Date:** 2026-03-28
**Status:** DRAFT
**Author:** Coder Agent (Phase 3)

---

## Table of Contents

1. [Overview](#1-overview)
2. [System Context](#2-system-context)
3. [Electronic Warehouse Receipt (eWR) Lifecycle](#3-electronic-warehouse-receipt-ewr-lifecycle)
4. [Warehouse & Facility Registry](#4-warehouse--facility-registry)
5. [Commodity Inventory Management](#5-commodity-inventory-management)
6. [Delivery Instruction Processing](#6-delivery-instruction-processing)
7. [Quality Inspection & Grading](#7-quality-inspection--grading)
8. [Receipt Collateralization](#8-receipt-collateralization)
9. [API Contracts (gRPC)](#9-api-contracts-grpc)
10. [SQL Data Model](#10-sql-data-model)
11. [Integration Points](#11-integration-points)
12. [Performance Requirements](#12-performance-requirements)
13. [Deployment Architecture](#13-deployment-architecture)
14. [Failure Modes & Recovery](#14-failure-modes--recovery)

---

## 1. Overview

The ACE Warehouse Service manages the lifecycle of Electronic Warehouse Receipts (eWRs) for physical commodity storage, transfer, and delivery on the Agriculture Commodity Exchange of Mongolia. It is the bridge between physical commodities and the exchange's financial trading system.

### Design Principles

- **Immutable receipt ledger**: Every eWR state change (issuance, transfer, pledge, cancellation) is recorded as an append-only event. Receipts themselves are never deleted — they transition to terminal states (CANCELLED, DELIVERED).
- **Double-entry inventory**: Commodity quantities are tracked with double-entry accounting — every inbound creates a credit, every outbound creates a debit. Running balances are derived, never stored directly.
- **Zero-dependency Go module**: Following the established pattern (matching-engine, clearing-engine, etc.), the warehouse service is a standalone Go service with no shared library dependencies.
- **Regulatory compliance**: eWR issuance follows Mongolian grain warehouse receipt standards. Each receipt has a unique serial number traceable to a licensed facility.

### Scope

This spec covers:
- eWR lifecycle management (issuance → transfer → delivery/cancellation)
- Warehouse facility registration and capacity tracking
- Commodity inventory with grade, lot, and location tracking
- Physical delivery instruction processing
- Quality inspection and grading workflow
- Receipt collateralization (pledge/release for margin)
- gRPC API contracts for all operations
- SQL migration for the `warehouse` schema

This spec does NOT cover:
- Exchange order matching (T007/T008)
- Clearing and settlement financial flows (T027)
- Market data distribution
- Authentication and authorization (T005)

---

## 2. System Context

```
                    +------------------+
                    |   API Gateway    |
                    +--------+---------+
                             |
                   gRPC (receipt ops, delivery, inventory)
                             |
                    +--------v---------+
                    | Warehouse Service |
                    | gRPC :50058      |
                    | health :8088     |
                    +--+-----+------+--+
                       |     |      |
          +------------+     |      +-------------+
          |                  |                    |
  +-------v-------+  +------v--------+  +--------v--------+
  | PostgreSQL     |  | Kafka/NATS    |  | Clearing Engine  |
  | (warehouse     |  | (receipt      |  | :50052           |
  |  schema)       |  |  events)      |  | (delivery oblig.)|
  +----------------+  +---------------+  +-----------------+
                                                |
                                         +------v--------+
                                         | Margin Engine  |
                                         | :50053         |
                                         | (collateral)   |
                                         +---------------+
```

### Service Dependencies

| Dependency | Purpose | Protocol |
|---|---|---|
| PostgreSQL (`warehouse` schema) | Receipt persistence, inventory, facility registry | SQL via `ace_warehouse_svc` role |
| Kafka/NATS | Receipt event publishing (issuance, transfer, delivery) | Async messaging |
| Clearing Engine (:50052) | Query delivery obligations, confirm physical delivery completion | gRPC |
| Margin Engine (:50053) | Notify of pledged/released collateral for margin calculations | gRPC |
| Compliance Service (:50056) | KYC verification for receipt holders, audit trail | gRPC |

---

## 3. Electronic Warehouse Receipt (eWR) Lifecycle

### 3.1 Receipt State Machine

```
                         +----------+
        IssueReceipt --> |  ACTIVE  |
                         +----+-----+
                              |
              +-------+-------+-------+-------+
              |       |               |       |
        (transfer) (pledge)      (deliver) (cancel)
              |       |               |       |
        +-----v-+  +--v------+  +----v---+ +-v--------+
        | ACTIVE |  | PLEDGED |  |DELIVERY| |CANCELLED |
        | (new   |  +-+-------+  |_PENDING| +----------+
        | holder)|    |          +---+----+
        +--------+    |              |
                 (release)     (complete)
                      |              |
                +-----v-+     +-----v-----+
                | ACTIVE |    | DELIVERED  |
                +--------+    +-----------+
```

Terminal states: `CANCELLED`, `DELIVERED`.

### 3.2 Receipt Data Structure

```
WarehouseReceipt {
    receipt_id:         UUID
    receipt_number:     string          // Sequential: "eWR-{facility_code}-{YYYYMMDD}-{seq}"
    facility_id:        UUID
    holder_id:          UUID            // Current owner (participant_id)
    commodity_id:       UUID
    grade:              string          // Quality grade (e.g., "GRADE_A", "GRADE_B")
    quantity:           Decimal(18,4)   // Net quantity in standard units (metric tonnes)
    gross_quantity:     Decimal(18,4)   // Gross weight before deductions
    unit:               string          // "MT" (metric tonnes), "KG", "HEAD" (livestock)
    lot_number:         string          // Physical lot identifier within the facility
    storage_location:   string          // Bin/silo/bay identifier
    harvest_year:       int             // Crop year
    inspection_id:      UUID            // Reference to quality inspection record
    status:             ACTIVE | PLEDGED | DELIVERY_PENDING | DELIVERED | CANCELLED
    pledged_to:         UUID            // Clearing member ID if pledged, null otherwise
    issued_at:          Timestamp
    expires_at:         Timestamp       // Receipt expiry (max 1 year from issuance)
    cancelled_at:       Timestamp
    delivered_at:       Timestamp
    created_at:         Timestamp
    updated_at:         Timestamp
}
```

### 3.3 Receipt Number Convention

Format: `eWR-{FACILITY_CODE}-{YYYYMMDD}-{SEQUENCE}`

Examples:
- `eWR-UB001-20260715-00001` — Ulaanbaatar facility #001, issued July 15, sequence 1
- `eWR-DK003-20260901-00042` — Darkhan facility #003, issued Sept 1, sequence 42

The sequence resets daily per facility. Receipt numbers are immutable once assigned.

### 3.4 Issuance Rules

1. Facility must be `ACTIVE` and licensed
2. Commodity must match facility's approved commodity list
3. Quantity must not exceed facility's remaining capacity
4. Quality inspection must be `PASSED` with a valid inspection record
5. Holder (depositor) must have an active participant account with completed KYC
6. Receipt expiry defaults to 365 days from issuance (configurable per facility)

### 3.5 Transfer Rules

1. Only `ACTIVE` receipts can be transferred
2. `PLEDGED` receipts cannot be transferred (must be released first)
3. New holder must have an active participant account with completed KYC
4. Transfer creates an event in `warehouse.receipt_events` with from/to holder
5. Transfers are atomic — the receipt holder changes in a single transaction

### 3.6 Cancellation Rules

1. Only `ACTIVE` receipts can be cancelled
2. `PLEDGED` receipts cannot be cancelled (must be released first)
3. `DELIVERY_PENDING` receipts cannot be cancelled
4. Cancellation restores capacity to the facility
5. Cancelled receipts retain all data for audit (soft-delete via status)

---

## 4. Warehouse & Facility Registry

### 4.1 Facility Data Structure

```
Facility {
    facility_id:        UUID
    facility_code:      string          // Unique short code (e.g., "UB001", "DK003")
    name:               string
    operator_id:        UUID            // Participant who operates the facility
    license_number:     string          // Regulatory license
    license_expiry:     Date
    address:            string
    latitude:           Decimal(10,7)
    longitude:          Decimal(10,7)
    region:             string          // Province/Aimag
    total_capacity:     Decimal(18,4)   // Maximum storage in metric tonnes
    capacity_unit:      string          // "MT"
    approved_commodities: UUID[]        // Which commodities this facility can store
    status:             ACTIVE | SUSPENDED | DECOMMISSIONED
    created_at:         Timestamp
    updated_at:         Timestamp
}
```

### 4.2 Capacity Tracking

Available capacity is computed, not stored:

```
available_capacity(facility_id) =
    facility.total_capacity
    - SUM(receipt.quantity WHERE receipt.facility_id = facility_id
                           AND receipt.status IN ('ACTIVE', 'PLEDGED', 'DELIVERY_PENDING'))
```

A facility cannot accept new deposits if `available_capacity < deposit_quantity`.

### 4.3 Facility Lifecycle

| State | Meaning |
|---|---|
| `ACTIVE` | Accepting deposits, issuing receipts |
| `SUSPENDED` | No new deposits; existing receipts still valid and transferable |
| `DECOMMISSIONED` | Permanently closed; all receipts must be withdrawn or transferred |

Suspension triggers an event notification to all receipt holders at the facility.

---

## 5. Commodity Inventory Management

### 5.1 Inventory Event Model (Double-Entry)

Every physical movement creates an inventory event:

```
InventoryEvent {
    event_id:       UUID
    facility_id:    UUID
    commodity_id:   UUID
    lot_number:     string
    event_type:     DEPOSIT | WITHDRAWAL | ADJUSTMENT | TRANSFER_IN | TRANSFER_OUT | SHRINKAGE
    quantity:       Decimal(18,4)   // Positive for inflows, negative for outflows
    reference_id:   UUID            // Receipt ID or delivery ID
    reference_type: string          // "RECEIPT", "DELIVERY", "INSPECTION"
    notes:          string
    created_by:     UUID            // Operator or system
    created_at:     Timestamp
}
```

### 5.2 Event Types

| Event Type | Trigger | Effect |
|---|---|---|
| `DEPOSIT` | Receipt issuance | +quantity at facility |
| `WITHDRAWAL` | Delivery completion or cancellation | -quantity at facility |
| `ADJUSTMENT` | Re-inspection reveals quantity discrepancy | +/- correction |
| `TRANSFER_IN` | Inter-facility transfer received | +quantity at destination |
| `TRANSFER_OUT` | Inter-facility transfer sent | -quantity at origin |
| `SHRINKAGE` | Natural loss (moisture, pests) | -quantity (logged for audit) |

### 5.3 Lot Tracking

Each physical lot within a facility is identified by `lot_number`. Lots are:
- Assigned at deposit time
- Unique within a facility (not globally)
- Immutable once assigned to a receipt
- Used for traceability from receipt back to physical storage

### 5.4 Inventory Query

Current inventory per facility/commodity/lot:

```sql
SELECT facility_id, commodity_id, lot_number,
       SUM(quantity) AS current_quantity
FROM warehouse.inventory_events
GROUP BY facility_id, commodity_id, lot_number
HAVING SUM(quantity) > 0;
```

---

## 6. Delivery Instruction Processing

### 6.1 Delivery Workflow

Physical delivery connects the exchange's financial settlement to warehouse operations.

```
    Clearing Engine                  Warehouse Service
    (delivery obligation)            (physical fulfillment)
           |
    1. CreateDeliveryObligation
           |
           +------- InitiateDelivery -------->
           |                                  |
           |                          2. Validate receipt
           |                             matches obligation
           |                                  |
           |                          3. Status -> DELIVERY_PENDING
           |                                  |
           |                          4. Schedule physical
           |                             pickup/transfer
           |                                  |
           |       <-- DeliveryConfirmed -----+
           |                                  |
    5. Mark obligation FULFILLED       6. Status -> DELIVERED
           |                              Withdrawal event
           |
    7. Settlement proceeds
```

### 6.2 Delivery Instruction Data

```
DeliveryInstruction {
    delivery_id:        UUID
    receipt_id:         UUID            // The eWR being delivered
    obligation_id:      UUID            // Clearing engine delivery obligation
    seller_id:          UUID            // Participant delivering the commodity
    buyer_id:           UUID            // Participant receiving the commodity
    delivery_type:      PHYSICAL | CASH_SETTLEMENT
    facility_id:        UUID            // Where the commodity is stored
    destination_id:     UUID            // Destination facility (if physical transfer)
    quantity:           Decimal(18,4)
    scheduled_date:     Date            // Expected delivery date
    status:             PENDING | IN_PROGRESS | COMPLETED | FAILED | CANCELLED
    failure_reason:     string
    completed_at:       Timestamp
    created_at:         Timestamp
    updated_at:         Timestamp
}
```

### 6.3 Delivery Types

| Type | Process |
|---|---|
| **PHYSICAL** | Commodity physically moves from seller's facility to buyer. Receipt is cancelled at origin; new receipt issued at destination if buyer wants to store. |
| **CASH_SETTLEMENT** | No physical movement. Delivery obligation is settled financially. Receipt status remains unchanged (buyer may take physical delivery later). |

### 6.4 Delivery Validation

Before initiating delivery:
1. Receipt must be `ACTIVE` (not `PLEDGED` or already in delivery)
2. Receipt holder must match the seller in the delivery obligation
3. Receipt commodity/grade must match the obligation's contract specs
4. Receipt quantity must be >= obligation quantity
5. Delivery date must be within the contract's delivery window

### 6.5 Partial Delivery

If a receipt covers more than the obligation quantity:
- Original receipt is split: obligation quantity delivered, remainder stays as a new `ACTIVE` receipt
- The split is atomic — no intermediate state where quantity is unaccounted

---

## 7. Quality Inspection & Grading

### 7.1 Inspection Workflow

```
Deposit Request
      |
      v
+-----+------+
| SCHEDULED  |  (inspector assigned, date set)
+-----+------+
      |
      v
+-----+------+
| IN_PROGRESS|  (inspector on-site, sampling)
+-----+------+
      |
  +---+---+
  |       |
PASS    FAIL
  |       |
  v       v
+----+ +------+
|PASS| |FAILED|
+----+ +------+
  |       |
  v       v
Issue   Reject
Receipt deposit
```

### 7.2 Inspection Record

```
Inspection {
    inspection_id:      UUID
    facility_id:        UUID
    commodity_id:       UUID
    lot_number:         string
    inspector_id:       UUID            // Licensed inspector
    inspection_type:    INITIAL | RE_INSPECTION | PERIODIC
    status:             SCHEDULED | IN_PROGRESS | PASSED | FAILED
    scheduled_date:     Date
    completed_date:     Date
    gross_weight:       Decimal(18,4)
    net_weight:         Decimal(18,4)   // After moisture/foreign matter deductions
    moisture_pct:       Decimal(5,2)
    foreign_matter_pct: Decimal(5,2)
    protein_pct:        Decimal(5,2)    // For grains
    test_weight:        Decimal(8,2)    // kg/hectoliter
    grade_assigned:     string          // "GRADE_A", "GRADE_B", "GRADE_C", "SUBSTANDARD"
    defects:            string          // JSON array of defect descriptions
    notes:              string
    certificate_number: string          // Official grading certificate
    created_at:         Timestamp
    updated_at:         Timestamp
}
```

### 7.3 Grading Criteria (Mongolia Grain Standard)

| Parameter | Grade A | Grade B | Grade C |
|---|---|---|---|
| Moisture | ≤ 13.0% | ≤ 14.5% | ≤ 16.0% |
| Foreign matter | ≤ 1.0% | ≤ 2.0% | ≤ 3.0% |
| Protein (wheat) | ≥ 12.5% | ≥ 11.0% | ≥ 9.5% |
| Test weight (wheat) | ≥ 76 kg/hl | ≥ 74 kg/hl | ≥ 72 kg/hl |

Commodities graded `SUBSTANDARD` (exceeding Grade C thresholds) are rejected for receipt issuance.

### 7.4 Re-Inspection

Receipts stored longer than 90 days may trigger periodic re-inspection to detect shrinkage or quality degradation. If re-inspection reveals:
- **Grade unchanged**: No action required
- **Grade downgraded**: Receipt grade updated, event published, margin engine notified (collateral value may change)
- **Quantity reduced (shrinkage)**: Adjustment inventory event created, receipt quantity updated

---

## 8. Receipt Collateralization

### 8.1 Pledge/Release Workflow

Warehouse receipts can serve as collateral for margin requirements:

```
Participant (holder)
      |
  PledgeReceipt(receipt_id, clearing_member_id)
      |
      v
Warehouse Service
  1. Validate receipt is ACTIVE
  2. Status -> PLEDGED, pledged_to = clearing_member_id
  3. Publish PledgeEvent to Kafka
      |
      v
Margin Engine
  4. Receives PledgeEvent
  5. Adds receipt value to participant's collateral pool
  6. Recalculates margin requirements

---

ReleaseReceipt(receipt_id)
      |
      v
Warehouse Service
  1. Validate receipt is PLEDGED
  2. Verify clearing member authorizes release
  3. Status -> ACTIVE, pledged_to = null
  4. Publish ReleaseEvent to Kafka
      |
      v
Margin Engine
  5. Receives ReleaseEvent
  6. Removes receipt from collateral pool
  7. Recalculates margin requirements (may trigger margin call)
```

### 8.2 Collateral Valuation

The receipt's collateral value is computed by the margin engine as:

```
collateral_value = receipt.quantity × commodity_reference_price × haircut_factor
```

Where:
- `commodity_reference_price` = latest settlement price from the exchange
- `haircut_factor` = discount for commodity type and grade (e.g., 0.80 for Grade A wheat, 0.70 for Grade B)

The warehouse service does NOT compute collateral value — it only manages the pledge/release status and notifies the margin engine.

### 8.3 Pledge Constraints

1. Only `ACTIVE` receipts can be pledged
2. A receipt can only be pledged to one clearing member at a time
3. Pledged receipts cannot be transferred, delivered, or cancelled
4. Release requires authorization from the pledgee (clearing member)
5. If the facility is suspended, pledged receipts remain valid but their haircut may increase

---

## 9. API Contracts (gRPC)

### 9.1 Service Definition

```protobuf
syntax = "proto3";

package ace.warehouse.v1;

option go_package = "ace-platform/src/warehouse-service/proto/warehousepb";

import "google/protobuf/timestamp.proto";

// WarehouseService manages electronic warehouse receipts
// and physical commodity inventory.
service WarehouseService {
    // Receipt lifecycle
    rpc IssueReceipt(IssueReceiptRequest) returns (IssueReceiptResponse);
    rpc TransferReceipt(TransferReceiptRequest) returns (TransferReceiptResponse);
    rpc CancelReceipt(CancelReceiptRequest) returns (CancelReceiptResponse);
    rpc GetReceipt(GetReceiptRequest) returns (GetReceiptResponse);
    rpc ListReceipts(ListReceiptsRequest) returns (ListReceiptsResponse);

    // Delivery
    rpc InitiateDelivery(InitiateDeliveryRequest) returns (InitiateDeliveryResponse);
    rpc CompleteDelivery(CompleteDeliveryRequest) returns (CompleteDeliveryResponse);
    rpc GetDelivery(GetDeliveryRequest) returns (GetDeliveryResponse);
    rpc ListDeliveries(ListDeliveriesRequest) returns (ListDeliveriesResponse);

    // Inventory
    rpc GetInventory(GetInventoryRequest) returns (GetInventoryResponse);
    rpc GetFacilityCapacity(GetFacilityCapacityRequest) returns (GetFacilityCapacityResponse);

    // Collateralization
    rpc PledgeReceipt(PledgeReceiptRequest) returns (PledgeReceiptResponse);
    rpc ReleaseReceipt(ReleaseReceiptRequest) returns (ReleaseReceiptResponse);

    // Facility management
    rpc RegisterFacility(RegisterFacilityRequest) returns (RegisterFacilityResponse);
    rpc UpdateFacility(UpdateFacilityRequest) returns (UpdateFacilityResponse);
    rpc GetFacility(GetFacilityRequest) returns (GetFacilityResponse);
    rpc ListFacilities(ListFacilitiesRequest) returns (ListFacilitiesResponse);

    // Inspection
    rpc ScheduleInspection(ScheduleInspectionRequest) returns (ScheduleInspectionResponse);
    rpc RecordInspectionResult(RecordInspectionResultRequest) returns (RecordInspectionResultResponse);
    rpc GetInspection(GetInspectionRequest) returns (GetInspectionResponse);
}

// ─── Enums ──────────────────────────────────────────────────────────────────

enum ReceiptStatus {
    RECEIPT_STATUS_UNSPECIFIED = 0;
    RECEIPT_STATUS_ACTIVE = 1;
    RECEIPT_STATUS_PLEDGED = 2;
    RECEIPT_STATUS_DELIVERY_PENDING = 3;
    RECEIPT_STATUS_DELIVERED = 4;
    RECEIPT_STATUS_CANCELLED = 5;
}

enum FacilityStatus {
    FACILITY_STATUS_UNSPECIFIED = 0;
    FACILITY_STATUS_ACTIVE = 1;
    FACILITY_STATUS_SUSPENDED = 2;
    FACILITY_STATUS_DECOMMISSIONED = 3;
}

enum DeliveryType {
    DELIVERY_TYPE_UNSPECIFIED = 0;
    DELIVERY_TYPE_PHYSICAL = 1;
    DELIVERY_TYPE_CASH_SETTLEMENT = 2;
}

enum DeliveryStatus {
    DELIVERY_STATUS_UNSPECIFIED = 0;
    DELIVERY_STATUS_PENDING = 1;
    DELIVERY_STATUS_IN_PROGRESS = 2;
    DELIVERY_STATUS_COMPLETED = 3;
    DELIVERY_STATUS_FAILED = 4;
    DELIVERY_STATUS_CANCELLED = 5;
}

enum InspectionType {
    INSPECTION_TYPE_UNSPECIFIED = 0;
    INSPECTION_TYPE_INITIAL = 1;
    INSPECTION_TYPE_RE_INSPECTION = 2;
    INSPECTION_TYPE_PERIODIC = 3;
}

enum InspectionStatus {
    INSPECTION_STATUS_UNSPECIFIED = 0;
    INSPECTION_STATUS_SCHEDULED = 1;
    INSPECTION_STATUS_IN_PROGRESS = 2;
    INSPECTION_STATUS_PASSED = 3;
    INSPECTION_STATUS_FAILED = 4;
}

// ─── Messages ───────────────────────────────────────────────────────────────

message Receipt {
    string receipt_id = 1;
    string receipt_number = 2;
    string facility_id = 3;
    string holder_id = 4;
    string commodity_id = 5;
    string grade = 6;
    string quantity = 7;           // Decimal as string (e.g., "100.0000")
    string gross_quantity = 8;
    string unit = 9;
    string lot_number = 10;
    string storage_location = 11;
    int32 harvest_year = 12;
    string inspection_id = 13;
    ReceiptStatus status = 14;
    string pledged_to = 15;        // Empty if not pledged
    google.protobuf.Timestamp issued_at = 16;
    google.protobuf.Timestamp expires_at = 17;
    google.protobuf.Timestamp created_at = 18;
    google.protobuf.Timestamp updated_at = 19;
}

message Facility {
    string facility_id = 1;
    string facility_code = 2;
    string name = 3;
    string operator_id = 4;
    string license_number = 5;
    string license_expiry = 6;     // Date as "YYYY-MM-DD"
    string address = 7;
    string latitude = 8;
    string longitude = 9;
    string region = 10;
    string total_capacity = 11;    // Decimal as string
    string capacity_unit = 12;
    repeated string approved_commodity_ids = 13;
    FacilityStatus status = 14;
    google.protobuf.Timestamp created_at = 15;
    google.protobuf.Timestamp updated_at = 16;
}

message DeliveryInstruction {
    string delivery_id = 1;
    string receipt_id = 2;
    string obligation_id = 3;
    string seller_id = 4;
    string buyer_id = 5;
    DeliveryType delivery_type = 6;
    string facility_id = 7;
    string destination_id = 8;
    string quantity = 9;
    string scheduled_date = 10;    // "YYYY-MM-DD"
    DeliveryStatus status = 11;
    string failure_reason = 12;
    google.protobuf.Timestamp completed_at = 13;
    google.protobuf.Timestamp created_at = 14;
    google.protobuf.Timestamp updated_at = 15;
}

message Inspection {
    string inspection_id = 1;
    string facility_id = 2;
    string commodity_id = 3;
    string lot_number = 4;
    string inspector_id = 5;
    InspectionType inspection_type = 6;
    InspectionStatus status = 7;
    string scheduled_date = 8;
    string completed_date = 9;
    string gross_weight = 10;
    string net_weight = 11;
    string moisture_pct = 12;
    string foreign_matter_pct = 13;
    string protein_pct = 14;
    string test_weight = 15;
    string grade_assigned = 16;
    string defects = 17;           // JSON array
    string notes = 18;
    string certificate_number = 19;
    google.protobuf.Timestamp created_at = 20;
    google.protobuf.Timestamp updated_at = 21;
}

message InventoryItem {
    string facility_id = 1;
    string commodity_id = 2;
    string lot_number = 3;
    string current_quantity = 4;   // Decimal as string
    string unit = 5;
}

// ─── Request/Response ───────────────────────────────────────────────────────

// IssueReceipt
message IssueReceiptRequest {
    string facility_id = 1;
    string holder_id = 2;
    string commodity_id = 3;
    string grade = 4;
    string quantity = 5;
    string gross_quantity = 6;
    string unit = 7;
    string lot_number = 8;
    string storage_location = 9;
    int32 harvest_year = 10;
    string inspection_id = 11;
}

message IssueReceiptResponse {
    Receipt receipt = 1;
}

// TransferReceipt
message TransferReceiptRequest {
    string receipt_id = 1;
    string new_holder_id = 2;
}

message TransferReceiptResponse {
    Receipt receipt = 1;
}

// CancelReceipt
message CancelReceiptRequest {
    string receipt_id = 1;
    string reason = 2;
}

message CancelReceiptResponse {
    Receipt receipt = 1;
}

// GetReceipt
message GetReceiptRequest {
    string receipt_id = 1;
}

message GetReceiptResponse {
    Receipt receipt = 1;
}

// ListReceipts
message ListReceiptsRequest {
    string holder_id = 1;          // Filter by holder (optional)
    string facility_id = 2;        // Filter by facility (optional)
    string commodity_id = 3;       // Filter by commodity (optional)
    ReceiptStatus status = 4;      // Filter by status (optional)
    int32 page_size = 5;
    string page_token = 6;
}

message ListReceiptsResponse {
    repeated Receipt receipts = 1;
    string next_page_token = 2;
}

// InitiateDelivery
message InitiateDeliveryRequest {
    string receipt_id = 1;
    string obligation_id = 2;
    string buyer_id = 3;
    DeliveryType delivery_type = 4;
    string destination_id = 5;     // Required for PHYSICAL delivery
    string scheduled_date = 6;
}

message InitiateDeliveryResponse {
    DeliveryInstruction delivery = 1;
}

// CompleteDelivery
message CompleteDeliveryRequest {
    string delivery_id = 1;
    bool success = 2;
    string failure_reason = 3;     // Required if success=false
}

message CompleteDeliveryResponse {
    DeliveryInstruction delivery = 1;
}

// GetDelivery
message GetDeliveryRequest {
    string delivery_id = 1;
}

message GetDeliveryResponse {
    DeliveryInstruction delivery = 1;
}

// ListDeliveries
message ListDeliveriesRequest {
    string seller_id = 1;
    string buyer_id = 2;
    DeliveryStatus status = 3;
    int32 page_size = 4;
    string page_token = 5;
}

message ListDeliveriesResponse {
    repeated DeliveryInstruction deliveries = 1;
    string next_page_token = 2;
}

// GetInventory
message GetInventoryRequest {
    string facility_id = 1;
    string commodity_id = 2;       // Optional — all commodities if empty
}

message GetInventoryResponse {
    repeated InventoryItem items = 1;
    string total_quantity = 2;
}

// GetFacilityCapacity
message GetFacilityCapacityRequest {
    string facility_id = 1;
}

message GetFacilityCapacityResponse {
    string total_capacity = 1;
    string used_capacity = 2;
    string available_capacity = 3;
    string capacity_unit = 4;
}

// PledgeReceipt
message PledgeReceiptRequest {
    string receipt_id = 1;
    string clearing_member_id = 2;
}

message PledgeReceiptResponse {
    Receipt receipt = 1;
}

// ReleaseReceipt
message ReleaseReceiptRequest {
    string receipt_id = 1;
    string clearing_member_id = 2; // Must match current pledgee
}

message ReleaseReceiptResponse {
    Receipt receipt = 1;
}

// RegisterFacility
message RegisterFacilityRequest {
    string facility_code = 1;
    string name = 2;
    string operator_id = 3;
    string license_number = 4;
    string license_expiry = 5;
    string address = 6;
    string latitude = 7;
    string longitude = 8;
    string region = 9;
    string total_capacity = 10;
    string capacity_unit = 11;
    repeated string approved_commodity_ids = 12;
}

message RegisterFacilityResponse {
    Facility facility = 1;
}

// UpdateFacility
message UpdateFacilityRequest {
    string facility_id = 1;
    string name = 2;
    string license_number = 3;
    string license_expiry = 4;
    string address = 5;
    string total_capacity = 6;
    FacilityStatus status = 7;
    repeated string approved_commodity_ids = 8;
}

message UpdateFacilityResponse {
    Facility facility = 1;
}

// GetFacility
message GetFacilityRequest {
    string facility_id = 1;
}

message GetFacilityResponse {
    Facility facility = 1;
}

// ListFacilities
message ListFacilitiesRequest {
    string region = 1;
    FacilityStatus status = 2;
    int32 page_size = 3;
    string page_token = 4;
}

message ListFacilitiesResponse {
    repeated Facility facilities = 1;
    string next_page_token = 2;
}

// ScheduleInspection
message ScheduleInspectionRequest {
    string facility_id = 1;
    string commodity_id = 2;
    string lot_number = 3;
    string inspector_id = 4;
    InspectionType inspection_type = 5;
    string scheduled_date = 6;
}

message ScheduleInspectionResponse {
    Inspection inspection = 1;
}

// RecordInspectionResult
message RecordInspectionResultRequest {
    string inspection_id = 1;
    string gross_weight = 2;
    string net_weight = 3;
    string moisture_pct = 4;
    string foreign_matter_pct = 5;
    string protein_pct = 6;
    string test_weight = 7;
    string grade_assigned = 8;
    string defects = 9;
    string notes = 10;
    string certificate_number = 11;
    bool passed = 12;
}

message RecordInspectionResultResponse {
    Inspection inspection = 1;
}

// GetInspection
message GetInspectionRequest {
    string inspection_id = 1;
}

message GetInspectionResponse {
    Inspection inspection = 1;
}
```

### 9.2 Error Codes

| gRPC Code | Condition |
|---|---|
| `NOT_FOUND` | Receipt, facility, delivery, or inspection not found |
| `FAILED_PRECONDITION` | Receipt not in valid state for operation (e.g., pledged receipt cannot be transferred) |
| `ALREADY_EXISTS` | Duplicate facility code or receipt number |
| `INVALID_ARGUMENT` | Missing required fields, invalid quantity, bad date format |
| `RESOURCE_EXHAUSTED` | Facility capacity exceeded |
| `PERMISSION_DENIED` | Caller not authorized (not the holder, not the pledgee) |
| `INTERNAL` | Database or messaging system failure |

---

## 10. SQL Data Model

### 10.1 Schema: `warehouse`

```sql
-- V8__warehouse_tables.sql
-- Warehouse service: receipts, facilities, inventory, deliveries, inspections

CREATE SCHEMA IF NOT EXISTS warehouse;

-- ─── Facilities ─────────────────────────────────────────────────────────────

CREATE TABLE warehouse.facilities (
    facility_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_code       VARCHAR(20) NOT NULL UNIQUE,
    name                VARCHAR(200) NOT NULL,
    operator_id         UUID NOT NULL,
    license_number      VARCHAR(50) NOT NULL,
    license_expiry      DATE NOT NULL,
    address             TEXT NOT NULL,
    latitude            NUMERIC(10,7),
    longitude           NUMERIC(10,7),
    region              VARCHAR(100) NOT NULL,
    total_capacity      NUMERIC(18,4) NOT NULL,
    capacity_unit       VARCHAR(10) NOT NULL DEFAULT 'MT',
    status              VARCHAR(20) NOT NULL DEFAULT 'ACTIVE'
                        CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DECOMMISSIONED')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE warehouse.facility_commodities (
    facility_id         UUID NOT NULL REFERENCES warehouse.facilities(facility_id),
    commodity_id        UUID NOT NULL,
    PRIMARY KEY (facility_id, commodity_id)
);

-- ─── Inspections ────────────────────────────────────────────────────────────

CREATE TABLE warehouse.inspections (
    inspection_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_id         UUID NOT NULL REFERENCES warehouse.facilities(facility_id),
    commodity_id        UUID NOT NULL,
    lot_number          VARCHAR(50) NOT NULL,
    inspector_id        UUID NOT NULL,
    inspection_type     VARCHAR(20) NOT NULL
                        CHECK (inspection_type IN ('INITIAL', 'RE_INSPECTION', 'PERIODIC')),
    status              VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED'
                        CHECK (status IN ('SCHEDULED', 'IN_PROGRESS', 'PASSED', 'FAILED')),
    scheduled_date      DATE NOT NULL,
    completed_date      DATE,
    gross_weight        NUMERIC(18,4),
    net_weight          NUMERIC(18,4),
    moisture_pct        NUMERIC(5,2),
    foreign_matter_pct  NUMERIC(5,2),
    protein_pct         NUMERIC(5,2),
    test_weight         NUMERIC(8,2),
    grade_assigned      VARCHAR(20),
    defects             JSONB,
    notes               TEXT,
    certificate_number  VARCHAR(50),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_inspections_facility ON warehouse.inspections(facility_id);
CREATE INDEX idx_inspections_status ON warehouse.inspections(status);

-- ─── Warehouse Receipts ─────────────────────────────────────────────────────

CREATE TABLE warehouse.receipts (
    receipt_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_number      VARCHAR(40) NOT NULL UNIQUE,
    facility_id         UUID NOT NULL REFERENCES warehouse.facilities(facility_id),
    holder_id           UUID NOT NULL,
    commodity_id        UUID NOT NULL,
    grade               VARCHAR(20) NOT NULL,
    quantity            NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    gross_quantity      NUMERIC(18,4) NOT NULL CHECK (gross_quantity > 0),
    unit                VARCHAR(10) NOT NULL DEFAULT 'MT',
    lot_number          VARCHAR(50) NOT NULL,
    storage_location    VARCHAR(50),
    harvest_year        INT NOT NULL,
    inspection_id       UUID REFERENCES warehouse.inspections(inspection_id),
    status              VARCHAR(20) NOT NULL DEFAULT 'ACTIVE'
                        CHECK (status IN ('ACTIVE', 'PLEDGED', 'DELIVERY_PENDING',
                                          'DELIVERED', 'CANCELLED')),
    pledged_to          UUID,
    issued_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    cancelled_at        TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_receipts_holder ON warehouse.receipts(holder_id);
CREATE INDEX idx_receipts_facility ON warehouse.receipts(facility_id);
CREATE INDEX idx_receipts_status ON warehouse.receipts(status);
CREATE INDEX idx_receipts_commodity ON warehouse.receipts(commodity_id);
CREATE INDEX idx_receipts_number ON warehouse.receipts(receipt_number);

-- ─── Receipt Events (Audit Trail) ──────────────────────────────────────────

CREATE TABLE warehouse.receipt_events (
    event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id          UUID NOT NULL REFERENCES warehouse.receipts(receipt_id),
    event_type          VARCHAR(30) NOT NULL
                        CHECK (event_type IN ('ISSUED', 'TRANSFERRED', 'PLEDGED',
                                              'RELEASED', 'DELIVERY_INITIATED',
                                              'DELIVERED', 'CANCELLED')),
    from_holder_id      UUID,
    to_holder_id        UUID,
    pledged_to          UUID,
    metadata            JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_receipt_events_receipt ON warehouse.receipt_events(receipt_id);
CREATE INDEX idx_receipt_events_type ON warehouse.receipt_events(event_type);

-- ─── Inventory Events (Double-Entry) ───────────────────────────────────────

CREATE TABLE warehouse.inventory_events (
    event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_id         UUID NOT NULL REFERENCES warehouse.facilities(facility_id),
    commodity_id        UUID NOT NULL,
    lot_number          VARCHAR(50) NOT NULL,
    event_type          VARCHAR(20) NOT NULL
                        CHECK (event_type IN ('DEPOSIT', 'WITHDRAWAL', 'ADJUSTMENT',
                                              'TRANSFER_IN', 'TRANSFER_OUT', 'SHRINKAGE')),
    quantity            NUMERIC(18,4) NOT NULL,
    reference_id        UUID,
    reference_type      VARCHAR(20),
    notes               TEXT,
    created_by          UUID NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_inventory_events_facility ON warehouse.inventory_events(facility_id);
CREATE INDEX idx_inventory_events_commodity ON warehouse.inventory_events(commodity_id);
CREATE INDEX idx_inventory_events_lot ON warehouse.inventory_events(facility_id, commodity_id, lot_number);

-- ─── Delivery Instructions ──────────────────────────────────────────────────

CREATE TABLE warehouse.deliveries (
    delivery_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id          UUID NOT NULL REFERENCES warehouse.receipts(receipt_id),
    obligation_id       UUID NOT NULL,
    seller_id           UUID NOT NULL,
    buyer_id            UUID NOT NULL,
    delivery_type       VARCHAR(20) NOT NULL
                        CHECK (delivery_type IN ('PHYSICAL', 'CASH_SETTLEMENT')),
    facility_id         UUID NOT NULL REFERENCES warehouse.facilities(facility_id),
    destination_id      UUID,
    quantity            NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    scheduled_date      DATE NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'IN_PROGRESS', 'COMPLETED',
                                          'FAILED', 'CANCELLED')),
    failure_reason      TEXT,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_deliveries_receipt ON warehouse.deliveries(receipt_id);
CREATE INDEX idx_deliveries_obligation ON warehouse.deliveries(obligation_id);
CREATE INDEX idx_deliveries_seller ON warehouse.deliveries(seller_id);
CREATE INDEX idx_deliveries_buyer ON warehouse.deliveries(buyer_id);
CREATE INDEX idx_deliveries_status ON warehouse.deliveries(status);

-- ─── Views ──────────────────────────────────────────────────────────────────

-- Current inventory per facility/commodity/lot
CREATE VIEW warehouse.current_inventory AS
SELECT facility_id, commodity_id, lot_number,
       SUM(quantity) AS current_quantity
FROM warehouse.inventory_events
GROUP BY facility_id, commodity_id, lot_number
HAVING SUM(quantity) > 0;

-- Facility capacity utilization
CREATE VIEW warehouse.facility_utilization AS
SELECT f.facility_id,
       f.facility_code,
       f.total_capacity,
       COALESCE(SUM(r.quantity), 0) AS used_capacity,
       f.total_capacity - COALESCE(SUM(r.quantity), 0) AS available_capacity,
       f.capacity_unit
FROM warehouse.facilities f
LEFT JOIN warehouse.receipts r
    ON r.facility_id = f.facility_id
    AND r.status IN ('ACTIVE', 'PLEDGED', 'DELIVERY_PENDING')
GROUP BY f.facility_id, f.facility_code, f.total_capacity, f.capacity_unit;
```

---

## 11. Integration Points

### 11.1 Clearing Engine Integration

| Operation | Direction | Protocol | Description |
|---|---|---|---|
| Query delivery obligation | Warehouse → Clearing | gRPC | Validate obligation exists and matches receipt |
| Delivery confirmed | Warehouse → Clearing | gRPC/Event | Notify that physical delivery is complete |
| Delivery failed | Warehouse → Clearing | gRPC/Event | Notify failure; clearing re-processes obligation |

Event topic: `ace.warehouse.delivery`

### 11.2 Margin Engine Integration

| Operation | Direction | Protocol | Description |
|---|---|---|---|
| Receipt pledged | Warehouse → Margin | Event | Margin engine adds receipt to collateral pool |
| Receipt released | Warehouse → Margin | Event | Margin engine removes from collateral pool |
| Grade changed | Warehouse → Margin | Event | Collateral value may change (haircut recalc) |

Event topic: `ace.warehouse.collateral`

### 11.3 Event Schema

```json
{
    "event_id": "uuid",
    "event_type": "RECEIPT_ISSUED | RECEIPT_TRANSFERRED | RECEIPT_PLEDGED | RECEIPT_RELEASED | DELIVERY_COMPLETED | DELIVERY_FAILED | GRADE_CHANGED",
    "receipt_id": "uuid",
    "facility_id": "uuid",
    "holder_id": "uuid",
    "commodity_id": "uuid",
    "quantity": "100.0000",
    "grade": "GRADE_A",
    "timestamp": "2026-07-15T10:30:00Z",
    "metadata": {}
}
```

### 11.4 Compliance Service Integration

- Receipt issuance triggers a KYC check on the depositor (holder_id)
- Receipt transfer triggers a KYC check on the new holder
- All receipt events are forwarded to the compliance audit trail

---

## 12. Performance Requirements

| Metric | Target |
|---|---|
| Receipt issuance latency (p99) | < 200ms |
| Receipt transfer latency (p99) | < 100ms |
| Pledge/release latency (p99) | < 100ms |
| Inventory query latency (p99) | < 500ms |
| Delivery initiation latency (p99) | < 300ms |
| Throughput (receipts/day) | 10,000+ |
| Concurrent facilities | 500+ |
| Receipts per facility | 100,000+ |

### 12.1 Scaling Strategy

- **Horizontal scaling**: Stateless gRPC servers behind a load balancer; state in PostgreSQL
- **Read replicas**: Inventory and receipt queries routed to read replicas
- **Connection pooling**: PgBouncer with 20 connections per instance
- **Caching**: Facility metadata cached in-memory (TTL 5 min); receipts are NOT cached (consistency critical)

---

## 13. Deployment Architecture

```
+---------------------+
|   Kubernetes Pod    |
| +-------+ +------+ |
| | gRPC  | | HTTP | |
| | :50058| | :8088| |
| +-------+ +------+ |
| |   warehouse-svc | |
| +-----------------+ |
+---------------------+
         |
    Istio sidecar
    (mTLS, traffic)
```

- **Port 50058**: gRPC API (all warehouse operations)
- **Port 8088**: HTTP health endpoint (`/healthz`, `/readyz`, `/metrics`)
- **Replicas**: 2 minimum (HA), scale to 6 based on gRPC request rate
- **Resource requests**: 256Mi memory, 250m CPU
- **Resource limits**: 512Mi memory, 500m CPU

### 13.1 Database Connection

- Service role: `ace_warehouse_svc` (read/write on `warehouse` schema)
- Read-only role: `ace_warehouse_ro` (for reporting/analytics)
- Connection string via Kubernetes secret: `warehouse-db-credentials`

---

## 14. Failure Modes & Recovery

| Failure | Impact | Recovery |
|---|---|---|
| PostgreSQL down | All operations fail | Circuit breaker; retry with exponential backoff; health check reports unhealthy |
| Kafka/NATS down | Events not published | Buffer events in-memory (bounded queue, 1000 events); retry on reconnect; idempotent consumers handle duplicates |
| Clearing engine unreachable | Delivery validation fails | Delivery initiation returns UNAVAILABLE; retry via scheduled job |
| Margin engine unreachable | Pledge/release events not received | Events are persistent in Kafka; margin engine catches up on recovery |
| Facility capacity inconsistency | Over-issuance risk | Use `SELECT ... FOR UPDATE` on facility row during issuance to serialize capacity checks |
| Duplicate receipt issuance | Double-counting risk | Receipt number uniqueness constraint prevents duplicates; idempotency key on IssueReceipt |
| Receipt in inconsistent state | Data integrity risk | All state transitions in single DB transaction; receipt_events table provides audit trail for reconciliation |

### 14.1 Idempotency

All mutating RPCs accept an optional `idempotency_key` in gRPC metadata. The service stores processed keys in a `warehouse.idempotency_keys` table with a 24-hour TTL. Duplicate requests return the original response without re-executing.

### 14.2 Reconciliation

A daily batch job reconciles:
1. Sum of active receipt quantities per facility vs facility capacity view
2. Inventory event running totals vs receipt quantities
3. Pledged receipts vs margin engine's collateral records (via gRPC query)

Discrepancies are logged as alerts and published to `ace.warehouse.reconciliation` topic.
