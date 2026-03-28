-- V8__warehouse_tables.sql
-- Warehouse service: electronic warehouse receipts, facilities, inventory, deliveries, inspections
-- Spec: docs/T037_warehouse_spec.md

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

-- ─── Idempotency Keys ──────────────────────────────────────────────────────

CREATE TABLE warehouse.idempotency_keys (
    idempotency_key     VARCHAR(100) PRIMARY KEY,
    response_data       BYTEA NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '24 hours')
);

CREATE INDEX idx_idempotency_expires ON warehouse.idempotency_keys(expires_at);

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

-- ─── Service Role ───────────────────────────────────────────────────────────

-- Grant warehouse service role access
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_warehouse_svc') THEN
        CREATE ROLE ace_warehouse_svc LOGIN;
    END IF;
END $$;

GRANT USAGE ON SCHEMA warehouse TO ace_warehouse_svc;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA warehouse TO ace_warehouse_svc;
GRANT SELECT ON warehouse.current_inventory TO ace_warehouse_svc;
GRANT SELECT ON warehouse.facility_utilization TO ace_warehouse_svc;

-- Read-only role for reporting
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_warehouse_ro') THEN
        CREATE ROLE ace_warehouse_ro LOGIN;
    END IF;
END $$;

GRANT USAGE ON SCHEMA warehouse TO ace_warehouse_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA warehouse TO ace_warehouse_ro;
GRANT SELECT ON warehouse.current_inventory TO ace_warehouse_ro;
GRANT SELECT ON warehouse.facility_utilization TO ace_warehouse_ro;
